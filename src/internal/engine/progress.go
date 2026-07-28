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
	Name       string `json:"name"`
	Total      int64  `json:"total"`
	Downloaded int64  `json:"downloaded"`
	// State: downloading / done / failed
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

// progressEmitInterval 单文件进度事件的最小推送间隔，避免高频刷屏。
const progressEmitInterval = 200 * time.Millisecond

// progress 实现 core/downloader 的 Progress 接口
// （对应 ref/tdl/app/dl/progress.go，进度条替换为事件回调）。
type progress struct {
	task *Task
	it   *iter

	rewriteExt bool

	mu       sync.Mutex
	lastEmit map[int]time.Time
}

func newProgress(task *Task, it *iter, rewriteExt bool) *progress {
	return &progress{
		task:       task,
		it:         it,
		rewriteExt: rewriteExt,
		lastEmit:   make(map[int]time.Time),
	}
}

func (p *progress) OnAdd(elem downloader.Elem) {
	e := elem.(*iterElem)
	p.emit(e, e.file.Size, 0, "downloading", nil)
}

func (p *progress) OnDownload(elem downloader.Elem, state downloader.ProgressState) {
	e := elem.(*iterElem)

	// 节流：同一文件 200ms 内只推送一次
	p.mu.Lock()
	last := p.lastEmit[e.id]
	now := time.Now()
	if now.Sub(last) < progressEmitInterval {
		p.mu.Unlock()
		return
	}
	p.lastEmit[e.id] = now
	p.mu.Unlock()

	p.emit(e, state.Total, state.Downloaded, "downloading", nil)
}

func (p *progress) OnDone(elem downloader.Elem, err error) {
	e := elem.(*iterElem)

	if cerr := e.to.Close(); cerr != nil {
		p.fail(e, errors.Wrap(cerr, "close file"))
		return
	}

	if err != nil {
		if !errors.Is(err, context.Canceled) { // 用户取消不算失败
			p.fail(e, err)
		}
		_ = os.Remove(e.to.Name()) // 尽力清理临时文件
		return
	}

	p.it.Finish(e.logicalPos)

	if perr := p.donePost(e); perr != nil {
		p.fail(e, errors.Wrap(perr, "post file"))
		return
	}

	p.task.onFileDone(e.info)
	p.emit(e, e.file.Size, e.file.Size, "done", nil)
}

// donePost 去除 .tmp 后缀、可选按 MIME 重写扩展名、还原消息时间。
func (p *progress) donePost(e *iterElem) error {
	newfile := strings.TrimSuffix(filepath.Base(e.to.Name()), tempExt)

	if p.rewriteExt {
		mime, err := mimetype.DetectFile(e.to.Name())
		if err != nil {
			return errors.Wrap(err, "detect mime")
		}
		if ext := mime.Extension(); ext != "" && filepath.Ext(newfile) != ext {
			newfile = fsutil.GetNameWithoutExt(newfile) + ext
		}
	}

	newpath := filepath.Join(filepath.Dir(e.to.Name()), newfile)
	if err := os.Rename(e.to.Name(), newpath); err != nil {
		return errors.Wrap(err, "rename file")
	}

	if e.file.Date > 0 {
		fileTime := time.Unix(e.file.Date, 0)
		if err := os.Chtimes(newpath, fileTime, fileTime); err != nil {
			return errors.Wrap(err, "set file time")
		}
	}

	return nil
}

func (p *progress) fail(e *iterElem, err error) {
	p.task.onFileFailed()
	p.emit(e, e.file.Size, 0, "failed", err)
}

func (p *progress) emit(e *iterElem, total, downloaded int64, state string, err error) {
	ev := FileEvent{
		TaskID:     p.task.ID,
		FileID:     e.id,
		Name:       strings.TrimSuffix(filepath.Base(e.to.Name()), tempExt),
		Total:      total,
		Downloaded: downloaded,
		State:      state,
	}
	if err != nil {
		ev.Error = err.Error()
	}
	p.task.emitFile(ev)
}
