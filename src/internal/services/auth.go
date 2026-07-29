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

// sessionKeyName 会话在 kv 中的 key 名，与 ref/tdl core/storage/session.go 的布局一致（SVC-10）。
const sessionKeyName = "session"

// AuthService 账号登录管理：验证码登录、二维码登录、Desktop 会话导入与登出。
type AuthService struct {
	cfg     *config.Manager
	kv      kv.Storage
	emitter *events.Emitter

	// OnSessionChanged 会话变更时的回调（登出、登录/导入成功、登录流程启动前），
	// 用于关闭依赖旧会话的对话常驻连接，下次查询用新会话自动重建；可为空。
	OnSessionChanged func()
	// HasActiveDownloads 活跃下载任务检查注入点：下载连接与登录流程会互踩同一份会话，
	// 重登/登出前存在活跃下载时明确拒绝；可为空。
	HasActiveDownloads func() bool

	mu     sync.Mutex
	gen    uint64 // 登录流程代际计数，每次 begin 递增
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
	if err := s.prepareLogin(); err != nil {
		return err
	}
	logAuth.Infof("启动验证码登录: %s", maskPhone(phone))

	flow := s.begin()
	go s.runCodeLogin(flow, phone)
	return nil
}

// StartQRLogin 启动二维码登录流程，二维码 URL 通过 login:update(stage=qr) 事件推送。
func (s *AuthService) StartQRLogin() error {
	if err := s.prepareLogin(); err != nil {
		return err
	}
	logAuth.Infof("启动二维码登录")
	flow := s.begin()
	go s.runQRLogin(flow)
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

// Logout 清除本地会话数据。存在活跃下载时拒绝登出，
// 避免下载任务拿着已删除的会话继续运行直到报不可解释的错误。
func (s *AuthService) Logout() error {
	if s.HasActiveDownloads != nil && s.HasActiveDownloads() {
		return errors.New("存在进行中的下载任务，请先暂停或取消全部下载后再登出")
	}
	s.CancelLogin() // 同时终止进行中的登录流程
	logAuth.Infof("开始登出，清除本地会话")
	kvd, err := s.kv.Open(engine.Namespace)
	if err != nil {
		logAuth.Errorf("登出失败（open kv）: %v", err)
		return errors.Wrap(err, "open kv")
	}

	ctx := context.Background()
	// 会话与 App 标记（key 与 core/storage/session.go 保持一致）
	if err = kvd.Delete(ctx, keygen.New(sessionKeyName)); err != nil && !errors.Is(err, storage.ErrNotFound) {
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
	if s.OnSessionChanged != nil {
		s.OnSessionChanged()
	}
	logAuth.Infof("登出完成")
	return nil
}

// prepareLogin 登录/导入入口前的安全检查与准备：
// 活跃下载持有旧会话连接，重写会话会互相覆盖，明确拒绝；
// 并先停掉旧账号的对话常驻连接，避免登录流程与其并发读写同一会话。
func (s *AuthService) prepareLogin() error {
	if s.HasActiveDownloads != nil && s.HasActiveDownloads() {
		return errors.New("存在进行中的下载任务，请先暂停或取消全部下载后再重新登录")
	}
	if s.OnSessionChanged != nil {
		s.OnSessionChanged()
	}
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
	if err := s.prepareLogin(); err != nil {
		return err
	}
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

	// 写前备份现有会话与 App 标记：校验失败时写回备份而非删除，
	// 避免导入失败连带毁掉原有效会话。
	sessionKey := keygen.New(sessionKeyName)
	oldSession, sErr := kvd.Get(ctx, sessionKey)
	hadSession := sErr == nil
	oldApp, aErr := kvd.Get(ctx, key.App())
	hadApp := aErr == nil

	if err = loader.Save(ctx, data); err != nil {
		return errors.Wrap(err, "保存会话失败")
	}
	if err = kvd.Set(ctx, key.App(), []byte(pkgtclient.AppDesktop)); err != nil {
		return errors.Wrap(err, "设置 app 失败")
	}

	// 建连校验：会话无效时恢复备份，不留下假登录态
	user, err := s.verifySession(ctx, kvd)
	if err != nil {
		logAuth.Errorf("Desktop 会话校验失败，恢复原会话: %v", err)
		s.restoreSession(ctx, kvd, sessionKey, oldSession, hadSession, oldApp, hadApp)
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

// restoreSession 校验失败后恢复导入前的会话状态：有备份写回备份，
// 无备份则删除新写入的数据并清空配置中的登录标记，避免假登录态。
func (s *AuthService) restoreSession(ctx context.Context, kvd storage.Storage, sessionKey string, oldSession []byte, hadSession bool, oldApp []byte, hadApp bool) {
	if hadSession {
		_ = kvd.Set(ctx, sessionKey, oldSession)
	} else {
		_ = kvd.Delete(ctx, sessionKey)
	}
	if hadApp {
		_ = kvd.Set(ctx, key.App(), oldApp)
	} else {
		_ = kvd.Delete(ctx, key.App())
	}
	if !hadSession {
		// 原本就无会话：同步清空配置登录态，保持 Status() 与 kv 一致
		st := s.cfg.Get()
		if st.LoggedInUserID != 0 {
			st.LoggedInUserID = 0
			st.LoggedInUsername = ""
			if uerr := s.cfg.Update(st); uerr != nil {
				logAuth.Warnf("清空登录态配置失败: %v", uerr)
			}
		}
	}
}

// ---- 内部实现 ----

// loginFlow 一次登录流程的代际上下文：ctx、交互通道与代际号均归属本代。
type loginFlow struct {
	ctx    context.Context
	gen    uint64
	codeCh chan string
	pwdCh  chan string
}

// begin 初始化一次登录流程：取消上一代流程并分配新代际。
func (s *AuthService) begin() *loginFlow {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil { // 终止上一个未完成的流程
		s.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.gen++
	s.cancel = cancel
	s.codeCh = make(chan string, 1)
	s.pwdCh = make(chan string, 1)
	return &loginFlow{ctx: ctx, gen: s.gen, codeCh: s.codeCh, pwdCh: s.pwdCh}
}

// finish 仅当自己仍是当前代际时才清理 cancel，
// 避免旧流程的异步收尾取消刚启动的新流程。
func (s *AuthService) finish(gen uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// isCurrent 判断代际是否仍是当前流程，事件发送前判定，避免旧流程污染新流程 UI。
func (s *AuthService) isCurrent(gen uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen == gen
}

// runCodeLogin 验证码登录（对应 ref/tdl/app/login/code.go，交互替换为事件+通道）。
func (s *AuthService) runCodeLogin(flow *loginFlow, phone string) {
	defer s.finish(flow.gen)
	ctx := flow.ctx

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

			flow := auth.NewFlow(&guiAuth{svc: s, phone: phone, codeCh: flow.codeCh, pwdCh: flow.pwdCh}, auth.SendCodeOptions{})
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

	if err != nil && !errors.Is(err, context.Canceled) && s.isCurrent(flow.gen) {
		logAuth.Errorf("验证码登录失败: %v", err)
		s.emitter.Emit(events.Login, events.LoginUpdate{Stage: "error", Error: err.Error()})
	}
}

// runQRLogin 二维码登录（对应 ref/tdl/app/login/qr.go，二维码渲染移至前端）。
func (s *AuthService) runQRLogin(flow *loginFlow) {
	defer s.finish(flow.gen)
	ctx := flow.ctx

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

				pwd, perr := s.waitPassword(ctx, flow.pwdCh)
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

	if err != nil && !errors.Is(err, context.Canceled) && s.isCurrent(flow.gen) {
		logAuth.Errorf("二维码登录失败: %v", err)
		s.emitter.Emit(events.Login, events.LoginUpdate{Stage: "error", Error: err.Error()})
	}
}

// waitPassword 等待前端提交二步验证密码，通道归属调用方自己的代际。
func (s *AuthService) waitPassword(ctx context.Context, ch chan string) (string, error) {
	s.emitter.Emit(events.Login, events.LoginUpdate{Stage: "need_password"})

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
	// 会话已切换：关闭旧账号的常驻连接，后续查询用新会话重建
	if s.OnSessionChanged != nil {
		s.OnSessionChanged()
	}
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
	// SVC-08：配置落盘失败不阻断登录流程，但必须留痕便于排查
	if err := s.cfg.Update(st); err != nil {
		logAuth.Warnf("保存登录态配置失败: %v", err)
	}
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
// 通过事件通知前端、代际归属的通道接收输入（对应 ref/tdl/app/login/code.go 的 termAuth）。
type guiAuth struct {
	svc    *AuthService
	phone  string
	codeCh chan string
	pwdCh  chan string
}

func (a *guiAuth) Phone(_ context.Context) (string, error) { return a.phone, nil }

func (a *guiAuth) Code(ctx context.Context, _ *tg.AuthSentCode) (string, error) {
	a.svc.emitter.Emit(events.Login, events.LoginUpdate{Stage: "need_code"})

	select {
	case code := <-a.codeCh:
		return code, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (a *guiAuth) Password(ctx context.Context) (string, error) {
	return a.svc.waitPassword(ctx, a.pwdCh)
}

func (a *guiAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("不支持注册 Telegram 账号")
}

func (a *guiAuth) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	return &auth.SignUpRequired{TermsOfService: tos}
}
