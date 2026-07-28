package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// 包级全局：root 在包初始化时即可用，业务包可安全地在 var 声明中调用 L()。
// Init 前日志仅进入环形缓冲，Init 后接通文件与前端推送。
var (
	level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	gsink = newSink(level)
	root  = zap.New(gsink, zap.AddCaller())
)

// Init 应用启动时初始化：绑定前端推送回调并应用配置。
func Init(s LogSettings, emit func([]LogEntry)) {
	gsink.setEmit(emit)
	Reconfigure(s)
}

// Reconfigure 热更新日志配置（级别、目标、目录、格式、滚动策略）。
func Reconfigure(s LogSettings) {
	s = s.WithDefaults()
	level.SetLevel(parseLevel(s.Level))
	gsink.reconfigure(s)
}

// Close 应用退出时 flush 并关闭文件 writer。
func Close() {
	gsink.close()
}

// L 返回带模块名的 SugaredLogger（zap 标准 API：Debugf/Infof/Warnf/Errorf）。
func L(module string) *zap.SugaredLogger {
	return root.Named(module).Sugar()
}

// Recent 返回内存环形缓冲快照（日志页初始加载）。
func Recent() []LogEntry {
	return gsink.recent()
}

// CurrentDir 当前生效的日志目录。
func CurrentDir() string {
	gsink.mu.Lock()
	defer gsink.mu.Unlock()
	return gsink.settings.Dir
}

func parseLevel(s string) zapcore.Level {
	switch s {
	case "debug":
		return zapcore.DebugLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
