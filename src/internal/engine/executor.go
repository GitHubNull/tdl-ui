package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram/peers"

	"github.com/iyear/tdl/core/dcpool"
	"github.com/iyear/tdl/core/downloader"
	"github.com/iyear/tdl/core/storage"
	coretclient "github.com/iyear/tdl/core/tclient"
	pkgtclient "github.com/iyear/tdl/pkg/tclient"
	"github.com/iyear/tdl/pkg/tmessage"

	"tdl-ui/internal/config"
	"tdl-ui/internal/events"
	"tdl-ui/internal/scriptapi"
	"tdl-ui/internal/store"
)

// run 驱动任务执行并根据结果迁移状态。
func (m *Manager) run(t *Task) {
	if !m.exists(t.ID) { // 记录已被删除（如删除全部文件），不再执行
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})

	t.mu.Lock()
	if t.removed || t.status != StatusQueued {
		// 排队期间已被暂停/取消/移除，放弃执行
		t.mu.Unlock()
		cancel()
		return
	}
	t.cancel = cancel
	t.runDone = runDone
	t.status = StatusRunning
	t.mu.Unlock()
	logEngine.Infof("任务开始运行: id=%s", t.ID)
	t.emitUpdate()

	_ = t.contracts.SafeOnTaskStart(t.info())

	exec := m.executor
	if exec == nil {
		exec = m.execute
	}
	err := exec(ctx, t)
	cancel()

	t.mu.Lock()
	t.cancel = nil
	t.runDone = nil
	switch {
	case err == nil:
		t.status = StatusDone
	case errors.Is(err, context.Canceled):
		if t.paused {
			t.status = StatusPaused
		} else {
			t.status = StatusCanceled
		}
	default:
		t.status = StatusFailed
		t.errMsg = err.Error()
	}
	status := t.status
	t.mu.Unlock()
	close(runDone)

	if status == StatusFailed {
		logEngine.Errorf("任务失败: id=%s err=%v", t.ID, err)
	} else {
		logEngine.Infof("任务结束: id=%s 状态=%s", t.ID, status)
	}

	t.emitUpdate()
	_ = t.contracts.SafeOnTaskDone(t.info())
}

// execute 单次任务执行：建连 → 解析链接 → 装配迭代器 → 断点续传 → 下载。
// 装配流程对应 ref/tdl/app/dl/dl.go 的 Run。
func (m *Manager) execute(ctx context.Context, t *Task) (rerr error) {
	settings := m.deps.Cfg.Get()

	// 每轮从 task_items 表重组装 URLs/Selections，与持久化的消息项保持一致。
	urls, selections := t.opts.URLs, t.opts.Selections
	if m.deps.Store != nil {
		items, err := m.deps.Store.ListItems(ctx, t.ID)
		if err != nil {
			return errors.Wrap(err, "读取任务消息项失败")
		}
		if len(items) > 0 {
			urls, selections = itemsToOptions(items)
			t.mu.Lock()
			t.opts.URLs = urls
			t.opts.Selections = selections
			t.mu.Unlock()
		}
	}

	kvd, err := m.deps.KV.Open(Namespace)
	if err != nil {
		return errors.Wrap(err, "open kv")
	}

	c, err := pkgtclient.New(ctx, pkgtclient.Options{
		KV:               kvd,
		Proxy:            config.EffectiveProxy(settings),
		ReconnectTimeout: reconnectTimeout,
	}, false)
	if err != nil {
		return errors.Wrap(err, "create client")
	}

	return coretclient.RunWithAuth(ctx, c, func(ctx context.Context) error {
		pool := dcpool.NewPool(c,
			int64(settings.PoolSize),
			coretclient.NewDefaultMiddlewares(ctx, reconnectTimeout)...)
		defer func() { _ = pool.Close() }()

		var dialogs []*tmessage.Dialog
		if len(urls) > 0 {
			d, err := tmessage.Parse(tmessage.FromURL(ctx, pool, kvd, urls))
			if err != nil {
				return errors.Wrap(err, "解析消息链接失败")
			}
			dialogs = d
		}

		manager := peers.Options{Storage: storage.NewPeers(kvd)}.Build(pool.Default(ctx))

		// 选集下载：按对话 + 消息 ID 直接解析，与链接结果合并
		if len(selections) > 0 {
			d, err := selectionsToDialogs(ctx, manager, selections)
			if err != nil {
				return err
			}
			dialogs = append(dialogs, d...)
		}

		it, err := newIter(pool, manager, dialogs, IterOptions{
			Dir:        t.opts.Dir,
			RewriteExt: t.opts.RewriteExt,
			SkipSame:   t.opts.SkipSame,
			Template:   t.opts.Template,
			Group:      t.opts.Group,
			Contracts:  t.contracts,
			Renames:    t.opts.Renames,
			OnSkip: func(info scriptapi.FileInfo, reason string) {
				m.deps.Emitter.Emit(events.ScriptLog,
					fmt.Sprintf("[%s] %s: %s", t.ID, info.FileName, reason))
			},
			OnTotalDecr: func() {
				// ENG-06：被跳过的消息同步递减 total，保证任务 Done 时 finished 能追平 total；
				// 不即时 emit（大量无媒体消息时避免事件风暴），由下一次进度事件带出。
				t.mu.Lock()
				if t.total > 0 {
					t.total--
				}
				t.mu.Unlock()
			},
		})
		if err != nil {
			return err
		}
		// 暂停/取消后清理 elem 通道内滞留的文件句柄与孤儿 .tmp（ENG-02）
		defer it.Close()

		t.mu.Lock()
		t.total = it.Total()
		t.mu.Unlock()

		// 断点续传：从 resume_keys 加载已完成集合（内容坐标）；Restart 则清空进度
		if t.opts.Restart {
			if m.deps.Store != nil {
				_ = m.deps.Store.DeleteResume(ctx, t.ID)
			}
		} else if m.deps.Store != nil {
			finished, err := m.deps.Store.LoadFinished(ctx, t.ID)
			if err != nil {
				return errors.Wrap(err, "加载断点失败")
			}
			it.SetFinished(finished)
		}

		t.mu.Lock()
		t.finished = len(it.Finished())
		t.mu.Unlock()
		t.emitUpdate()

		defer func() { // 保存或清理断点（暂停/取消时任务 ctx 已取消，必须用 Background 完成写入，STO-03）
			if m.deps.Store == nil {
				return
			}
			if rerr != nil {
				if saveErr := m.deps.Store.SaveFinished(context.Background(), t.ID, it.Finished()); saveErr != nil &&
					!errors.Is(saveErr, store.ErrStoreClosed) { // 关闭阶段兜底写入失败不污染任务错误（ARC-002）
					rerr = errors.Wrapf(rerr, "save resume: %v", saveErr)
				}
			} else {
				_ = m.deps.Store.DeleteResume(context.Background(), t.ID)
			}
		}()

		return downloader.New(downloader.Options{
			Pool:    pool,
			Threads: settings.Threads,
			Iter:    it,
			// 进度节流间隔可经配置调优，零值回退默认 200ms（ARC-003）
			Progress: newProgress(t, it, t.opts.RewriteExt,
				time.Duration(settings.ProgressIntervalMs)*time.Millisecond),
		}).Download(ctx, settings.Limit)
	})
}
