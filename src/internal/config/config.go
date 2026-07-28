// Package config 管理应用配置与数据目录。
// 配置以 JSON 形式持久化在用户数据目录（Windows 下为 %AppData%\tdl-ui）。
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Settings 应用全局设置。
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
	// LoggedInUserID 最近一次登录成功的用户 ID（仅用于界面展示）
	LoggedInUserID int64 `json:"loggedInUserId"`
	// LoggedInUsername 最近一次登录成功的用户名（仅用于界面展示）
	LoggedInUsername string `json:"loggedInUsername"`
}

// DefaultTemplate 与 tdl CLI 默认命名模板保持一致。
const DefaultTemplate = "{{ .DialogID }}_{{ .MessageID }}_{{ filenamify .FileName }}"

// Manager 负责设置的加载与保存，并提供数据目录路径。
type Manager struct {
	mu       sync.RWMutex
	settings Settings
	dataDir  string
}

// NewManager 初始化数据目录并加载设置。
func NewManager() (*Manager, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dataDir := filepath.Join(base, "tdl-ui")

	for _, dir := range []string{dataDir, filepath.Join(dataDir, "kv"), filepath.Join(dataDir, "scripts")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	m := &Manager{dataDir: dataDir, settings: defaultSettings()}
	if err := m.load(); err != nil && !os.IsNotExist(err) {
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
	}
}

// DataDir 应用数据根目录。
func (m *Manager) DataDir() string { return m.dataDir }

// KVDir bolt 存储目录（Telegram 会话等）。
func (m *Manager) KVDir() string { return filepath.Join(m.dataDir, "kv") }

// ScriptDir 用户脚本目录。
func (m *Manager) ScriptDir() string { return filepath.Join(m.dataDir, "scripts") }

func (m *Manager) settingsPath() string { return filepath.Join(m.dataDir, "settings.json") }

// Get 返回当前设置副本。
func (m *Manager) Get() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings
}

// Update 保存新的设置。
func (m *Manager) Update(s Settings) error {
	m.mu.Lock()
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
	m.settings = s
	m.mu.Unlock()
	return m.save()
}

func (m *Manager) load() error {
	b, err := os.ReadFile(m.settingsPath())
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return json.Unmarshal(b, &m.settings)
}

func (m *Manager) save() error {
	m.mu.RLock()
	b, err := json.MarshalIndent(m.settings, "", "  ")
	m.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(m.settingsPath(), b, 0o644)
}
