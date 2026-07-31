package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/iyear/tdl/core/storage/keygen"
	"github.com/iyear/tdl/pkg/kv"

	"tdl-ui/internal/config"
	"tdl-ui/internal/engine"
)

// ---- LOGIC-001 回归：登出/重登必须 join 登录流程 goroutine ----

// 流程正常退出（关闭 done）时，cancelLoginAndWait 应在超时前成功返回。
func TestCancelLoginAndWaitJoinsFlow(t *testing.T) {
	s := &AuthService{}
	flow := s.begin()

	go func() {
		time.Sleep(200 * time.Millisecond) // 模拟 gotd 层慢退出
		close(flow.done)
	}()

	start := time.Now()
	if err := s.cancelLoginAndWait(2 * time.Second); err != nil {
		t.Fatalf("流程已退出仍返回错误: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("未等待流程退出即返回（耗时 %s），join 语义失效", elapsed)
	}
}

// 流程未退出（done 不关闭）时应超时返回错误，调用方不得继续删除会话。
func TestCancelLoginAndWaitTimeout(t *testing.T) {
	s := &AuthService{}
	s.begin() // done 永不关闭，模拟流程挂死

	if err := s.cancelLoginAndWait(100 * time.Millisecond); err == nil {
		t.Fatal("流程未退出却未返回超时错误")
	}
}

// 从未启动过登录流程时应立即成功返回。
func TestCancelLoginAndWaitNoFlow(t *testing.T) {
	s := &AuthService{}
	if err := s.cancelLoginAndWait(100 * time.Millisecond); err != nil {
		t.Fatalf("无流程时应直接成功，实际 %v", err)
	}
}

// ---- LOGIC-003 回归：启动对账以 kv 会话为权威源 ----

// newReconcileFixture 构造带真实 config 与 bolt kv 的 AuthService。
func newReconcileFixture(t *testing.T) (*AuthService, *config.Manager, kv.Storage) {
	t.Helper()
	dir := t.TempDir()
	cfg, err := config.NewManagerAt(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatalf("初始化配置失败: %v", err)
	}
	kvs, err := kv.New(kv.DriverBolt, map[string]any{"path": filepath.Join(dir, "kv")})
	if err != nil {
		t.Fatalf("初始化 kv 失败: %v", err)
	}
	t.Cleanup(func() { _ = kvs.Close() })
	return NewAuthService(cfg, kvs, nil), cfg, kvs
}

// kv 无会话 + config 有登录态 → 对账后 config 清零。
func TestReconcileClearsStaleLoginState(t *testing.T) {
	s, cfg, _ := newReconcileFixture(t)

	st := cfg.Get()
	st.LoggedInUserID = 42
	st.LoggedInUsername = "ghost"
	if err := cfg.Update(st); err != nil {
		t.Fatalf("写入登录态失败: %v", err)
	}

	s.ReconcileOnStartup()

	got := cfg.Get()
	if got.LoggedInUserID != 0 || got.LoggedInUsername != "" {
		t.Fatalf("kv 无会话时应清零展示登录态，实际 id=%d name=%q", got.LoggedInUserID, got.LoggedInUsername)
	}
	if st := s.Status(); st.LoggedIn || st.SessionPresent {
		t.Fatalf("对账后 Status 应为未登录且无会话: %+v", st)
	}
}

// kv 有会话 + config 无登录态 → config 不动，sessionPresent 提示前端。
func TestReconcileReportsSessionPresent(t *testing.T) {
	s, cfg, kvs := newReconcileFixture(t)

	kvd, err := kvs.Open(engine.Namespace)
	if err != nil {
		t.Fatalf("open kv: %v", err)
	}
	if err := kvd.Set(context.Background(), keygen.New(sessionKeyName), []byte("creds")); err != nil {
		t.Fatalf("写入会话失败: %v", err)
	}

	s.ReconcileOnStartup()

	got := cfg.Get()
	if got.LoggedInUserID != 0 || got.LoggedInUsername != "" {
		t.Fatalf("kv 有会话时不得伪造展示登录态: id=%d name=%q", got.LoggedInUserID, got.LoggedInUsername)
	}
	st := s.Status()
	if st.LoggedIn {
		t.Fatal("config 无登录态时 LoggedIn 应为 false")
	}
	if !st.SessionPresent {
		t.Fatal("kv 有会话时 SessionPresent 应为 true")
	}
}
