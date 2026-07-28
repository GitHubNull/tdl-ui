// tdl-ui：基于 Wails v2 的 tdl 桌面 GUI。
// 核心下载能力复用 ref/tdl 子模块（github.com/iyear/tdl，AGPL-3.0）。
package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/iyear/tdl/pkg/kv"

	"tdl-ui/internal/config"
	"tdl-ui/internal/engine"
	"tdl-ui/internal/events"
	"tdl-ui/internal/logging"
	"tdl-ui/internal/script"
	"tdl-ui/internal/scriptapi"
	"tdl-ui/internal/services"
)

//go:embed all:frontend/dist
var assets embed.FS

var logApp = logging.L("app")

func main() {
	cfg, err := config.NewManager()
	if err != nil {
		log.Fatalf("初始化配置失败: %v", err)
	}

	emitter := &events.Emitter{}

	// 日志初始化：数据目录下 logging.yaml 存在时优先生效（启动时自动加载）
	logCfg := cfg.Get().Log
	if b, err := os.ReadFile(filepath.Join(cfg.DataDir(), "logging.yaml")); err == nil {
		if parsed, perr := logging.ParseYAML(b); perr == nil {
			logCfg = parsed
			settings := cfg.Get()
			settings.Log = parsed
			_ = cfg.Update(settings)
		}
	}
	logging.Init(logCfg, func(batch []logging.LogEntry) {
		emitter.Emit(events.Log, batch)
	})
	logApp.Infof("应用启动，数据目录: %s，日志目录: %s", cfg.DataDir(), logging.CurrentDir())

	// bolt kv 全局唯一实例，登录与下载共享（bbolt 文件锁不允许重复打开）
	kvs, err := kv.New(kv.DriverBolt, map[string]any{"path": cfg.KVDir()})
	if err != nil {
		logApp.Errorf("初始化存储失败: %v", err)
		logging.Close()
		log.Fatalf("初始化存储失败: %v", err)
	}
	logApp.Infof("bolt 存储就绪: %s", cfg.KVDir())

	// 脚本日志转发到前端，同时写入日志系统
	scriptLogger := logging.L("script")
	scriptapi.SetLogSink(func(msg string) {
		scriptLogger.Infof("%s", strings.TrimRight(msg, "\n"))
		emitter.Emit(events.ScriptLog, msg)
	})

	scriptStore := script.NewStore(cfg.ScriptDir())
	taskManager := engine.NewManager(engine.Deps{
		Cfg:     cfg,
		KV:      kvs,
		Emitter: emitter,
		Scripts: scriptStore,
	})

	authSvc := services.NewAuthService(cfg, kvs, emitter)
	downloadSvc := services.NewDownloadService(cfg, taskManager, emitter)
	scriptSvc := services.NewScriptService(scriptStore)
	settingsSvc := services.NewSettingsService(cfg)
	chatSvc := services.NewChatService(cfg, kvs)
	logSvc := services.NewLogService(cfg, emitter)
	// 登出后关闭对话服务常驻连接（会话已失效，下次查询自动重建）
	authSvc.OnLogout = chatSvc.Stop

	err = wails.Run(&options.App{
		Title:     "tdl UI",
		Width:     1200,
		Height:    800,
		MinWidth:  960,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: services.NewMediaHandler(chatSvc), // /media/thumb 与 /media/preview
		},
		BackgroundColour: &options.RGBA{R: 250, G: 250, B: 251, A: 1},
		OnStartup: func(ctx context.Context) {
			emitter.Bind(ctx)
			logApp.Infof("Wails 运行时就绪，窗口已启动")
		},
		OnShutdown: func(ctx context.Context) {
			logApp.Infof("应用退出，关闭存储与日志")
			_ = kvs.Close()
			logging.Close()
		},
		Bind: []interface{}{
			authSvc,
			downloadSvc,
			scriptSvc,
			settingsSvc,
			chatSvc,
			logSvc,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
	if err != nil {
		logApp.Errorf("启动失败: %v", err)
		logging.Close()
		log.Fatalf("启动失败: %v", err)
	}
}
