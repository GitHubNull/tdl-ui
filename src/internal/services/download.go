package services

import (
	"os"

	"github.com/go-faster/errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tdl-ui/internal/config"
	"tdl-ui/internal/engine"
	"tdl-ui/internal/events"
	"tdl-ui/internal/logging"
)

var logDownload = logging.L("download")

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
	logDownload.Infof("创建下载任务: 链接数=%d 选集数=%d 目录=%s 脚本=%s", len(opts.URLs), len(opts.Selections), opts.Dir, opts.ScriptName)
	id, err := s.manager.Create(opts)
	if err != nil {
		logDownload.Errorf("创建下载任务失败: %v", err)
		return "", err
	}
	logDownload.Infof("下载任务已创建: id=%s", id)
	return id, nil
}

// ListTasks 返回全部任务（倒序）。
func (s *DownloadService) ListTasks() []engine.TaskView {
	return s.manager.List()
}

// PauseTask 暂停任务，进度保留。
func (s *DownloadService) PauseTask(id string) error {
	logDownload.Infof("暂停任务: id=%s", id)
	return s.manager.Pause(id)
}

// ResumeTask 恢复任务（断点续传）。
func (s *DownloadService) ResumeTask(id string) error {
	logDownload.Infof("恢复任务: id=%s", id)
	return s.manager.Resume(id)
}

// CancelTask 取消任务。
func (s *DownloadService) CancelTask(id string) error {
	logDownload.Infof("取消任务: id=%s", id)
	return s.manager.Cancel(id)
}

// RemoveTask 移除终态任务。
func (s *DownloadService) RemoveTask(id string) error {
	logDownload.Infof("移除任务记录: id=%s", id)
	return s.manager.Remove(id)
}

// ListTaskFiles 返回任务内登记的文件列表。
func (s *DownloadService) ListTaskFiles(id string) ([]engine.TaskFile, error) {
	return s.manager.Files(id)
}

// ClearFinishedTasks 批量移除已完成任务记录（不删除文件）。
func (s *DownloadService) ClearFinishedTasks() error {
	logDownload.Infof("清除已完成任务记录")
	s.manager.ClearFinished()
	return nil
}

// DeleteTaskFiles 删除任务内指定文件；若任务文件全部删除则同步移除记录。
func (s *DownloadService) DeleteTaskFiles(id string, paths []string) error {
	logDownload.Infof("删除任务文件: id=%s 文件数=%d", id, len(paths))
	if err := s.manager.DeleteFiles(id, paths); err != nil {
		logDownload.Errorf("删除任务文件失败: id=%s err=%v", id, err)
		return err
	}
	return nil
}

// DeleteAllFiles 停止未完成任务并删除全部登记文件与任务记录。
func (s *DownloadService) DeleteAllFiles() error {
	logDownload.Warnf("删除全部下载文件与任务记录")
	if err := s.manager.DeleteAllFiles(); err != nil {
		logDownload.Errorf("删除全部文件失败: %v", err)
		return err
	}
	return nil
}

// OpenTaskDir 在系统文件管理器中打开任务保存目录。
func (s *DownloadService) OpenTaskDir(id string) error {
	dir, err := s.manager.TaskDir(id)
	if err != nil {
		return err
	}
	return openDirectory(dir)
}

// SelectDirectory 弹出系统目录选择框，kind 区分用途标题，currentDir 为默认起始目录。
// 取消时返回空串。
func (s *DownloadService) SelectDirectory(kind string, currentDir string) (string, error) {
	ctx := s.emitter.Ctx()
	if ctx == nil {
		return "", errors.New("应用尚未就绪")
	}
	title := "选择目录"
	switch kind {
	case "download":
		title = "选择下载目录"
	case "cache":
		title = "选择缓存目录"
	case "temp":
		title = "选择临时目录"
	case "logDir":
		title = "选择日志目录"
	case "logExport":
		title = "选择导出目录"
	}
	defaultDir := s.cfg.Get().DownloadDir
	if currentDir != "" {
		defaultDir = currentDir
	} else {
		recent := s.cfg.RecentDirs(kind)
		if len(recent) > 0 {
			defaultDir = recent[0]
		}
	}
	picked, err := runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
		Title:            title,
		DefaultDirectory: defaultDir,
	})
	if err != nil {
		return "", err
	}
	if picked != "" {
		if _, aerr := s.cfg.AddRecentDir(kind, picked); aerr != nil {
			logDownload.Warnf("记录最近目录失败: kind=%s dir=%s err=%v", kind, picked, aerr)
		}
	}
	return picked, nil
}

// DownloadedFile 对话内已完成下载的消息文件（媒体页"已下载"标记数据源）。
type DownloadedFile struct {
	MessageID int    `json:"messageId"`
	Path      string `json:"path"`
	Size      int64  `json:"size"`
}

// ListDownloadedMessages 返回对话内已下载完成且磁盘文件仍存在的消息列表；
// 同一消息多次下载时保留最新记录。
func (s *DownloadService) ListDownloadedMessages(dialogID int64) ([]DownloadedFile, error) {
	files, err := s.manager.DownloadedFiles(dialogID)
	if err != nil {
		logDownload.Errorf("查询已下载文件失败: dialog=%d err=%v", dialogID, err)
		return nil, err
	}
	// 按 updated_at 升序遍历，后写覆盖即同 messageId 取最新；磁盘已删的陈旧记录不计入。
	latest := make(map[int]DownloadedFile, len(files))
	for _, f := range files {
		if st, err := os.Stat(f.Path); err != nil || st.IsDir() {
			continue
		}
		latest[f.MessageID] = DownloadedFile{MessageID: f.MessageID, Path: f.Path, Size: f.Size}
	}
	out := make([]DownloadedFile, 0, len(latest))
	for _, f := range latest {
		out = append(out, f)
	}
	return out, nil
}

// OpenDownloadedFile 用系统默认程序打开指定消息已下载的文件（双击打开入口）。
func (s *DownloadService) OpenDownloadedFile(dialogID int64, messageID int) error {
	f, ok, err := s.manager.DownloadedFile(dialogID, messageID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("该文件尚未下载或已被删除")
	}
	logDownload.Infof("打开已下载文件: dialog=%d msg=%d path=%s", dialogID, messageID, f.Path)
	if err := openFile(f.Path); err != nil {
		logDownload.Errorf("打开文件失败: path=%s err=%v", f.Path, err)
		return errors.New("该文件尚未下载或已被删除")
	}
	return nil
}

// RedownloadFile 重新下载任务中的单个文件。
func (s *DownloadService) RedownloadFile(taskID string, filePath string) error {
	logDownload.Infof("重新下载文件: task=%s path=%s", taskID, filePath)
	if err := s.manager.RedownloadFile(taskID, filePath); err != nil {
		logDownload.Errorf("重新下载文件失败: task=%s path=%s err=%v", taskID, filePath, err)
		return err
	}
	return nil
}

// DeleteFileRecord 删除单个文件记录（不删除磁盘文件）。
func (s *DownloadService) DeleteFileRecord(taskID string, filePath string) error {
	logDownload.Infof("删除文件记录: task=%s path=%s", taskID, filePath)
	if err := s.manager.DeleteFileRecord(taskID, filePath); err != nil {
		logDownload.Errorf("删除文件记录失败: task=%s path=%s err=%v", taskID, filePath, err)
		return err
	}
	return nil
}

// RevealFileInDir 在系统文件管理器中打开目录并选中指定文件。
func (s *DownloadService) RevealFileInDir(filePath string) error {
	logDownload.Infof("定位文件: path=%s", filePath)
	if err := revealFile(filePath); err != nil {
		logDownload.Errorf("定位文件失败: path=%s err=%v", filePath, err)
		return err
	}
	return nil
}

// ResumeTaskWithPending 恢复包含未完成文件的任务（继续下载未完成的部分）。
func (s *DownloadService) ResumeTaskWithPending(id string) error {
	logDownload.Infof("继续下载未完成文件: id=%s", id)
	if err := s.manager.ResumeTaskWithPending(id); err != nil {
		logDownload.Errorf("继续下载失败: id=%s err=%v", id, err)
		return err
	}
	return nil
}
