package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"text/template"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/core/dcpool"
	"github.com/iyear/tdl/core/downloader"
	"github.com/iyear/tdl/core/tmedia"
	"github.com/iyear/tdl/core/util/fsutil"
	"github.com/iyear/tdl/core/util/tutil"
	"github.com/iyear/tdl/pkg/tmessage"
	"github.com/iyear/tdl/pkg/tplfunc"
	"github.com/iyear/tdl/pkg/utils"

	"tdl-ui/internal/script"
	"tdl-ui/internal/scriptapi"
)

const tempExt = ".tmp"

// maxAlbumSize Telegram 相册最多 10 项。elem 通道容量必须 ≥ 该值（ENG-09）：
// processGrouped 会在一次 process 调用内连续向 elem 发送整组元素，
// 而消费方在 Next() 返回 true 后才逐个 Value()；配合 Next() 顶部的
// len(i.elem) > 0 检查，保证发送永不阻塞。改动三者之一需同步评审。
const maxAlbumSize = 10

// fileTemplate 命名模板的数据模型，与 tdl 保持一致。
type fileTemplate struct {
	DialogID     int64
	MessageID    int
	MessageDate  int64
	FileName     string
	FileCaption  string
	FileSize     string
	DownloadDate int64
}

// IterOptions 迭代器装配参数。
type IterOptions struct {
	Dir        string
	RewriteExt bool
	SkipSame   bool
	Template   string
	Group      bool
	Contracts  *script.Contracts // 可选：过滤/命名脚本契约
	OnSkip     func(info scriptapi.FileInfo, reason string)
	// OnTotalDecr 消息被跳过（无媒体/已删除/被过滤/同名同大小）时回调，
	// 供消费方递减 total，避免任务 Done 后进度仍显示 12/20（ENG-06）。
	// 注：断点命中的跳过已计入 finished，不触发此回调。
	OnTotalDecr func()
}

// iter 下载元素迭代器（对应 ref/tdl/app/dl/iter.go，去除 include/exclude，
// 由脚本 Filter 接管过滤，Rename 接管命名）。
type iter struct {
	pool    dcpool.Pool
	manager *peers.Manager
	dialogs []*tmessage.Dialog
	tpl     *template.Template
	opts    IterOptions

	mu           *sync.Mutex // 保护 finished/err 与索引推进；counter/elem 仅由 Next/Value 的单 goroutine 驱动方访问
	finished     map[string]struct{}
	dialogIndex  int
	messageIndex int

	counter int
	elem    chan downloader.Elem
	err     error
}

func newIter(pool dcpool.Pool, manager *peers.Manager, dialogs []*tmessage.Dialog, opts IterOptions) (*iter, error) {
	tpl, err := template.New("dl").
		Funcs(tplfunc.FuncMap(tplfunc.All...)).
		Parse(opts.Template)
	if err != nil {
		return nil, errors.Wrap(err, "parse template")
	}

	if len(dialogs) == 0 {
		return nil, errors.New("至少需要一条有效的消息链接")
	}

	// 保持 fingerprint 稳定（与 tdl 相同的排序规则）
	sortDialogs(dialogs)

	return &iter{
		pool:    pool,
		manager: manager,
		dialogs: dialogs,
		tpl:     tpl,
		opts:    opts,

		mu:       &sync.Mutex{},
		finished: make(map[string]struct{}),
		counter:  -1,
		elem:     make(chan downloader.Elem, maxAlbumSize), // 分组消息缓冲（ENG-09 不变量）
	}, nil
}

func (i *iter) Next(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		i.setErr(ctx.Err()) // ENG-07：与 Err()/process 统一持锁读写
		return false
	default:
	}

	if len(i.elem) > 0 { // 分组消息尚未消费完
		return true
	}

	for {
		ok, skip := i.process(ctx)
		if skip {
			continue
		}
		return ok
	}
}

func (i *iter) process(ctx context.Context) (ret bool, skip bool) {
	// ENG-08：锁内仅做终止判断与索引快照/推进，网络与磁盘 I/O 全部在锁外执行，
	// 避免 worker 的 Finish/Finished（断点持久化）被 I/O 长时间阻塞。
	i.mu.Lock()
	if i.dialogIndex >= len(i.dialogs) || i.messageIndex >= len(i.dialogs[i.dialogIndex].Messages) || i.err != nil {
		i.mu.Unlock()
		return false, false
	}
	peer, msg := i.dialogs[i.dialogIndex].Peer, i.dialogs[i.dialogIndex].Messages[i.messageIndex]
	if i.messageIndex++; i.messageIndex >= len(i.dialogs[i.dialogIndex].Messages) {
		i.dialogIndex++
		i.messageIndex = 0
	}
	i.mu.Unlock()

	from, err := i.manager.FromInputPeer(ctx, peer)
	if err != nil {
		i.setErr(errors.Wrap(err, "resolve from input peer"))
		return false, false
	}
	message, err := tutil.GetSingleMessage(ctx, i.pool.Default(ctx), peer, msg)
	if err != nil {
		if errors.Is(err, tutil.ErrMessageDeleted) { // 已删除消息直接跳过
			i.decrTotal()
			return false, true
		}
		i.setErr(errors.Wrap(err, "resolve message"))
		return false, false
	}

	if _, ok := message.GetGroupedID(); ok && i.opts.Group {
		return i.processGrouped(ctx, message, from)
	}

	if i.isFinished(resumeKey(from.ID(), message.ID)) { // 断点续传：已完成（已计入 finished，不减 total）
		return false, true
	}

	return i.processSingle(ctx, message, from, false)
}

// resumeKey 断点坐标：内容坐标 "dialogID:messageID"。
func resumeKey(dialogID int64, messageID int) string {
	return fmt.Sprintf("%d:%d", dialogID, messageID)
}

// processSingle 处理单条消息；inGroup 表示来自相册展开（整组在 total 中仅占 1 个名额，
// 组内跳过不单独递减 total，由 processGrouped 整组结算）。
func (i *iter) processSingle(ctx context.Context, message *tg.Message, from peers.Peer, inGroup bool) (bool, bool) {
	item, ok := tmedia.GetMedia(message)
	if !ok { // 无媒体消息
		if !inGroup {
			i.decrTotal()
		}
		return false, true
	}

	info := scriptapi.FileInfo{
		DialogID:    from.ID(),
		MessageID:   message.ID,
		MessageDate: int64(message.Date),
		FileName:    item.Name,
		FileCaption: message.Message,
		FileSize:    item.Size,
	}

	// 脚本过滤：返回 false 则跳过该文件
	if keep, err := i.opts.Contracts.SafeFilter(info); err != nil {
		i.notifySkip(info, fmt.Sprintf("过滤脚本出错，默认保留: %v", err))
	} else if !keep {
		i.notifySkip(info, "被过滤脚本跳过")
		if !inGroup {
			i.decrTotal()
		}
		return false, true
	}

	// 命名：优先脚本 Rename，出错或空串回退默认模板
	name := ""
	if i.opts.Contracts.HasRename() {
		if n, err := i.opts.Contracts.SafeRename(info); err != nil {
			i.notifySkip(info, fmt.Sprintf("命名脚本出错，回退默认模板: %v", err))
		} else {
			name = n
		}
	}
	if name == "" {
		toName := bytes.Buffer{}
		if err := i.tpl.Execute(&toName, &fileTemplate{
			DialogID:     info.DialogID,
			MessageID:    info.MessageID,
			MessageDate:  info.MessageDate,
			FileName:     info.FileName,
			FileCaption:  info.FileCaption,
			FileSize:     utils.Byte.FormatBinaryBytes(info.FileSize),
			DownloadDate: time.Now().Unix(),
		}); err != nil {
			i.setErr(errors.Wrap(err, "execute template"))
			return false, false
		}
		name = toName.String()
	}

	if i.opts.SkipSame {
		if stat, err := os.Stat(filepath.Join(i.opts.Dir, name)); err == nil {
			if fsutil.GetNameWithoutExt(name) == fsutil.GetNameWithoutExt(stat.Name()) &&
				stat.Size() == item.Size {
				i.notifySkip(info, "本地已存在同名同大小文件")
				if !inGroup {
					i.decrTotal()
				}
				return false, true
			}
		}
	}

	path := filepath.Join(i.opts.Dir, name+tempExt)

	// 命名结果可包含子目录，按需创建
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		i.setErr(errors.Wrap(err, "create dir"))
		return false, false
	}

	to, err := os.Create(path)
	if err != nil {
		i.setErr(errors.Wrap(err, "create file"))
		return false, false
	}

	i.counter++
	i.elem <- &iterElem{
		id:        i.counter,
		resumeKey: resumeKey(from.ID(), message.ID),

		from:    from,
		fromMsg: message,
		file:    item,
		info:    info,

		to: to,
	}

	return true, false
}

func (i *iter) processGrouped(ctx context.Context, message *tg.Message, from peers.Peer) (bool, bool) {
	grouped, err := tutil.GetGroupedMessages(ctx, i.pool.Default(ctx), from.InputPeer(), message)
	if err != nil {
		i.setErr(errors.Wrapf(err, "resolve grouped message %d/%d", from.ID(), message.ID))
		return false, false
	}

	hasValid, anyFinished := false, false
	for _, msg := range grouped {
		if i.isFinished(resumeKey(from.ID(), msg.ID)) {
			anyFinished = true
			continue
		}

		ret, skip := i.processSingle(ctx, msg, from, true)
		if !ret && !skip {
			return false, false
		}
		if ret {
			hasValid = true
		}
	}

	if !hasValid && !anyFinished {
		i.decrTotal() // 整组无一产出且非断点命中，释放该组占用的 total 名额
	}
	return hasValid, !hasValid
}

func (i *iter) notifySkip(info scriptapi.FileInfo, reason string) {
	if i.opts.OnSkip != nil {
		i.opts.OnSkip(info, reason)
	}
}

// decrTotal 通知消费方一条消息被跳过、不再计入 total（ENG-06）。
func (i *iter) decrTotal() {
	if i.opts.OnTotalDecr != nil {
		i.opts.OnTotalDecr()
	}
}

// setErr / Err / isFinished 统一持锁访问共享字段（ENG-07）。
func (i *iter) setErr(err error) {
	i.mu.Lock()
	i.err = err
	i.mu.Unlock()
}

func (i *iter) isFinished(key string) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	_, ok := i.finished[key]
	return ok
}

func (i *iter) Value() downloader.Elem { return <-i.elem }

// Close 排空 elem 缓冲通道，关闭滞留元素的文件句柄并删除对应 .tmp 文件。
// 暂停/取消时 downloader 已停止消费，通道内未被消费的元素若不清理，
// fd 会泄漏到进程退出，孤儿 .tmp 也因未登记而无法被删除流程覆盖。
func (i *iter) Close() {
	for {
		select {
		case e := <-i.elem:
			if el, ok := e.(*iterElem); ok && el.to != nil {
				name := el.to.Name()
				_ = el.to.Close()
				_ = os.Remove(name)
			}
		default:
			return
		}
	}
}

func (i *iter) Err() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.err
}

func (i *iter) SetFinished(finished map[string]struct{}) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.finished = finished
}

// Finished 返回断点集合的拷贝，避免消费方在不持锁的情况下与 Finish 并发迭代/写同一 map。
func (i *iter) Finished() map[string]struct{} {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := make(map[string]struct{}, len(i.finished))
	for k := range i.finished {
		out[k] = struct{}{}
	}
	return out
}

func (i *iter) Finish(key string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.finished[key] = struct{}{}
}

func (i *iter) Total() int {
	i.mu.Lock()
	defer i.mu.Unlock()

	total := 0
	for _, m := range i.dialogs {
		total += len(m.Messages)
	}
	return total
}

func sortDialogs(dialogs []*tmessage.Dialog) {
	sort.Slice(dialogs, func(i, j int) bool {
		return tutil.GetInputPeerID(dialogs[i].Peer) <
			tutil.GetInputPeerID(dialogs[j].Peer)
	})

	for _, m := range dialogs {
		sort.Ints(m.Messages)
	}
}
