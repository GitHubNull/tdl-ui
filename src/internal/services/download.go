package services

import (
	"github.com/go-faster/errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tdl-ui/internal/config"
	"tdl-ui/internal/engine"
	"tdl-ui/internal/events"
)

// DownloadService 下载任务管理。
type DownloadService struct {
	cfg     *config.Manager
	manager *engine.Manager
	emitter *events.Emitter
}

// NewDownloadService 创建下载服务。
func NewDownloadService(cfg *config.Manager, manager *engine.Manager, emitter *events.Emitter) *DownloadService {
	return &DownloadService{cfg: cfg, manager: manager, emitter: emitter}
}

// CreateTask 创建并启动下载任务，返回任务 ID。
func (s *DownloadService) CreateTask(opts engine.TaskOptions) (string, error) {
	return s.manager.Create(opts)
}

// ListTasks 返回全部任务（倒序）。
func (s *DownloadService) ListTasks() []engine.TaskView {
	return s.manager.List()
}

// PauseTask 暂停任务，进度保留。
func (s *DownloadService) PauseTask(id string) error { return s.manager.Pause(id) }

// ResumeTask 恢复任务（断点续传）。
func (s *DownloadService) ResumeTask(id string) error { return s.manager.Resume(id) }

// CancelTask 取消任务。
func (s *DownloadService) CancelTask(id string) error { return s.manager.Cancel(id) }

// RemoveTask 移除终态任务。
func (s *DownloadService) RemoveTask(id string) error { return s.manager.Remove(id) }

// SelectDirectory 弹出系统目录选择框，返回所选目录（取消时为空串）。
func (s *DownloadService) SelectDirectory() (string, error) {
	ctx := s.emitter.Ctx()
	if ctx == nil {
		return "", errors.New("应用尚未就绪")
	}
	return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
		Title:            "选择下载目录",
		DefaultDirectory: s.cfg.Get().DownloadDir,
	})
}
