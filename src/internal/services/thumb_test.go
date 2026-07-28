package services

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gotd/td/tg"
)

func TestExpandStrippedThumb(t *testing.T) {
	body := []byte{0x01, 0x28, 0x1e, 0xAA, 0xBB, 0xCC}
	got := expandStrippedThumb(body)
	if got == nil {
		t.Fatal("合法 stripped 数据不应返回 nil")
	}
	if !bytes.HasPrefix(got, []byte{0xFF, 0xD8}) {
		t.Fatal("输出应以 JPEG SOI (FFD8) 开头")
	}
	if !bytes.HasSuffix(got, []byte{0xFF, 0xD9}) {
		t.Fatal("输出应以 JPEG EOI (FFD9) 结尾")
	}
	if got[164] != 0x28 || got[166] != 0x1e {
		t.Fatalf("宽高字节未写入: header[164]=%x header[166]=%x", got[164], got[166])
	}
	// 体数据应完整拼接在头之后
	if !bytes.Contains(got, []byte{0xAA, 0xBB, 0xCC}) {
		t.Fatal("JPEG 体数据丢失")
	}
}

func TestExpandStrippedThumbInvalid(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		{0x01, 0x10},       // 过短
		{0x02, 0x10, 0x10, 0xAA}, // 首字节非 0x01
	}
	for _, c := range cases {
		if got := expandStrippedThumb(c); got != nil {
			t.Fatalf("非法输入 %v 应返回 nil", c)
		}
	}
}

func TestStrippedThumbURI(t *testing.T) {
	sizes := []tg.PhotoSizeClass{
		&tg.PhotoSize{Type: "m", Size: 1024},
		&tg.PhotoStrippedSize{Type: "i", Bytes: []byte{0x01, 0x20, 0x20, 0xAA}},
	}
	uri := strippedThumbURI(sizes)
	if !strings.HasPrefix(uri, "data:image/jpeg;base64,") {
		t.Fatalf("应返回 JPEG data URI，实际 %q", uri)
	}

	if got := strippedThumbURI([]tg.PhotoSizeClass{&tg.PhotoSize{Type: "m"}}); got != "" {
		t.Fatalf("无 stripped 尺寸应返回空串，实际 %q", got)
	}
}

func TestPickThumbSize(t *testing.T) {
	// 优先 "m"
	typ, _ := pickThumbSize([]tg.PhotoSizeClass{
		&tg.PhotoSize{Type: "s", Size: 100},
		&tg.PhotoSize{Type: "m", Size: 500},
		&tg.PhotoSize{Type: "x", Size: 2000},
	})
	if typ != "m" {
		t.Fatalf("应优先选择 m，实际 %q", typ)
	}

	// 无 "m" 时取最小
	typ, _ = pickThumbSize([]tg.PhotoSizeClass{
		&tg.PhotoSize{Type: "x", Size: 2000},
		&tg.PhotoSize{Type: "s", Size: 100},
	})
	if typ != "s" {
		t.Fatalf("应回退最小尺寸 s，实际 %q", typ)
	}

	// 仅 cached：返回内联字节
	typ, inline := pickThumbSize([]tg.PhotoSizeClass{
		&tg.PhotoCachedSize{Type: "s", Bytes: []byte{1, 2, 3}},
	})
	if typ != "" || len(inline) != 3 {
		t.Fatalf("仅 cached 时应返回内联字节，实际 typ=%q inline=%v", typ, inline)
	}
}
