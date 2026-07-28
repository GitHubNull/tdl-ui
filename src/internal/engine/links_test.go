package engine

import (
	"strings"
	"testing"
)

func TestNormalizeURLsValid(t *testing.T) {
	got, err := NormalizeURLs([]string{
		"  https://t.me/opencfdchannel/4434  ",
		"",
		"https://t.me/c/1234567890/456",
		"https://t.me/opencfdchannel/4434", // 重复，应去重
		"https://t.me/somegroup/12/3456",   // 话题群
		"https://t.me/opencfdchannel/4434?comment=360409",
		"https://telegram.me/channel_name/123",
	})
	if err != nil {
		t.Fatalf("预期通过，实际报错: %v", err)
	}
	// 7 条输入：去 1 空行、去 1 重复，剩 5 条有效
	expect := []string{
		"https://t.me/opencfdchannel/4434",
		"https://t.me/c/1234567890/456",
		"https://t.me/somegroup/12/3456",
		"https://t.me/opencfdchannel/4434?comment=360409",
		"https://telegram.me/channel_name/123",
	}
	if len(got) != len(expect) {
		t.Fatalf("预期 %d 条，实际 %d 条: %v", len(expect), len(got), got)
	}
	for i := range expect {
		if got[i] != expect[i] {
			t.Errorf("第 %d 条预期 %q，实际 %q", i, expect[i], got[i])
		}
	}
}

func TestNormalizeURLsInvalid(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string // 错误信息需包含的关键词
	}{
		{"非 t.me 域名", "https://example.com/chan/123", "t.me"},
		{"缺少消息 ID", "https://t.me/channel_name", "消息 ID"},
		{"消息 ID 非数字", "https://t.me/channel_name/abc", "数字"},
		{"私有频道缺段", "https://t.me/c/1234567890", "私有频道"},
		{"无协议", "t.me/channel_name/123", "https"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NormalizeURLs([]string{c.url})
			if err == nil {
				t.Fatalf("预期报错，实际通过: %s", c.url)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("错误信息 %q 未包含关键词 %q", err.Error(), c.want)
			}
		})
	}
}

func TestNormalizeURLsEmpty(t *testing.T) {
	if _, err := NormalizeURLs([]string{"", "   "}); err == nil {
		t.Fatal("全空输入预期报错")
	}
}
