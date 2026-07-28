package services

import (
	"os"
	"path/filepath"

	"tdl-ui/internal/config"
	"tdl-ui/internal/logging"
)

var logSettings = logging.L("settings")

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

// Save 保存设置，日志配置立即热生效。
func (s *SettingsService) Save(settings config.Settings) error {
	if err := s.cfg.Update(settings); err != nil {
		logSettings.Errorf("保存设置失败: %v", err)
		return err
	}
	logging.Reconfigure(s.cfg.Get().Log)
	logSettings.Infof("设置已保存，日志配置已热更新（级别=%s 目标=%s）", s.cfg.Get().Log.Level, s.cfg.Get().Log.Targets)
	return nil
}

// DataDir 返回应用数据目录（用于界面展示）。
func (s *SettingsService) DataDir() string { return s.cfg.DataDir() }

// ClearCache 清空缓存目录下的 thumbs/、previews/ 两个子目录后重建（不递归删根目录）。
func (s *SettingsService) ClearCache() error {
	root := s.cfg.CacheDir()
	for _, kind := range []string{cacheKindThumb, cacheKindPreview} {
		sub := filepath.Join(root, kind)
		if err := os.RemoveAll(sub); err != nil {
			logSettings.Errorf("清空缓存子目录失败 %s: %v", sub, err)
			return err
		}
		if err := os.MkdirAll(sub, 0o755); err != nil {
			return err
		}
	}
	logSettings.Infof("缓存已清空: %s", root)
	return nil
}
