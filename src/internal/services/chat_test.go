package services

import (
	"context"
	"errors"
	"testing"
	"time"

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

// ---- ensureStarted 并发与错误路径回归测试 ----

// awaitErr 在限定时间内等待 ensureStarted 返回，超时即视为卡死。
func awaitErr(t *testing.T, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("调用未在限定时间内返回（回归：死锁/挂死）")
		return nil
	}
}

// 回归：ready 返回错误（如未登录）后 ensureStarted 必须返回错误而非与收尾协程互锁卡死。
func TestEnsureStartedErrorNoDeadlock(t *testing.T) {
	s := &ChatService{
		runClient: func(_ context.Context, ready chan<- error, _, _, _ <-chan chatJob) {
			ready <- errNotLoggedIn
		},
	}

	for i := 0; i < 2; i++ { // 连续两次：验证失败后无残留状态阻塞重试
		err := awaitErr(t, s.ensureStarted)
		if !errors.Is(err, errNotLoggedIn) {
			t.Fatalf("第 %d 次预期 errNotLoggedIn，实际 %v", i+1, err)
		}
	}
}

// 建连挂起（网络不通/未配代理）时应在 connectTimeout 内返回超时错误。
func TestEnsureStartedConnectTimeout(t *testing.T) {
	old := connectTimeout
	connectTimeout = 200 * time.Millisecond
	defer func() { connectTimeout = old }()

	s := &ChatService{
		runClient: func(ctx context.Context, _ chan<- error, _, _, _ <-chan chatJob) {
			<-ctx.Done() // 模拟连接阶段永久阻塞，直到被取消
		},
	}

	err := awaitErr(t, s.ensureStarted)
	if err == nil {
		t.Fatal("预期超时错误，实际成功")
	}
}

// 就绪后 invoke 能正常执行任务，Stop 后状态复位可重新启动。
func TestEnsureStartedInvokeAndStop(t *testing.T) {
	s := &ChatService{
		runClient: func(ctx context.Context, ready chan<- error, jobs, _, _ <-chan chatJob) {
			ready <- nil
			for {
				select {
				case j := <-jobs:
					j.done <- j.fn(ctx, nil)
				case <-ctx.Done():
					return
				}
			}
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	called := false
	err := s.invoke(ctx, func(_ context.Context, _ *tg.Client) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("invoke 失败: %v", err)
	}
	if !called {
		t.Fatal("任务闭包未被执行")
	}

	s.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		running := s.running
		s.mu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Stop 后 running 未复位")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ARC-001 回归：建连阶段生命周期句柄已提前发布，StopAndWait 必须等待
// 未就绪的连接协程退出（阻塞至超时），而非因句柄缺失立即返回。
func TestStopAndWaitCoversConnectingPhase(t *testing.T) {
	old := connectTimeout
	connectTimeout = 5 * time.Second
	defer func() { connectTimeout = old }()

	release := make(chan struct{})
	s := &ChatService{
		runClient: func(_ context.Context, _ chan<- error, _, _, _ <-chan chatJob) {
			<-release // 模拟慢建连：对 ctx 取消暂时无感知
		},
	}
	defer close(release)

	go func() { _ = s.ensureStarted() }()

	// 句柄应在建连 goroutine 启动前发布，很快可见
	deadline := time.Now().Add(2 * time.Second)
	for {
		s.mu.Lock()
		published := s.dead != nil
		s.mu.Unlock()
		if published {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("建连阶段未发布生命周期句柄（ARC-001 回归）")
		}
		time.Sleep(5 * time.Millisecond)
	}

	started := time.Now()
	s.StopAndWait(100 * time.Millisecond)
	if elapsed := time.Since(started); elapsed < 100*time.Millisecond {
		t.Fatalf("StopAndWait 应阻塞等待建连协程退出直至超时，实际仅 %v", elapsed)
	}
}
