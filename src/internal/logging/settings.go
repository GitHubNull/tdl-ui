// Package logging 基于 zap + lumberjack 的应用日志核心：
// 文件滚动输出与前端事件推送双目标，格式模板可配置。
package logging

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

// LogSettings 日志配置，随应用设置持久化，也可由 YAML 文件导入。
type LogSettings struct {
	// Targets 输出目标：file / ui / both
	Targets string `json:"targets" yaml:"targets"`
	// Level 最低级别：debug / info / warn / error
	Level string `json:"level" yaml:"level"`
	// Dir 日志目录，空则为程序目录下 logs 子目录
	Dir string `json:"dir" yaml:"dir"`
	// Format 格式模板，支持占位符 {datetime} {level} {module} {file} {func} {line} {msg}
	Format string `json:"format" yaml:"format"`
	// MaxSizeMB 单文件上限（MB），超过后滚动
	MaxSizeMB int `json:"maxSizeMb" yaml:"maxSizeMb"`
	// MaxAgeDays 滚动备份保留天数
	MaxAgeDays int `json:"maxAgeDays" yaml:"maxAgeDays"`
	// MaxBackups 滚动备份保留个数
	MaxBackups int `json:"maxBackups" yaml:"maxBackups"`
}

const (
	// DefaultFormat 默认格式模板。
	DefaultFormat = "{datetime} [{level}] - [{module}] - [{file}] - [{func}] - [{line}] - {msg}"
	// TimeLayout 时间格式，秒后 5 位小数（如 2026-07-28 21:59:00.12345）。
	TimeLayout = "2006-01-02 15:04:05.00000"
	// FileName 当前日志文件名，滚动备份由 lumberjack 自动命名。
	FileName = "tdl-ui.log"
)

// DefaultDir 默认日志目录：可执行文件所在目录下的 logs 子目录。
func DefaultDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "logs"
	}
	return filepath.Join(filepath.Dir(exe), "logs")
}

// DefaultSettings 返回全默认配置。
func DefaultSettings() LogSettings {
	return LogSettings{}.WithDefaults()
}

// ensureWritableDir 确保目录可创建并可写（写入探针文件后删除，LOG-03）。
func ensureWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	probe := filepath.Join(dir, ".tdl-ui-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return err
	}
	_ = os.Remove(probe)
	return nil
}

// fallbackLogDir 默认目录不可写时的回退位置：用户缓存目录（Windows 为 %LOCALAPPDATA%）。
func fallbackLogDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "tdl-ui", "logs")
}

// WithDefaults 缺省/非法字段回填默认值，返回规范化副本。
func (s LogSettings) WithDefaults() LogSettings {
	switch s.Targets {
	case "file", "ui", "both":
	default:
		s.Targets = "both"
	}
	switch strings.ToLower(s.Level) {
	case "debug", "info", "warn", "error":
		s.Level = strings.ToLower(s.Level)
	default:
		s.Level = "info"
	}
	if s.Dir == "" {
		s.Dir = DefaultDir()
	}
	if s.Format == "" {
		s.Format = DefaultFormat
	}
	if s.MaxSizeMB <= 0 {
		s.MaxSizeMB = 10
	}
	if s.MaxAgeDays <= 0 {
		s.MaxAgeDays = 7
	}
	if s.MaxBackups <= 0 {
		s.MaxBackups = 10
	}
	return s
}

// FileToUI 目标开关便捷判断。
func (s LogSettings) fileEnabled() bool { return s.Targets == "file" || s.Targets == "both" }
func (s LogSettings) uiEnabled() bool   { return s.Targets == "ui" || s.Targets == "both" }

// ParseYAML 解析 YAML 配置，缺省字段用默认值补齐。
func ParseYAML(b []byte) (LogSettings, error) {
	var s LogSettings
	if err := yaml.Unmarshal(b, &s); err != nil {
		return LogSettings{}, err
	}
	return s.WithDefaults(), nil
}

// ToYAML 序列化为 YAML（含全部字段，可作导入模板）。
func (s LogSettings) ToYAML() ([]byte, error) {
	return yaml.Marshal(s.WithDefaults())
}
