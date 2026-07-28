package logging

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func testEntry(msg string) zapcore.Entry {
	return zapcore.Entry{
		Level:      zapcore.InfoLevel,
		Time:       time.Date(2026, 7, 28, 21, 59, 0, 123450000, time.Local),
		LoggerName: "auth",
		Message:    msg,
		Caller: zapcore.EntryCaller{
			Defined:  true,
			File:     "e:/devs/app/internal/services/auth.go",
			Line:     42,
			Function: "tdl-ui/internal/services.(*AuthService).Logout",
		},
	}
}

func TestSinkFormat(t *testing.T) {
	s := newSink(zap.NewAtomicLevelAt(zapcore.DebugLevel))
	defer s.close()
	s.settings.Targets = "ui" // 不写文件

	if err := s.Write(testEntry("用户登出"), nil); err != nil {
		t.Fatal(err)
	}
	got := s.recent()
	if len(got) != 1 {
		t.Fatalf("环形缓冲条数 = %d", len(got))
	}
	e := got[0]
	want := "2026-07-28 21:59:00.12345 [INFO] - [auth] - [auth.go] - [(*AuthService).Logout] - [42] - 用户登出"
	if e.Text != want {
		t.Fatalf("格式化结果不符:\n got: %s\nwant: %s", e.Text, want)
	}
	if e.Seq != 1 || e.Line != 42 || e.Module != "auth" || e.Func != "(*AuthService).Logout" {
		t.Fatalf("字段解析错误: %+v", e)
	}
}

func TestFormatEntryCustomTemplate(t *testing.T) {
	e := LogEntry{Time: "T", Level: "WARN", Module: "m", File: "f.go", Func: "fn", Line: 7, Msg: "hello"}
	got := formatEntry("{level}|{module}|{line}|{msg}", e)
	if got != "WARN|m|7|hello" {
		t.Fatalf("自定义模板渲染错误: %s", got)
	}
}

func TestShortFunc(t *testing.T) {
	cases := map[string]string{
		"tdl-ui/internal/services.(*AuthService).Logout": "(*AuthService).Logout",
		"main.main":  "main",
		"pkg.fn":     "fn",
		"standalone": "standalone",
	}
	for in, want := range cases {
		if got := shortFunc(in); got != want {
			t.Errorf("shortFunc(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRingTrim(t *testing.T) {
	s := newSink(zap.NewAtomicLevelAt(zapcore.DebugLevel))
	defer s.close()
	s.settings.Targets = "ui"

	for i := 0; i < ringCap+10; i++ {
		_ = s.Write(testEntry(fmt.Sprintf("msg-%d", i)), nil)
	}
	got := s.recent()
	if len(got) != ringCap {
		t.Fatalf("缓冲应裁剪至 %d，实际 %d", ringCap, len(got))
	}
	// 序号连续且保留最新
	if got[0].Seq != 11 || got[len(got)-1].Seq != ringCap+10 {
		t.Fatalf("裁剪后序号错误: 首 %d 尾 %d", got[0].Seq, got[len(got)-1].Seq)
	}
}

func TestFlushBatch(t *testing.T) {
	s := newSink(zap.NewAtomicLevelAt(zapcore.DebugLevel))
	defer s.close()
	s.settings.Targets = "ui"

	var mu = make(chan []LogEntry, 1)
	s.setEmit(func(batch []LogEntry) { mu <- batch })

	_ = s.Write(testEntry("a"), nil)
	_ = s.Write(testEntry("b"), nil)
	s.flush()

	select {
	case batch := <-mu:
		if len(batch) != 2 {
			t.Fatalf("批量条数 = %d", len(batch))
		}
	case <-time.After(time.Second):
		t.Fatal("flush 未触发 emit")
	}
}

func TestUIDisabledNoPending(t *testing.T) {
	s := newSink(zap.NewAtomicLevelAt(zapcore.DebugLevel))
	defer s.close()
	s.settings.Targets = "file"
	s.file = nil // 不实际写盘
	s.setEmit(func([]LogEntry) { t.Fatal("file 目标下不应推送前端") })

	_ = s.Write(testEntry("x"), nil)
	s.flush()
	// 环形缓冲仍应写入
	if len(s.recent()) != 1 {
		t.Fatal("环形缓冲应始终写入")
	}
}

func TestLevelGating(t *testing.T) {
	lv := zap.NewAtomicLevelAt(zapcore.WarnLevel)
	s := newSink(lv)
	defer s.close()
	s.settings.Targets = "ui"

	logger := zap.New(s, zap.AddCaller()).Named("test").Sugar()
	logger.Infof("被过滤")
	logger.Warnf("保留")

	got := s.recent()
	if len(got) != 1 || !strings.Contains(got[0].Text, "保留") {
		t.Fatalf("级别门控失效: %+v", got)
	}

	// 热切换级别
	lv.SetLevel(zapcore.DebugLevel)
	logger.Debugf("debug 生效")
	if len(s.recent()) != 2 {
		t.Fatal("热切换级别未生效")
	}
}
