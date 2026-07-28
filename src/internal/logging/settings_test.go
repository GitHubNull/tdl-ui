package logging

import (
	"strings"
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestWithDefaults(t *testing.T) {
	s := LogSettings{}.WithDefaults()
	if s.Targets != "both" || s.Level != "info" || s.Format != DefaultFormat {
		t.Fatalf("默认值错误: %+v", s)
	}
	if s.MaxSizeMB != 10 || s.MaxAgeDays != 7 || s.MaxBackups != 10 {
		t.Fatalf("滚动默认值错误: %+v", s)
	}
	if s.Dir == "" {
		t.Fatal("默认目录不应为空")
	}

	// 非法值兜底
	bad := LogSettings{Targets: "x", Level: "trace", MaxSizeMB: -1}.WithDefaults()
	if bad.Targets != "both" || bad.Level != "info" || bad.MaxSizeMB != 10 {
		t.Fatalf("非法值未兜底: %+v", bad)
	}

	// 大小写归一
	up := LogSettings{Level: "DEBUG"}.WithDefaults()
	if up.Level != "debug" {
		t.Fatalf("级别未小写归一: %q", up.Level)
	}
}

func TestParseYAMLDefaults(t *testing.T) {
	s, err := ParseYAML([]byte("level: warn\nmaxSizeMb: 5\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Level != "warn" || s.MaxSizeMB != 5 {
		t.Fatalf("显式字段解析失败: %+v", s)
	}
	if s.Targets != "both" || s.Format != DefaultFormat || s.MaxAgeDays != 7 {
		t.Fatalf("缺省字段未补齐: %+v", s)
	}

	if _, err := ParseYAML([]byte(":::bad yaml")); err == nil {
		t.Fatal("非法 YAML 应报错")
	}
}

func TestYAMLRoundTrip(t *testing.T) {
	src := LogSettings{Targets: "file", Level: "error", Dir: "d", Format: "{msg}", MaxSizeMB: 1, MaxAgeDays: 2, MaxBackups: 3}
	b, err := src.ToYAML()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseYAML(b)
	if err != nil {
		t.Fatal(err)
	}
	if got != src {
		t.Fatalf("往返不一致: %+v != %+v", got, src)
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]zapcore.Level{
		"debug": zapcore.DebugLevel,
		"info":  zapcore.InfoLevel,
		"warn":  zapcore.WarnLevel,
		"error": zapcore.ErrorLevel,
		"other": zapcore.InfoLevel,
	}
	for in, want := range cases {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestTimeLayoutPrecision(t *testing.T) {
	// 秒后必须是 5 位小数
	parts := strings.SplitN(TimeLayout, ".", 2)
	if len(parts) != 2 || len(parts[1]) != 5 {
		t.Fatalf("时间格式精度错误: %s", TimeLayout)
	}
}
