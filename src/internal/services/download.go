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

// ListTaskFiles 返回任务内登记的文件列表。
func (s *DownloadService) ListTaskFiles(id string) ([]engine.TaskFile, error) {
	return s.manager.Files(id)
}

// ClearFinishedTasks 批量移除已完成任务记录（不删除文件）。
func (s *DownloadService) ClearFinishedTasks() error {
	s.manager.ClearFinished()
	return nil
}

// DeleteTaskFiles 删除任务内指定文件；若任务文件全部删除则同步移除记录。
func (s *DownloadService) DeleteTaskFiles(id string, paths []string) error {
	return s.manager.DeleteFiles(id, paths)
}

// DeleteAllFiles 停止未完成任务并删除全部登记文件与任务记录。
func (s *DownloadService) DeleteAllFiles() error {
	return s.manager.DeleteAllFiles()
}

// OpenTaskDir 在系统文件管理器中打开任务保存目录。
func (s *DownloadService) OpenTaskDir(id string) error {
	dir, err := s.manager.TaskDir(id)
	if err != nil {
		return err
	}
	return openDirectory(dir)
}

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
