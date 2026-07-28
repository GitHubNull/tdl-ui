package logging

import (
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogEntry 单条日志的结构化视图，随 log:batch 事件推送给前端。
type LogEntry struct {
	// Seq 全局递增序号，前端行号栏使用
	Seq    uint64 `json:"seq"`
	Time   string `json:"time"`
	Level  string `json:"level"`
	Module string `json:"module"`
	File   string `json:"file"`
	Func   string `json:"func"`
	Line   int    `json:"line"`
	Msg    string `json:"msg"`
	// Text 按格式模板渲染后的整行文本（与文件内容一致）
	Text string `json:"text"`
}

// ringCap UI 环形缓冲上限；flushInterval 前端批量推送周期。
const (
	ringCap       = 5000
	flushInterval = 250 * time.Millisecond
)

// sink 自定义 zapcore.Core：格式模板渲染后写入 lumberjack 文件，
// 并维护内存环形缓冲 + 批量推送前端。zap 负责级别门控、调用方捕获与并发入口。
type sink struct {
	mu       sync.Mutex
	settings LogSettings
	file     *lumberjack.Logger
	emit     func([]LogEntry)
	ring     []LogEntry
	pending  []LogEntry
	seq      uint64
	enab     zapcore.LevelEnabler
	stopCh   chan struct{}
	stopOnce sync.Once
}

func newSink(enab zapcore.LevelEnabler) *sink {
	s := &sink{
		settings: DefaultSettings(),
		ring:     make([]LogEntry, 0, 256),
		enab:     enab,
		stopCh:   make(chan struct{}),
	}
	go s.flushLoop()
	return s
}

// reconfigure 热更新配置：替换文件 writer 与格式模板。
func (s *sink) reconfigure(cfg LogSettings) {
	cfg = cfg.WithDefaults()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
	s.settings = cfg
	if cfg.fileEnabled() {
		s.file = &lumberjack.Logger{
			Filename:   filepath.Join(cfg.Dir, FileName),
			MaxSize:    cfg.MaxSizeMB,
			MaxAge:     cfg.MaxAgeDays,
			MaxBackups: cfg.MaxBackups,
			LocalTime:  true,
		}
	}
}

func (s *sink) setEmit(emit func([]LogEntry)) {
	s.mu.Lock()
	s.emit = emit
	s.mu.Unlock()
}

// recent 返回环形缓冲快照。
func (s *sink) recent() []LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]LogEntry, len(s.ring))
	copy(out, s.ring)
	return out
}

// ---- zapcore.Core 接口实现 ----

func (s *sink) Enabled(lvl zapcore.Level) bool { return s.enab.Enabled(lvl) }

func (s *sink) With([]zapcore.Field) zapcore.Core { return s }

func (s *sink) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if s.Enabled(ent.Level) {
		return ce.AddCore(ent, s)
	}
	return ce
}

func (s *sink) Write(ent zapcore.Entry, _ []zapcore.Field) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	e := LogEntry{
		Seq:    s.seq,
		Time:   ent.Time.Format(TimeLayout),
		Level:  levelText(ent.Level),
		Module: ent.LoggerName,
		File:   filepath.Base(ent.Caller.File),
		Func:   shortFunc(ent.Caller.Function),
		Line:   ent.Caller.Line,
		Msg:    ent.Message,
	}
	e.Text = formatEntry(s.settings.Format, e)

	// 文件目标：写失败静默降级，不影响主流程
	if s.file != nil {
		_, _ = s.file.Write([]byte(e.Text + "\n"))
	}

	// 环形缓冲始终写入（供日志页初始加载），事件推送按 UI 目标开关
	if len(s.ring) >= ringCap {
		s.ring = s.ring[1:]
	}
	s.ring = append(s.ring, e)
	if s.settings.uiEnabled() && s.emit != nil {
		s.pending = append(s.pending, e)
	}
	return nil
}

func (s *sink) Sync() error {
	s.flush()
	return nil
}

// close 停止推送循环并关闭文件。
func (s *sink) close() {
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.flush()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
}

// ---- 前端批量推送 ----

func (s *sink) flushLoop() {
	t := time.NewTicker(flushInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			s.flush()
		case <-s.stopCh:
			return
		}
	}
}

func (s *sink) flush() {
	s.mu.Lock()
	batch := s.pending
	s.pending = nil
	emit := s.emit
	s.mu.Unlock()
	if len(batch) > 0 && emit != nil {
		emit(batch)
	}
}

// ---- 格式化辅助 ----

// formatEntry 按模板做占位符替换。
func formatEntry(format string, e LogEntry) string {
	return strings.NewReplacer(
		"{datetime}", e.Time,
		"{level}", e.Level,
		"{module}", e.Module,
		"{file}", e.File,
		"{func}", e.Func,
		"{line}", strconv.Itoa(e.Line),
		"{msg}", e.Msg,
	).Replace(format)
}

// levelText zap 级别转大写文本。
func levelText(lvl zapcore.Level) string {
	return strings.ToUpper(lvl.String())
}

// shortFunc 完整函数路径截短：
// "tdl-ui/internal/services.(*AuthService).Logout" -> "(*AuthService).Logout"。
func shortFunc(fn string) string {
	if i := strings.LastIndexByte(fn, '/'); i >= 0 {
		fn = fn[i+1:]
	}
	if i := strings.IndexByte(fn, '.'); i >= 0 {
		fn = fn[i+1:]
	}
	return fn
}
