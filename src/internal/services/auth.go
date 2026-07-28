// Package services Wails 绑定服务层：暴露给前端调用的方法集合。
package services

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/session"
	tdtdesktop "github.com/gotd/td/session/tdesktop"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/iyear/tdl/core/storage"
	"github.com/iyear/tdl/core/storage/keygen"
	"github.com/iyear/tdl/core/util/fsutil"
	"github.com/iyear/tdl/pkg/key"
	"github.com/iyear/tdl/pkg/kv"
	pkgtclient "github.com/iyear/tdl/pkg/tclient"
	"github.com/iyear/tdl/pkg/tpath"

	"tdl-ui/internal/config"
	"tdl-ui/internal/engine"
	"tdl-ui/internal/events"
	"tdl-ui/internal/logging"
)

var logAuth = logging.L("auth")

// reconnectTimeout 登录客户端断线重连超时。
const reconnectTimeout = 5 * time.Minute

// AuthService 账号登录管理：验证码登录、二维码登录、Desktop 会话导入与登出。
type AuthService struct {
	cfg     *config.Manager
	kv      kv.Storage
	emitter *events.Emitter

	// OnLogout 登出成功后的回调（如关闭对话服务常驻连接），可为空。
	OnLogout func()

	mu     sync.Mutex
	cancel context.CancelFunc
	codeCh chan string
	pwdCh  chan string
}

// NewAuthService 创建登录服务。
func NewAuthService(cfg *config.Manager, kvs kv.Storage, emitter *events.Emitter) *AuthService {
	return &AuthService{cfg: cfg, kv: kvs, emitter: emitter}
}

// LoginStatus 当前登录状态（来自本地记录，不发起网络请求）。
type LoginStatus struct {
	LoggedIn bool   `json:"loggedIn"`
	UserID   int64  `json:"userId"`
	Username string `json:"username"`
}

// Status 返回本地记录的登录状态。
func (s *AuthService) Status() LoginStatus {
	st := s.cfg.Get()
	return LoginStatus{
		LoggedIn: st.LoggedInUserID != 0,
		UserID:   st.LoggedInUserID,
		Username: st.LoggedInUsername,
	}
}

// StartCodeLogin 启动验证码登录流程。
// 后续通过 login:update 事件驱动：need_code → SubmitCode，need_password → SubmitPassword。
func (s *AuthService) StartCodeLogin(phone string) error {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return errors.New("手机号不能为空")
	}
	logAuth.Infof("启动验证码登录: %s", maskPhone(phone))

	ctx, err := s.begin()
	if err != nil {
		return err
	}

	go s.runCodeLogin(ctx, phone)
	return nil
}

// StartQRLogin 启动二维码登录流程，二维码 URL 通过 login:update(stage=qr) 事件推送。
func (s *AuthService) StartQRLogin() error {
	logAuth.Infof("启动二维码登录")
	ctx, err := s.begin()
	if err != nil {
		return err
	}

	go s.runQRLogin(ctx)
	return nil
}

// SubmitCode 提交收到的验证码。
func (s *AuthService) SubmitCode(code string) {
	logAuth.Debugf("收到验证码提交")
	s.mu.Lock()
	ch := s.codeCh
	s.mu.Unlock()
	if ch != nil {
		select {
		case ch <- strings.TrimSpace(code):
		default:
		}
	}
}

// SubmitPassword 提交二步验证密码。
func (s *AuthService) SubmitPassword(pwd string) {
	logAuth.Debugf("收到二步验证密码提交")
	s.mu.Lock()
	ch := s.pwdCh
	s.mu.Unlock()
	if ch != nil {
		select {
		case ch <- pwd:
		default:
		}
	}
}

// CancelLogin 取消进行中的登录流程。
func (s *AuthService) CancelLogin() {
	logAuth.Infof("取消登录流程")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// Logout 清除本地会话数据。
func (s *AuthService) Logout() error {
	logAuth.Infof("开始登出，清除本地会话")
	kvd, err := s.kv.Open(engine.Namespace)
	if err != nil {
		logAuth.Errorf("登出失败（open kv）: %v", err)
		return errors.Wrap(err, "open kv")
	}

	ctx := context.Background()
	// 会话与 App 标记（key 与 core/storage/session.go 保持一致）
	if err = kvd.Delete(ctx, keygen.New("session")); err != nil && !errors.Is(err, storage.ErrNotFound) {
		logAuth.Errorf("登出失败（删除会话）: %v", err)
		return errors.Wrap(err, "delete session")
	}
	_ = kvd.Delete(ctx, key.App())

	st := s.cfg.Get()
	st.LoggedInUserID = 0
	st.LoggedInUsername = ""
	if err = s.cfg.Update(st); err != nil {
		return err
	}

	s.emitter.Emit(events.Login, events.LoginUpdate{Stage: "logout"})
	if s.OnLogout != nil {
		s.OnLogout()
	}
	logAuth.Infof("登出完成")
	return nil
}

// DesktopAccount Telegram Desktop 中检测到的账号。
type DesktopAccount struct {
	UserID string `json:"userId"`
}

// DetectDesktopPath 自动探测本机 Telegram Desktop 数据目录，未找到返回空串。
func (s *AuthService) DetectDesktopPath() string {
	home := s.homeDir()
	for _, p := range tpath.Desktop.AppData(home) {
		if path := appendTData(p); fsutil.PathExists(path) {
			logAuth.Debugf("探测到 Telegram Desktop 数据目录: %s", path)
			return path
		}
	}
	logAuth.Debugf("未探测到 Telegram Desktop 数据目录")
	return ""
}

// ListDesktopAccounts 读取 Telegram Desktop 会话中的账号列表。
func (s *AuthService) ListDesktopAccounts(path, passcode string) ([]DesktopAccount, error) {
	if path == "" {
		return nil, errors.New("未找到 Telegram Desktop 数据目录，请手动指定")
	}

	accounts, err := tdtdesktop.Read(appendTData(path), []byte(passcode))
	if err != nil {
		logAuth.Errorf("读取 Desktop 会话失败: %v", err)
		return nil, errors.Wrap(err, "读取 Desktop 会话失败")
	}

	out := make([]DesktopAccount, 0, len(accounts))
	for _, acc := range accounts {
		out = append(out, DesktopAccount{UserID: strconv.FormatUint(acc.Authorization.UserID, 10)})
	}
	logAuth.Infof("Desktop 会话中检测到 %d 个账号", len(out))
	return out, nil
}

// ImportDesktopSession 导入指定账号的 Desktop 会话（对应 ref/tdl/app/login/desktop.go），
// 并建连校验会话有效性；校验失败时回滚已写入的会话，避免出现假登录态。
func (s *AuthService) ImportDesktopSession(path, passcode, userID string) error {
	logAuth.Infof("导入 Desktop 会话，账号 %s", userID)
	accounts, err := tdtdesktop.Read(appendTData(path), []byte(passcode))
	if err != nil {
		logAuth.Errorf("读取 Desktop 会话失败: %v", err)
		return errors.Wrap(err, "读取 Desktop 会话失败")
	}

	var target *tdtdesktop.Account
	for i, acc := range accounts {
		if strconv.FormatUint(acc.Authorization.UserID, 10) == userID {
			target = &accounts[i]
			break
		}
	}
	if target == nil {
		return errors.Errorf("未找到账号 %s", userID)
	}

	data, err := session.TDesktopSession(*target)
	if err != nil {
		return errors.Wrap(err, "转换会话失败")
	}

	ctx := context.Background()
	kvd, err := s.kv.Open(engine.Namespace)
	if err != nil {
		return errors.Wrap(err, "open kv")
	}

	loader := &session.Loader{Storage: storage.NewSession(kvd, true)}
	if err = loader.Save(ctx, data); err != nil {
		return errors.Wrap(err, "保存会话失败")
	}
	if err = kvd.Set(ctx, key.App(), []byte(pkgtclient.AppDesktop)); err != nil {
		return errors.Wrap(err, "设置 app 失败")
	}

	// 建连校验：会话无效时回滚，不留下假登录态
	user, err := s.verifySession(ctx, kvd)
	if err != nil {
		logAuth.Errorf("Desktop 会话校验失败，已回滚: %v", err)
		s.clearSession(ctx, kvd)
		return err
	}

	s.loginSuccess(user)
	return nil
}

// verifySession 建连校验当前会话是否已授权，并返回账号信息；带超时避免挂死。
func (s *AuthService) verifySession(parent context.Context, kvd storage.Storage) (*tg.User, error) {
	ctx, cancel := context.WithTimeout(parent, connectTimeout)
	defer cancel()

	c, err := pkgtclient.New(ctx, pkgtclient.Options{
		KV:               kvd,
		Proxy:            s.cfg.Get().Proxy,
		ReconnectTimeout: reconnectTimeout,
	}, false)
	if err != nil {
		return nil, errors.Wrap(err, "create client")
	}

	var user *tg.User
	err = c.Run(ctx, func(ctx context.Context) error {
		st, err := c.Auth().Status(ctx)
		if err != nil {
			return errors.Wrap(err, "查询登录状态失败")
		}
		if !st.Authorized {
			return errors.New("导入的会话未授权或已失效，请确认 Desktop 端处于登录状态后重新导入")
		}
		user, err = c.Self(ctx)
		if err != nil {
			return errors.Wrap(err, "获取账号信息失败")
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("连接 Telegram 超时，请检查网络或在「设置」中配置代理")
		}
		return nil, err
	}
	return user, nil
}

// clearSession 删除已写入的会话数据（校验失败回滚用）。
func (s *AuthService) clearSession(ctx context.Context, kvd storage.Storage) {
	_ = kvd.Delete(ctx, keygen.New("session"))
	_ = kvd.Delete(ctx, key.App())
}

// ---- 内部实现 ----

// begin 初始化一次登录流程的上下文与交互通道。
func (s *AuthService) begin() (context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil { // 终止上一个未完成的流程
		s.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.codeCh = make(chan string, 1)
	s.pwdCh = make(chan string, 1)
	return ctx, nil
}

func (s *AuthService) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// runCodeLogin 验证码登录（对应 ref/tdl/app/login/code.go，交互替换为事件+通道）。
func (s *AuthService) runCodeLogin(ctx context.Context, phone string) {
	defer s.finish()

	err := func() error {
		kvd, err := s.kv.Open(engine.Namespace)
		if err != nil {
			return errors.Wrap(err, "open kv")
		}

		// 与 tdl CLI 一致：验证码登录使用 Desktop App 凭据
		if err = kvd.Set(ctx, key.App(), []byte(pkgtclient.AppDesktop)); err != nil {
			return errors.Wrap(err, "set app")
		}

		c, err := pkgtclient.New(ctx, pkgtclient.Options{
			KV:               kvd,
			Proxy:            s.cfg.Get().Proxy,
			ReconnectTimeout: reconnectTimeout,
		}, true)
		if err != nil {
			return err
		}

		return c.Run(ctx, func(ctx context.Context) error {
			if err := c.Ping(ctx); err != nil {
				return err
			}

			flow := auth.NewFlow(&guiAuth{svc: s, phone: phone}, auth.SendCodeOptions{})
			if err := c.Auth().IfNecessary(ctx, flow); err != nil {
				return err
			}

			user, err := c.Self(ctx)
			if err != nil {
				return err
			}
			s.loginSuccess(user)
			return nil
		})
	}()

	if err != nil && !errors.Is(err, context.Canceled) {
		logAuth.Errorf("验证码登录失败: %v", err)
		s.emitter.Emit(events.Login, events.LoginUpdate{Stage: "error", Error: err.Error()})
	}
}

// runQRLogin 二维码登录（对应 ref/tdl/app/login/qr.go，二维码渲染移至前端）。
func (s *AuthService) runQRLogin(ctx context.Context) {
	defer s.finish()

	err := func() error {
		kvd, err := s.kv.Open(engine.Namespace)
		if err != nil {
			return errors.Wrap(err, "open kv")
		}

		if err = kvd.Set(ctx, key.App(), []byte(pkgtclient.AppDesktop)); err != nil {
			return errors.Wrap(err, "set app")
		}

		d := tg.NewUpdateDispatcher()
		c, err := pkgtclient.New(ctx, pkgtclient.Options{
			KV:               kvd,
			Proxy:            s.cfg.Get().Proxy,
			ReconnectTimeout: reconnectTimeout,
			UpdateHandler:    d,
		}, true)
		if err != nil {
			return errors.Wrap(err, "create client")
		}

		return c.Run(ctx, func(ctx context.Context) error {
			_, err := c.QR().Auth(ctx, qrlogin.OnLoginToken(d), func(ctx context.Context, token qrlogin.Token) error {
				s.emitter.Emit(events.Login, events.LoginUpdate{Stage: "qr", QRURL: token.URL()})
				return nil
			})

			if err != nil {
				// https://core.telegram.org/api/auth#2fa
				if !tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
					return errors.Wrap(err, "qr auth")
				}

				pwd, perr := s.waitPassword(ctx)
				if perr != nil {
					return perr
				}
				if _, err = c.Auth().Password(ctx, pwd); err != nil {
					return errors.Wrap(err, "2fa auth")
				}
			}

			user, err := c.Self(ctx)
			if err != nil {
				return errors.Wrap(err, "get self")
			}
			s.loginSuccess(user)
			return nil
		})
	}()

	if err != nil && !errors.Is(err, context.Canceled) {
		logAuth.Errorf("二维码登录失败: %v", err)
		s.emitter.Emit(events.Login, events.LoginUpdate{Stage: "error", Error: err.Error()})
	}
}

func (s *AuthService) waitPassword(ctx context.Context) (string, error) {
	s.emitter.Emit(events.Login, events.LoginUpdate{Stage: "need_password"})

	s.mu.Lock()
	ch := s.pwdCh
	s.mu.Unlock()

	select {
	case pwd := <-ch:
		return pwd, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *AuthService) loginSuccess(user *tg.User) {
	logAuth.Infof("登录成功，用户 ID %d", user.ID)
	s.saveUser(user.ID, user.Username)
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	s.emitter.Emit(events.Login, events.LoginUpdate{
		Stage: "success",
		User:  &events.User{ID: user.ID, Username: user.Username, Name: name},
	})
}

func (s *AuthService) saveUser(id int64, username string) {
	st := s.cfg.Get()
	st.LoggedInUserID = id
	st.LoggedInUsername = username
	_ = s.cfg.Update(st)
}

func (s *AuthService) homeDir() string {
	home, err := userHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// maskPhone 手机号脱敏：仅保留末 4 位。
func maskPhone(phone string) string {
	if len(phone) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(phone)-4) + phone[len(phone)-4:]
}

// guiAuth 实现 gotd 的 auth.UserAuthenticator：
// 通过事件通知前端、通道接收输入（对应 ref/tdl/app/login/code.go 的 termAuth）。
type guiAuth struct {
	svc   *AuthService
	phone string
}

func (a *guiAuth) Phone(_ context.Context) (string, error) { return a.phone, nil }

func (a *guiAuth) Code(ctx context.Context, _ *tg.AuthSentCode) (string, error) {
	a.svc.emitter.Emit(events.Login, events.LoginUpdate{Stage: "need_code"})

	a.svc.mu.Lock()
	ch := a.svc.codeCh
	a.svc.mu.Unlock()

	select {
	case code := <-ch:
		return code, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (a *guiAuth) Password(ctx context.Context) (string, error) {
	return a.svc.waitPassword(ctx)
}

func (a *guiAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("不支持注册 Telegram 账号")
}

func (a *guiAuth) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	return &auth.SignUpRequired{TermsOfService: tos}
}
