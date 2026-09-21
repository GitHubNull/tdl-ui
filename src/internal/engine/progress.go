package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/go-faster/errors"

	"github.com/iyear/tdl/core/downloader"
	"github.com/iyear/tdl/core/util/fsutil"
)

// FileEvent 文件级进度事件负载（task:file）。
type FileEvent struct {
	TaskID     string `json:"taskId"`
	FileID     int    `json:"fileId"`
	DialogID   int64  `json:"dialogId"`
	MessageID  int    `json:"messageId"`
	Name       string `json:"name"`
	Total      int64  `json:"total"`
	Downloaded int64  `json:"downloaded"`
	// State: downloading / done / failed
	State string `json:"state"`
	Error string `json:"error,omitempty"`
	// Path 仅 done 时携带：最终文件路径，供前端完成瞬间切换"已下载"态
	Path string `json:"path,omitempty"`
	// Speed 当前下载速度，字节/秒；仅 downloading 时有效
	Speed int64 `json:"speed,omitempty"`
}

// defaultProgressEmitInterval 单文件进度事件的最小推送间隔默认值，避免高频刷屏；
// 可经配置 tuning.progressIntervalMs 调优，零值回退此默认（ARC-003）。
const defaultProgressEmitInterval = 200 * time.Millisecond

// progress 实现 core/downloader 的 Progress 接口
// （对应 ref/tdl/app/dl/progress.go，进度条替换为事件回调）。
type progress struct {
	task *Task
	it   *iter

	rewriteExt   bool
	emitInterval time.Duration

	mu       sync.Mutex
	lastEmit map[int]time.Time

	speedMu   sync.Mutex
	lastBytes map[int]int64     // fileId -> 上次采样字节数
	lastTime  map[int]time.Time // fileId -> 上次采样时间
}

func newProgress(task *Task, it *iter, rewriteExt bool, emitInterval time.Duration) *progress {
	if emitInterval <= 0 {
		emitInterval = defaultProgressEmitInterval
	}
	return &progress{
		task:         task,
		it:           it,
		rewriteExt:   rewriteExt,
		emitInterval: emitInterval,
		lastEmit:     make(map[int]time.Time),
		lastBytes:    make(map[int]int64),
		lastTime:     make(map[int]time.Time),
	}
}

func (p *progress) OnAdd(elem downloader.Elem) {
	e := elem.(*iterElem)
	p.task.addFile(TaskFile{
		Name:      strings.TrimSuffix(filepath.Base(e.to.Name()), tempExt),
		Path:      e.to.Name(),
		Size:      e.file.Size,
		State:     "downloading",
		DialogID:  e.info.DialogID,
		MessageID: e.info.MessageID,
	})
	p.emit(e, e.file.Size, 0, "downloading", nil)
}

func (p *progress) OnDownload(elem downloader.Elem, state downloader.ProgressState) {
	e := elem.(*iterElem)

	// 节流：同一文件 emitInterval 内只推送一次
	p.mu.Lock()
	last := p.lastEmit[e.id]
	now := time.Now()
	if now.Sub(last) < p.emitInterval {
		p.mu.Unlock()
		return
	}
	p.lastEmit[e.id] = now
	p.mu.Unlock()

	speed := p.sampleSpeed(e.id, state.Downloaded, now)
	p.emitWithSpeed(e, state.Total, state.Downloaded, "downloading", nil, speed)
}

func (p *progress) OnDone(elem downloader.Elem, err error) {
	e := elem.(*iterElem)
	tmpPath := e.to.Name()

	if cerr := e.to.Close(); cerr != nil {
		p.fail(e, errors.Wrap(cerr, "close file"))
		return
	}

	if err != nil {
		if !errors.Is(err, context.Canceled) { // 用户取消不算失败
			p.fail(e, err)
		}
		_ = os.Remove(tmpPath) // 尽力清理临时文件
		p.task.dropFile(tmpPath)
		return
	}

	p.it.Finish(e.resumeKey)
	p.task.saveResumeKey(e.resumeKey) // 实时持久化断点（内容坐标，行级追加）

	newpath, perr := p.donePost(e)
	if perr != nil {
		p.fail(e, errors.Wrap(perr, "post file"))
		return
	}
	p.task.finishFile(tmpPath, TaskFile{
		Name:      filepath.Base(newpath),
		Path:      newpath,
		Size:      e.file.Size,
		State:     "done",
		DialogID:  e.info.DialogID,
		MessageID: e.info.MessageID,
	})

	p.task.onFileDone(e.info)
	ev := p.buildEvent(e, e.file.Size, e.file.Size, "done", nil)
	ev.Name = filepath.Base(newpath) // rewriteExt 可能改名，以最终文件名为准
	ev.Path = newpath
	ev.Speed = 0
	p.clearSpeedSample(e.id)
	p.task.emitFile(ev)
}

// donePost 去除 .tmp 后缀、可选按 MIME 重写扩展名、还原消息时间；返回最终路径。
func (p *progress) donePost(e *iterElem) (string, error) {
	newfile := strings.TrimSuffix(filepath.Base(e.to.Name()), tempExt)

	if p.rewriteExt {
		mime, err := mimetype.DetectFile(e.to.Name())
		if err != nil {
			return "", errors.Wrap(err, "detect mime")
		}
		if ext := mime.Extension(); ext != "" && filepath.Ext(newfile) != ext {
			newfile = fsutil.GetNameWithoutExt(newfile) + ext
		}
	}

	newpath := filepath.Join(filepath.Dir(e.to.Name()), newfile)
	if err := os.Rename(e.to.Name(), newpath); err != nil {
		return "", errors.Wrap(err, "rename file")
	}

	if e.file.Date > 0 {
		fileTime := time.Unix(e.file.Date, 0)
		if err := os.Chtimes(newpath, fileTime, fileTime); err != nil {
			return "", errors.Wrap(err, "set file time")
		}
	}

	return newpath, nil
}

func (p *progress) fail(e *iterElem, err error) {
	logEngine.Errorf("文件下载失败: task=%s 文件=%s err=%v", p.task.ID, filepath.Base(e.to.Name()), err)
	p.task.markFileFailed(e.to.Name())
	p.task.onFileFailed()
	p.clearSpeedSample(e.id)
	p.emit(e, e.file.Size, 0, "failed", err)
}

func (p *progress) emit(e *iterElem, total, downloaded int64, state string, err error) {
	p.task.emitFile(p.buildEvent(e, total, downloaded, state, err))
}

func (p *progress) emitWithSpeed(e *iterElem, total, downloaded int64, state string, err error, speed int64) {
	ev := p.buildEvent(e, total, downloaded, state, err)
	ev.Speed = speed
	p.task.emitFile(ev)
}

func (p *progress) buildEvent(e *iterElem, total, downloaded int64, state string, err error) FileEvent {
	ev := FileEvent{
		TaskID:     p.task.ID,
		FileID:     e.id,
		DialogID:   e.info.DialogID,
		MessageID:  e.info.MessageID,
		Name:       strings.TrimSuffix(filepath.Base(e.to.Name()), tempExt),
		Total:      total,
		Downloaded: downloaded,
		State:      state,
	}
	if err != nil {
		ev.Error = err.Error()
	}
	return ev
}

// sampleSpeed 计算文件当前下载速度（字节/秒），并更新采样点。
func (p *progress) sampleSpeed(fileID int, downloaded int64, now time.Time) int64 {
	p.speedMu.Lock()
	defer p.speedMu.Unlock()

	lastBytes, lastTime := p.lastBytes[fileID], p.lastTime[fileID]
	p.lastBytes[fileID] = downloaded
	p.lastTime[fileID] = now

	if lastTime.IsZero() {
		return 0
	}
	dt := now.Sub(lastTime).Seconds()
	if dt <= 0 {
		return 0
	}
	delta := downloaded - lastBytes
	if delta <= 0 {
		return 0
	}
	return int64(float64(delta) / dt)
}

// clearSpeedSample 清理文件的速度采样状态。
func (p *progress) clearSpeedSample(fileID int) {
	p.speedMu.Lock()
	delete(p.lastBytes, fileID)
	delete(p.lastTime, fileID)
	p.speedMu.Unlock()
}
