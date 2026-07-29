package logging

import (
	"fmt"
	"path/filepath"
	"sort"
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
	// fileErrLogged 首次文件写失败已告警，避免每条都刷（LOG-03）；reconfigure 时重置
	fileErrLogged bool
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
	s.fileErrLogged = false
	s.settings = cfg
	if cfg.fileEnabled() {
		// LOG-03：目录可写探测；Program Files 等只读位置回退到用户缓存目录，
		// 并向 UI ring 写一条 WARN，避免文件日志整体静默失效
		if err := ensureWritableDir(cfg.Dir); err != nil {
			if fallback := fallbackLogDir(); fallback != "" && ensureWritableDir(fallback) == nil {
				s.noteLocked("WARN", fmt.Sprintf("日志目录不可写（%s: %v），已回退到 %s", cfg.Dir, err, fallback))
				cfg.Dir = fallback
				s.settings = cfg
			} else {
				s.noteLocked("WARN", fmt.Sprintf("日志目录不可写（%s: %v），文件日志可能失效", cfg.Dir, err))
			}
		}
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

// With 返回记住绑定字段的包装 core，写入时与调用点字段合并（LOG-01）。
func (s *sink) With(fields []zapcore.Field) zapcore.Core {
	if len(fields) == 0 {
		return s
	}
	return &fieldedSink{sink: s, fields: fields}
}

func (s *sink) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if s.Enabled(ent.Level) {
		return ce.AddCore(ent, s)
	}
	return ce
}

func (s *sink) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	s.mu.Lock()

	s.seq++
	e := LogEntry{
		Seq:    s.seq,
		Time:   ent.Time.Format(TimeLayout),
		Level:  levelText(ent.Level),
		Module: ent.LoggerName,
		File:   filepath.Base(ent.Caller.File),
		Func:   shortFunc(ent.Caller.Function),
		Line:   ent.Caller.Line,
		Msg:    appendFields(ent.Message, fields),
	}
	e.Text = formatEntry(s.settings.Format, e)
	file := s.file

	// 环形缓冲始终写入（供日志页初始加载），事件推送按 UI 目标开关
	if len(s.ring) >= ringCap {
		s.ring = s.ring[1:]
	}
	s.ring = append(s.ring, e)
	if s.settings.uiEnabled() && s.emit != nil {
		s.pending = append(s.pending, e)
	}
	s.mu.Unlock()

	// LOG-02：文件写移出临界区，同步轮转（rename+新建）不再阻塞全部日志调用；
	// lumberjack 自身并发安全。写失败首次告警（LOG-03）
	if file != nil {
		if _, err := file.Write([]byte(e.Text + "\n")); err != nil {
			s.noteFileError(err)
		}
	}
	return nil
}

// noteFileError 首次文件写失败时向 UI ring 记一条 WARN（LOG-03），后续不再重复。
func (s *sink) noteFileError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fileErrLogged {
		return
	}
	s.fileErrLogged = true
	s.noteLocked("WARN", fmt.Sprintf("日志文件写入失败（后续错误不再提示）: %v", err))
}

// noteLocked 在持有 s.mu 时向 ring/pending 追加一条 logging 模块自身的告警。
func (s *sink) noteLocked(level, msg string) {
	s.seq++
	e := LogEntry{
		Seq:    s.seq,
		Time:   time.Now().Format(TimeLayout),
		Level:  level,
		Module: "logging",
		Msg:    msg,
	}
	e.Text = formatEntry(s.settings.Format, e)
	if len(s.ring) >= ringCap {
		s.ring = s.ring[1:]
	}
	s.ring = append(s.ring, e)
	if s.settings.uiEnabled() && s.emit != nil {
		s.pending = append(s.pending, e)
	}
}

func (s *sink) Sync() error {
	s.flush()
	return nil
}

// fieldedSink 携带 With 绑定字段的轻量包装 core，Write 时把绑定字段与调用点字段合并后交给底层 sink。
type fieldedSink struct {
	*sink
	fields []zapcore.Field
}

func (f *fieldedSink) With(fields []zapcore.Field) zapcore.Core {
	if len(fields) == 0 {
		return f
	}
	merged := make([]zapcore.Field, 0, len(f.fields)+len(fields))
	merged = append(merged, f.fields...)
	merged = append(merged, fields...)
	return &fieldedSink{sink: f.sink, fields: merged}
}

func (f *fieldedSink) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if f.Enabled(ent.Level) {
		return ce.AddCore(ent, f)
	}
	return ce
}

func (f *fieldedSink) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	merged := make([]zapcore.Field, 0, len(f.fields)+len(fields))
	merged = append(merged, f.fields...)
	merged = append(merged, fields...)
	return f.sink.Write(ent, merged)
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

// appendFields 将结构化字段渲染为 " {k=v ...}" 追加到消息文本（键名排序保证输出稳定）。
func appendFields(msg string, fields []zapcore.Field) string {
	if len(fields) == 0 {
		return msg
	}
	enc := zapcore.NewMapObjectEncoder()
	for i := range fields {
		fields[i].AddTo(enc)
	}
	keys := make([]string, 0, len(enc.Fields))
	for k := range enc.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(msg)
	b.WriteString(" {")
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%s=%v", k, enc.Fields[k])
	}
	b.WriteByte('}')
	return b.String()
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
