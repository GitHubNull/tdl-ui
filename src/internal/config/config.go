// Package config 管理应用配置与数据目录。
// 配置以 YAML 形式持久化在用户数据目录（Windows 下为 %AppData%\tdl-ui\config.yaml）。
package config

import (
	"os"
	"path/filepath"
	"sync"

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
	// LoggedInUserID 最近一次登录成功的用户 ID（仅用于界面展示）
	LoggedInUserID int64 `json:"loggedInUserId"`
	// LoggedInUsername 最近一次登录成功的用户名（仅用于界面展示）
	LoggedInUsername string `json:"loggedInUsername"`
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
	} `yaml:"storage"`
	Log     logging.LogSettings `yaml:"log"`
	Session struct {
		Proxy            string `yaml:"proxy"`
		LoggedInUserID   int64  `yaml:"loggedInUserId"`
		LoggedInUsername string `yaml:"loggedInUsername"`
	} `yaml:"session"`
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
	y.Log = s.Log
	y.Session.Proxy = s.Proxy
	y.Session.LoggedInUserID = s.LoggedInUserID
	y.Session.LoggedInUsername = s.LoggedInUsername
	return y
}

func yamlToSettings(y yamlConfig) Settings {
	return Settings{
		Proxy:            y.Session.Proxy,
		DownloadDir:      y.Download.Dir,
		Template:         y.Download.Template,
		Threads:          y.Download.Threads,
		Limit:            y.Download.Limit,
		PoolSize:         y.Download.PoolSize,
		Theme:            y.App.Theme,
		CacheDir:         y.Storage.CacheDir,
		LoggedInUserID:   y.Session.LoggedInUserID,
		LoggedInUsername: y.Session.LoggedInUsername,
		Log:              y.Log,
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
	dataDir := filepath.Join(base, "tdl-ui")

	for _, dir := range []string{
		dataDir,
		filepath.Join(dataDir, "kv"),
		filepath.Join(dataDir, "scripts"),
		filepath.Join(dataDir, "cache"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	m := &Manager{dataDir: dataDir, settings: defaultSettings()}
	if err := m.loadOrMigrate(); err != nil {
		return nil, err
	}
	return m, nil
}

func defaultSettings() Settings {
	home, _ := os.UserHomeDir()
	return Settings{
		Proxy:       "",
		DownloadDir: filepath.Join(home, "Downloads", "tdl-ui"),
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

func (m *Manager) configPath() string     { return filepath.Join(m.dataDir, "config.yaml") }
func (m *Manager) legacyJSONPath() string { return filepath.Join(m.dataDir, "settings.json") }

// Get 返回当前设置副本。
func (m *Manager) Get() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings
}

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
	s.Log = s.Log.WithDefaults()

	// 非空缓存目录：校验可创建且可写，失败则阻断保存。
	if s.CacheDir != "" {
		if err := verifyWritableDir(s.CacheDir); err != nil {
			return err
		}
	}

	m.mu.Lock()
	m.settings = s
	m.mu.Unlock()
	return m.save()
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
