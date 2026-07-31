// Package config 管理应用配置与数据目录。
// 配置以 YAML 形式持久化在用户数据目录（Windows 下为 %AppData%\tdl-ui\config.yaml）。
package config

import (
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-faster/errors"
	"gopkg.in/yaml.v2"

	"tdl-ui/internal/logging"
)

// Settings 应用全局设置（对前端与服务层暴露的扁平结构，仅保留 JSON 标签）。
type Settings struct {
	// Proxy 代理地址，如 socks5://127.0.0.1:1080 或 http://127.0.0.1:8080，空为直连
	Proxy string `json:"proxy"`
	// DownloadDir 默认下载目录
	DownloadDir string `json:"downloadDir"`
	// Template 默认文件命名模板（Go text/template，与 tdl 兼容）
	Template string `json:"template"`
	// Threads 单文件下载线程数
	Threads int `json:"threads"`
	// Limit 同时下载的最大文件数
	Limit int `json:"limit"`
	// PoolSize DC 连接池大小
	PoolSize int `json:"poolSize"`
	// Theme 主题：light / dark / system
	Theme string `json:"theme"`
	// CacheDir 缩略图/预览缓存目录，空则为 <DataDir>/cache
	CacheDir string `json:"cacheDir" yaml:"-"`
	// TempDir 在线视频边下边播的分段暂存目录，空则为 <DataDir>/tmp
	TempDir string `json:"tempDir" yaml:"-"`
	// LoggedInUserID 最近一次登录成功的用户 ID（仅用于界面展示）
	LoggedInUserID int64 `json:"loggedInUserId"`
	// LoggedInUsername 最近一次登录成功的用户名（仅用于界面展示）
	LoggedInUsername string `json:"loggedInUsername"`
	// ThumbWorkers 缩略图队列 worker 数，0 回退内置默认值 2（ARC-003，仅配置文件手改）
	ThumbWorkers int `json:"thumbWorkers"`
	// VideoWorkers 视频分段队列 worker 数，0 回退内置默认值 2（ARC-003，仅配置文件手改）
	VideoWorkers int `json:"videoWorkers"`
	// ProgressIntervalMs 单文件下载进度事件最小推送间隔（毫秒），0 回退内置默认值 200（ARC-003）
	ProgressIntervalMs int `json:"progressIntervalMs"`
	// RecentDirs 各用途最近使用目录历史（key＝download/logExport/cache/temp/logDir）
	RecentDirs map[string][]string `json:"recentDirs"`
	// Log 日志配置（输出目标、级别、目录、格式与滚动策略）
	Log logging.LogSettings `json:"log"`
}

// yamlConfig 是持久化用的分组结构，与扁平 Settings 互转。
type yamlConfig struct {
	App struct {
		Theme string `yaml:"theme"`
	} `yaml:"app"`
	Download struct {
		Dir      string `yaml:"dir"`
		Template string `yaml:"template"`
		Threads  int    `yaml:"threads"`
		Limit    int    `yaml:"limit"`
		PoolSize int    `yaml:"poolSize"`
	} `yaml:"download"`
	Storage struct {
		CacheDir string `yaml:"cacheDir"`
		TempDir  string `yaml:"tempDir"`
	} `yaml:"storage"`
	// Tuning 性能调优参数（无 UI 入口，零值回退内置默认，ARC-003）
	Tuning struct {
		ThumbWorkers       int `yaml:"thumbWorkers"`
		VideoWorkers       int `yaml:"videoWorkers"`
		ProgressIntervalMs int `yaml:"progressIntervalMs"`
	} `yaml:"tuning"`
	Log     logging.LogSettings `yaml:"log"`
	Session struct {
		Proxy            string `yaml:"proxy"`
		LoggedInUserID   int64  `yaml:"loggedInUserId"`
		LoggedInUsername string `yaml:"loggedInUsername"`
	} `yaml:"session"`
	RecentDirs map[string][]string `yaml:"recentDirs"`
}

func settingsToYAML(s Settings) yamlConfig {
	var y yamlConfig
	y.App.Theme = s.Theme
	y.Download.Dir = s.DownloadDir
	y.Download.Template = s.Template
	y.Download.Threads = s.Threads
	y.Download.Limit = s.Limit
	y.Download.PoolSize = s.PoolSize
	y.Storage.CacheDir = s.CacheDir
	y.Storage.TempDir = s.TempDir
	y.Tuning.ThumbWorkers = s.ThumbWorkers
	y.Tuning.VideoWorkers = s.VideoWorkers
	y.Tuning.ProgressIntervalMs = s.ProgressIntervalMs
	y.Log = s.Log
	y.Session.Proxy = s.Proxy
	y.Session.LoggedInUserID = s.LoggedInUserID
	y.Session.LoggedInUsername = s.LoggedInUsername
	y.RecentDirs = s.RecentDirs
	return y
}

func yamlToSettings(y yamlConfig) Settings {
	return Settings{
		Proxy:              y.Session.Proxy,
		DownloadDir:        y.Download.Dir,
		Template:           y.Download.Template,
		Threads:            y.Download.Threads,
		Limit:              y.Download.Limit,
		PoolSize:           y.Download.PoolSize,
		Theme:              y.App.Theme,
		CacheDir:           y.Storage.CacheDir,
		TempDir:            y.Storage.TempDir,
		LoggedInUserID:     y.Session.LoggedInUserID,
		LoggedInUsername:   y.Session.LoggedInUsername,
		ThumbWorkers:       y.Tuning.ThumbWorkers,
		VideoWorkers:       y.Tuning.VideoWorkers,
		ProgressIntervalMs: y.Tuning.ProgressIntervalMs,
		RecentDirs:         y.RecentDirs,
		Log:                y.Log,
	}
}

// DefaultTemplate 与 tdl CLI 默认命名模板保持一致。
const DefaultTemplate = "{{ .DialogID }}_{{ .MessageID }}_{{ filenamify .FileName }}"

// Manager 负责设置的加载与保存，并提供数据目录路径。
type Manager struct {
	mu       sync.RWMutex
	settings Settings
	dataDir  string
}

// NewManager 初始化数据目录并加载设置（YAML）。
// 迁移策略：config.yaml 存在则加载；否则若旧 settings.json 存在则读入并写出 config.yaml，
// 原文件重命名为 settings.json.bak；两者皆无则写出默认配置。
func NewManager() (*Manager, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return NewManagerAt(filepath.Join(base, "tdl-ui"))
}

// NewManagerAt 在指定数据目录初始化配置（测试与自定义部署入口）。
func NewManagerAt(dataDir string) (*Manager, error) {
	for _, dir := range []string{
		dataDir,
		filepath.Join(dataDir, "kv"),
		filepath.Join(dataDir, "scripts"),
		filepath.Join(dataDir, "cache"),
		filepath.Join(dataDir, "tmp"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	m := &Manager{dataDir: dataDir, settings: defaultSettings(dataDir)}
	if err := m.loadOrMigrate(); err != nil {
		return nil, err
	}
	return m, nil
}

func defaultSettings(dataDir string) Settings {
	// 默认下载目录：home 取失败时回落到数据目录下 downloads，
	// 避免生成相对路径导致落点不可预期（SVC-13）
	downloadDir := filepath.Join(dataDir, "downloads")
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		downloadDir = filepath.Join(home, "Downloads", "tdl-ui")
	}
	return Settings{
		Proxy:       "",
		DownloadDir: downloadDir,
		Template:    DefaultTemplate,
		Threads:     4,
		Limit:       2,
		PoolSize:    8,
		Theme:       "system",
		Log:         logging.DefaultSettings(),
	}
}

// DataDir 应用数据根目录。
func (m *Manager) DataDir() string { return m.dataDir }

// KVDir bolt 存储目录（Telegram 会话等）。
func (m *Manager) KVDir() string { return filepath.Join(m.dataDir, "kv") }

// ScriptDir 用户脚本目录。
func (m *Manager) ScriptDir() string { return filepath.Join(m.dataDir, "scripts") }

// CacheDir 缓存目录：设置为空时回落到 <DataDir>/cache。
func (m *Manager) CacheDir() string {
	m.mu.RLock()
	dir := m.settings.CacheDir
	m.mu.RUnlock()
	if dir == "" {
		return filepath.Join(m.dataDir, "cache")
	}
	return dir
}

// TempDir 视频临时分段目录：设置为空时回落到 <DataDir>/tmp。
func (m *Manager) TempDir() string {
	m.mu.RLock()
	dir := m.settings.TempDir
	m.mu.RUnlock()
	if dir == "" {
		return filepath.Join(m.dataDir, "tmp")
	}
	return dir
}

func (m *Manager) configPath() string     { return filepath.Join(m.dataDir, "config.yaml") }
func (m *Manager) legacyJSONPath() string { return filepath.Join(m.dataDir, "settings.json") }

// Get 返回当前设置副本。
func (m *Manager) Get() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings
}

// 参数上限（SVC-14）：防止前端传入夸张值拖垮连接池与 Telegram 限流。
const (
	maxThreads  = 16
	maxPoolSize = 64
)

// Update 保存新的设置。
func (m *Manager) Update(s Settings) error {
	// 空值兜底，避免前端传入非法配置
	if s.Template == "" {
		s.Template = DefaultTemplate
	}
	if s.Threads <= 0 {
		s.Threads = 4
	}
	if s.Limit <= 0 {
		s.Limit = 2
	}
	if s.PoolSize <= 0 {
		s.PoolSize = 8
	}
	// 上限钳制（SVC-14）
	if s.Threads > maxThreads {
		s.Threads = maxThreads
	}
	if s.PoolSize > maxPoolSize {
		s.PoolSize = maxPoolSize
	}
	s.Log = s.Log.WithDefaults()

	// Proxy 预校验（SVC-14）：非法值在保存时即报错，而非等到建连时才以晦涩错误暴露。
	if s.Proxy != "" {
		if err := validateProxy(s.Proxy); err != nil {
			return err
		}
	}

	// 非空缓存目录：校验可创建且可写，失败则阻断保存。
	if s.CacheDir != "" {
		if err := verifyWritableDir(s.CacheDir); err != nil {
			return err
		}
	}

	// 非空视频临时目录：同口径校验可创建且可写。
	if s.TempDir != "" {
		if err := verifyWritableDir(s.TempDir); err != nil {
			return err
		}
	}

	m.mu.Lock()
	m.settings = s
	m.mu.Unlock()
	return m.save()
}

// validateProxy 校验代理地址格式：scheme 白名单 + 非空主机名（SVC-14）。
func validateProxy(p string) error {
	u, err := url.Parse(p)
	if err != nil {
		return errors.Wrap(err, "代理地址无效")
	}
	switch u.Scheme {
	case "socks5", "http", "https":
	default:
		return errors.Errorf("代理协议仅支持 socks5/http/https，当前为 %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.Errorf("代理地址缺少主机名: %q", p)
	}
	return nil
}

// verifyWritableDir 确保目录可创建并可写（写入探针文件后删除）。
func verifyWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	probe := filepath.Join(dir, ".tdl-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return err
	}
	_ = os.Remove(probe)
	return nil
}

// loadOrMigrate 按迁移策略加载配置。
func (m *Manager) loadOrMigrate() error {
	// 1. config.yaml 已存在，直接加载。
	if b, err := os.ReadFile(m.configPath()); err == nil {
		return m.applyYAML(b)
	} else if !os.IsNotExist(err) {
		return err
	}

	// 2. 旧 settings.json 存在，读入 → 写出 config.yaml → 原文件重命名 .bak。
	if b, err := os.ReadFile(m.legacyJSONPath()); err == nil {
		if err := m.applyLegacyJSON(b); err != nil {
			return err
		}
		if err := m.save(); err != nil {
			return err
		}
		if err := os.Rename(m.legacyJSONPath(), m.legacyJSONPath()+".bak"); err != nil && !os.IsNotExist(err) {
			return err
		}
		logStore.Infof("已将旧 settings.json 迁移为 config.yaml")
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	// 3. 都不存在，写出默认配置。
	return m.save()
}

func (m *Manager) applyYAML(b []byte) error {
	var y yamlConfig
	if err := yaml.Unmarshal(b, &y); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings = yamlToSettings(y)
	m.settings.Log = m.settings.Log.WithDefaults()
	return nil
}

func (m *Manager) applyLegacyJSON(b []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := legacyJSONUnmarshal(b, &m.settings); err != nil {
		return err
	}
	m.settings.Log = m.settings.Log.WithDefaults()
	return nil
}

func (m *Manager) save() error {
	m.mu.RLock()
	y := settingsToYAML(m.settings)
	m.mu.RUnlock()
	b, err := yaml.Marshal(y)
	if err != nil {
		return err
	}
	return os.WriteFile(m.configPath(), b, 0o644)
}
