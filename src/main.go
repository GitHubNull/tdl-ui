// tdl-ui：基于 Wails v2 的 tdl 桌面 GUI。
// 核心下载能力复用 ref/tdl 子模块（github.com/iyear/tdl，AGPL-3.0）。
package main

import (
	"context"
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/iyear/tdl/pkg/kv"

	"tdl-ui/internal/config"
	"tdl-ui/internal/engine"
	"tdl-ui/internal/events"
	"tdl-ui/internal/script"
	"tdl-ui/internal/scriptapi"
	"tdl-ui/internal/services"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	cfg, err := config.NewManager()
	if err != nil {
		log.Fatalf("初始化配置失败: %v", err)
	}

	// bolt kv 全局唯一实例，登录与下载共享（bbolt 文件锁不允许重复打开）
	kvs, err := kv.New(kv.DriverBolt, map[string]any{"path": cfg.KVDir()})
	if err != nil {
		log.Fatalf("初始化存储失败: %v", err)
	}

	emitter := &events.Emitter{}

	// 脚本日志转发到前端
	scriptapi.SetLogSink(func(msg string) {
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
	// 登出后关闭对话服务常驻连接（会话已失效，下次查询自动重建）
	authSvc.OnLogout = chatSvc.Stop

	err = wails.Run(&options.App{
		Title:     "tdl UI",
		Width:     1200,
		Height:    800,
		MinWidth:  960,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 250, G: 250, B: 251, A: 1},
		OnStartup: func(ctx context.Context) {
			emitter.Bind(ctx)
		},
		OnShutdown: func(ctx context.Context) {
			_ = kvs.Close()
		},
		Bind: []interface{}{
			authSvc,
			downloadSvc,
			scriptSvc,
			settingsSvc,
			chatSvc,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
	if err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}
