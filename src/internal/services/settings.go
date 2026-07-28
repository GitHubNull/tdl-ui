package services

import (
	"tdl-ui/internal/config"
)

// SettingsService 应用设置读写。
type SettingsService struct {
	cfg *config.Manager
}

// NewSettingsService 创建设置服务。
func NewSettingsService(cfg *config.Manager) *SettingsService {
	return &SettingsService{cfg: cfg}
}

// Get 获取当前设置。
func (s *SettingsService) Get() config.Settings { return s.cfg.Get() }

// Save 保存设置。
func (s *SettingsService) Save(settings config.Settings) error {
	return s.cfg.Update(settings)
}

// DataDir 返回应用数据目录（用于界面展示）。
func (s *SettingsService) DataDir() string { return s.cfg.DataDir() }
