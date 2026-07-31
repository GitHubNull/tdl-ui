package services

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"

	"github.com/go-faster/errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tdl-ui/internal/config"
	"tdl-ui/internal/events"
	"tdl-ui/internal/logging"
)

var logLog = logging.L("log")

// defaultLogTailLines 读取日志文件尾部行数默认值（SVC-17）。
const defaultLogTailLines = 5000

// LogFileInfo 日志目录下的文件信息（供日志页历史文件列表）。
type LogFileInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"` // unix 秒
}

// LogService 日志浏览与配置服务。
type LogService struct {
	cfg     *config.Manager
	emitter *events.Emitter
}

// NewLogService 创建日志服务。
func NewLogService(cfg *config.Manager, emitter *events.Emitter) *LogService {
	return &LogService{cfg: cfg, emitter: emitter}
}

// GetRecent 返回内存环形缓冲快照（日志页初始加载）。
func (s *LogService) GetRecent() []logging.LogEntry {
	return logging.Recent()
}

// ListLogFiles 列出日志目录下的日志文件，按修改时间倒序。
func (s *LogService) ListLogFiles() ([]LogFileInfo, error) {
	dir := logging.CurrentDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []LogFileInfo{}, nil
		}
		logLog.Errorf("读取日志目录失败: %v", err)
		return nil, err
	}
	out := make([]LogFileInfo, 0, len(entries))
	for _, e := range entries {
		// SVC-12：按扩展名精确匹配，避免 Contains 误匹配 foo.log.tmp 等
		if e.IsDir() || filepath.Ext(e.Name()) != ".log" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, LogFileInfo{Name: e.Name(), Size: info.Size(), ModTime: info.ModTime().Unix()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime > out[j].ModTime })
	return out, nil
}

// ReadLogFile 读取指定日志文件尾部 maxLines 行（maxLines<=0 时默认 5000）。
// 仅允许日志目录内的文件名，防目录穿越。
func (s *LogService) ReadLogFile(name string, maxLines int) ([]string, error) {
	if name == "" || name != filepath.Base(name) {
		return nil, errors.New("非法文件名")
	}
	if maxLines <= 0 {
		maxLines = defaultLogTailLines
	}
	path := filepath.Join(logging.CurrentDir(), name)
	f, err := os.Open(path)
	if err != nil {
		logLog.Errorf("打开日志文件失败: %s: %v", name, err)
		return nil, err
	}
	defer f.Close()

	// 顺序扫描保留尾部 N 行（日志文件上限受滚动配置约束，可接受）
	lines := make([]string, 0, 1024)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > maxLines {
			lines = lines[1:]
		}
	}
	if err := sc.Err(); err != nil {
		logLog.Errorf("读取日志文件失败: %s: %v", name, err)
		return nil, err
	}
	logLog.Debugf("读取日志文件 %s，返回 %d 行", name, len(lines))
	return lines, nil
}

// ImportYAMLConfig 弹窗选择 YAML 文件导入日志配置，保存并热生效，返回新配置。
func (s *LogService) ImportYAMLConfig() (logging.LogSettings, error) {
	ctx := s.emitter.Ctx()
	if ctx == nil {
		return logging.LogSettings{}, errors.New("应用尚未就绪")
	}
	path, err := runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title: "导入日志 YAML 配置",
		Filters: []runtime.FileFilter{
			{DisplayName: "YAML 配置 (*.yaml;*.yml)", Pattern: "*.yaml;*.yml"},
		},
	})
	if err != nil {
		return logging.LogSettings{}, err
	}
	if path == "" { // 用户取消
		return logging.LogSettings{}, errors.New("已取消")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		logLog.Errorf("读取 YAML 配置失败: %v", err)
		return logging.LogSettings{}, err
	}
	parsed, err := logging.ParseYAML(b)
	if err != nil {
		logLog.Errorf("解析 YAML 配置失败: %v", err)
		return logging.LogSettings{}, errors.Wrap(err, "YAML 解析失败")
	}
	settings := s.cfg.Get()
	settings.Log = parsed
	if err := s.cfg.Update(settings); err != nil {
		return logging.LogSettings{}, err
	}
	logging.Reconfigure(parsed)
	logLog.Infof("已从 %s 导入日志配置并生效", filepath.Base(path))
	return parsed, nil
}

// ExportYAMLConfig 弹窗选择保存位置，导出当前日志配置 YAML。
func (s *LogService) ExportYAMLConfig() error {
	ctx := s.emitter.Ctx()
	if ctx == nil {
		return errors.New("应用尚未就绪")
	}
	path, err := runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
		Title:           "导出日志 YAML 配置",
		DefaultFilename: "logging.yaml",
		Filters: []runtime.FileFilter{
			{DisplayName: "YAML 配置 (*.yaml;*.yml)", Pattern: "*.yaml;*.yml"},
		},
	})
	if err != nil {
		return err
	}
	if path == "" { // 用户取消
		return nil
	}
	b, err := s.cfg.Get().Log.ToYAML()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		logLog.Errorf("导出 YAML 配置失败: %v", err)
		return err
	}
	logLog.Infof("日志配置已导出到 %s", path)
	return nil
}

// OpenLogDir 在系统文件管理器中打开日志目录。
func (s *LogService) OpenLogDir() error {
	dir := logging.CurrentDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return openDirectory(dir)
}

// ExportLogs 将日志内容写入指定目录下的文件，返回完整路径。
// 仅允许 .log / .txt / .csv 扩展名。
func (s *LogService) ExportLogs(dir, name, content string) (string, error) {
	if dir == "" || name == "" {
		return "", errors.New("导出目录和文件名不能为空")
	}
	// 安全检查：name 不能包含路径分隔符，防止目录穿越
	if name != filepath.Base(name) {
		return "", errors.New("非法文件名")
	}
	// 扩展名白名单
	ext := filepath.Ext(name)
	switch ext {
	case ".log", ".txt", ".csv":
	default:
		return "", errors.Errorf("不支持的文件格式: %s，仅支持 .log / .txt / .csv", ext)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logLog.Errorf("创建导出目录失败: %s: %v", dir, err)
		return "", err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		logLog.Errorf("导出日志失败: %s: %v", path, err)
		return "", err
	}
	logLog.Infof("日志已导出: %s (%d 字节)", path, len(content))
	return path, nil
}
