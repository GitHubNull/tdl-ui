package services

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestClassifyKind(t *testing.T) {
	cases := []struct {
		name               string
		mime               string
		videoAttr, audioAttr bool
		want               string
	}{
		{"视频MIME", "video/mp4", false, false, KindVideo},
		{"视频属性优先", "application/octet-stream", true, false, KindVideo},
		{"音频MIME", "audio/mpeg", false, false, KindAudio},
		{"音频属性", "application/octet-stream", false, true, KindAudio},
		{"普通文档", "application/zip", false, false, KindFile},
		{"图片文档算文件", "image/png", false, false, KindFile},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyKind(c.mime, c.videoAttr, c.audioAttr); got != c.want {
				t.Fatalf("预期 %s，实际 %s", c.want, got)
			}
		})
	}
}

func TestKindOfDocument(t *testing.T) {
	doc := &tg.Document{
		MimeType:   "application/octet-stream",
		Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeVideo{}},
	}
	if got := kindOfDocument(doc); got != KindVideo {
		t.Fatalf("带视频属性应为 video，实际 %s", got)
	}

	doc = &tg.Document{MimeType: "audio/flac"}
	if got := kindOfDocument(doc); got != KindAudio {
		t.Fatalf("audio/* 应为 audio，实际 %s", got)
	}
}

func TestServerFilterMapping(t *testing.T) {
	cases := []struct {
		name  string
		kinds []string
		want  tg.MessagesFilterClass // nil 表示回退历史
	}{
		{"空条件回退历史", nil, nil},
		{"仅图片", []string{KindPhoto}, &tg.InputMessagesFilterPhotos{}},
		{"仅视频", []string{KindVideo}, &tg.InputMessagesFilterVideo{}},
		{"图片+视频", []string{KindPhoto, KindVideo}, &tg.InputMessagesFilterPhotoVideo{}},
		{"仅音频", []string{KindAudio}, &tg.InputMessagesFilterDocument{}},
		{"仅文件", []string{KindFile}, &tg.InputMessagesFilterDocument{}},
		{"视频+文件", []string{KindVideo, KindFile}, &tg.InputMessagesFilterDocument{}},
		{"音频+文件", []string{KindAudio, KindFile}, &tg.InputMessagesFilterDocument{}},
		{"图片+文件回退", []string{KindPhoto, KindFile}, nil},
		{"全选回退", []string{KindPhoto, KindVideo, KindAudio, KindFile}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := serverFilter(c.kinds)
			if c.want == nil {
				if got != nil {
					t.Fatalf("预期回退历史(nil)，实际 %T", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("预期 %T，实际回退历史(nil)", c.want)
			}
			if string(got.TypeName()) != string(c.want.TypeName()) {
				t.Fatalf("预期 %T，实际 %T", c.want, got)
			}
		})
	}
}

func TestMatchesFilter(t *testing.T) {
	item := MediaItem{
		Name:    "Intro Video.MP4",
		Caption: "发布会开场视频",
		Size:    10 << 20, // 10 MiB
		Kind:    KindVideo,
		MIME:    "video/mp4",
	}

	cases := []struct {
		name string
		q    MediaQuery
		want bool
	}{
		{"无条件", MediaQuery{}, true},
		{"类型命中", MediaQuery{Kinds: []string{KindVideo}}, true},
		{"类型未命中", MediaQuery{Kinds: []string{KindPhoto}}, false},
		{"扩展名命中(大小写)", MediaQuery{Exts: []string{"mp4"}}, true},
		{"扩展名未命中", MediaQuery{Exts: []string{"mkv"}}, false},
		{"大小范围内", MediaQuery{MinSize: 5 << 20, MaxSize: 20 << 20}, true},
		{"小于下限", MediaQuery{MinSize: 11 << 20}, false},
		{"超过上限", MediaQuery{MaxSize: 9 << 20}, false},
		{"关键词命中文件名(不区分大小写)", MediaQuery{Query: "intro video"}, true},
		{"关键词命中caption", MediaQuery{Query: "开场"}, true},
		{"关键词未命中", MediaQuery{Query: "纪录片"}, false},
		{"组合条件", MediaQuery{Kinds: []string{KindVideo}, Exts: []string{"mp4"}, MinSize: 1 << 20, Query: "intro"}, true},
		{"组合一项失败", MediaQuery{Kinds: []string{KindVideo}, Exts: []string{"avi"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := matchesFilter(c.q, item); got != c.want {
				t.Fatalf("预期 %v，实际 %v", c.want, got)
			}
		})
	}
}

func TestNormalizeExts(t *testing.T) {
	got := normalizeExts([]string{" MP4 ", ".mkv", "mp4", "", "JPG"})
	want := []string{"mp4", "mkv", "jpg"}
	if len(got) != len(want) {
		t.Fatalf("预期 %v，实际 %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("预期 %v，实际 %v", want, got)
		}
	}
}

func TestNormalizeKinds(t *testing.T) {
	got := normalizeKinds([]string{"Video", "video", "photo", "unknown", ""})
	want := []string{KindVideo, KindPhoto}
	if len(got) != len(want) {
		t.Fatalf("预期 %v，实际 %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("预期 %v，实际 %v", want, got)
		}
	}
}
