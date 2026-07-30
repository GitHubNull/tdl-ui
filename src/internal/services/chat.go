package services

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/core/storage"
	"github.com/iyear/tdl/core/tmedia"
	"github.com/iyear/tdl/pkg/kv"
	pkgtclient "github.com/iyear/tdl/pkg/tclient"

	"tdl-ui/internal/config"
	"tdl-ui/internal/engine"
	"tdl-ui/internal/logging"
)

var logChat = logging.L("chat")

// 对话类型（定义在 engine，与 ref/tdl/app/chat 一致）。
const (
	DialogPrivate = engine.DialogPrivate
	DialogGroup   = engine.DialogGroup
	DialogChannel = engine.DialogChannel
)

// 媒体类型。
const (
	KindVideo = "video"
	KindPhoto = "photo"
	KindAudio = "audio"
	KindFile  = "file"
)

// Dialog 对话视图（私聊用户 / 群组 / 频道）。
type Dialog struct {
	ID            int64  `json:"id"`
	Type          string `json:"type"`
	Title         string `json:"title"`
	Username      string `json:"username,omitempty"`
	UnreadCount   int    `json:"unreadCount"`
	LastMessageAt int64  `json:"lastMessageAt,omitempty"` // unix 秒
}

// MediaQuery 对话内媒体分页查询（游标式：OffsetID 为上一页最后一条消息 ID）。
type MediaQuery struct {
	DialogID   int64    `json:"dialogId"`
	DialogType string   `json:"dialogType"`
	OffsetID   int      `json:"offsetId"`
	OffsetDate int64    `json:"offsetDate"` // unix 秒；>0 且 OffsetID=0 时从该日期附近开始（月份跳转）
	Limit      int      `json:"limit"`
	Query      string   `json:"query"`   // 文件名/caption 包含，不区分大小写
	Kinds      []string `json:"kinds"`   // video/photo/audio/file，空=全部
	Exts       []string `json:"exts"`    // 扩展名白名单（小写不含点），空=不限
	MinSize    int64    `json:"minSize"` // 字节，0=不限
	MaxSize    int64    `json:"maxSize"` // 字节，0=不限
}

// MediaItem 对话内的一个媒体文件（Dialog 1 — N MediaItem）。
type MediaItem struct {
	DialogID  int64  `json:"dialogId"`
	MessageID int    `json:"messageId"`
	Name      string `json:"name"`
	Caption   string `json:"caption"`
	Size      int64  `json:"size"`
	MIME      string `json:"mime"`
	Kind      string `json:"kind"` // video/photo/audio/file
	Date      int64  `json:"date"` // unix 秒
	Width     int    `json:"width,omitempty"`  // 像素宽（短缺时为 0）
	Height    int    `json:"height,omitempty"` // 像素高（短缺时为 0）
	Thumb     string `json:"thumb,omitempty"` // 内嵌模糊占位图（data URI，可空）
}

// MediaPage 一页媒体查询结果。
type MediaPage struct {
	Items      []MediaItem `json:"items"`
	NextOffset int         `json:"nextOffset"` // 0=没有更多
}

var errNotLoggedIn = errors.New("尚未登录或会话已失效，请在「账号」页重新登录或重新导入 Desktop 会话")

// 查询超时与分页上限（SVC-17：集中具名常量便于调优）。
const (
	listDialogsTimeout = 120 * time.Second // 全量对话列表需多次分页往返
	listMediaTimeout   = 90 * time.Second  // 单页媒体扫描（含苛刻过滤时的 maxScan 扫描）
	mediaPageLimitMax  = 200               // 单页媒体条数上限
)

// connectTimeout 建连与授权校验的等待上限；var 便于测试缩短。
var connectTimeout = 30 * time.Second

// chatJob 投递给常驻客户端执行循环的查询任务。
type chatJob struct {
	// ctx 调用方上下文：worker 取任务后据此跳过已放弃的请求，并将取消传播到实际执行（SVC-01）
	ctx  context.Context
	fn   func(ctx context.Context, api *tg.Client) error
	done chan error
}

// chatQueue 任务队列种类：列表查询、缩略图、视频分段三条互不阻塞的通道。
type chatQueue int

const (
	queueList chatQueue = iota
	queueThumb
	queueVideo
)

// ChatService 对话与媒体查询。
// 复用常驻 Telegram 连接（懒启动、断线自动重建），避免每次翻页重建连接的开销；
// 列表查询经 jobs 通道串行化，缩略图/预览图走独立的 thumbJobs 队列（小并发度），
// 视频分段走 videoJobs 队列，避免单张大图预览或视频推流阻塞列表与缩略图加载（ARC-04）。
type ChatService struct {
	cfg *config.Manager
	kv  kv.Storage

	mu        sync.Mutex
	running   bool
	starting  chan struct{} // 非 nil 表示建连进行中，关闭即结束
	cancel    context.CancelFunc
	jobs      chan chatJob
	thumbJobs chan chatJob
	videoJobs chan chatJob
	dead      chan struct{}

	// 缩略图/预览图磁盘缓存（软件运行目录 cache/ 下）
	thumbs *thumbCache
	// 视频元信息与分段内存缓存（流式播放用）
	videos *videoCache
	// 视频分段磁盘缓存（临时目录 video/ 下，边下边播的持久层）
	videoDisk *videoDiskCache
	// 后台顺序预取控制器（单活跃视频）
	prefetch videoPrefetcher

	// runClient 建连并驱动任务循环，可注入以便测试；为 nil 时使用真实 Telegram 客户端。
	runClient func(ctx context.Context, ready chan<- error, jobs, thumbJobs, videoJobs <-chan chatJob)
}

// NewChatService 创建对话服务。
func NewChatService(cfg *config.Manager, kvs kv.Storage) *ChatService {
	return &ChatService{
		cfg:       cfg,
		kv:        kvs,
		thumbs:    newThumbCache(func() string { return cfg.CacheDir() }),
		videos:    newVideoCache(),
		videoDisk: newVideoDiskCache(func() string { return cfg.TempDir() }),
	}
}

// ClearThumbCache 清空缩略图/预览图磁盘缓存、视频内存缓存与视频磁盘分段（设置页「清空缓存」入口，SVC-18）。
func (s *ChatService) ClearThumbCache() error {
	if s.videos != nil {
		s.videos.Clear()
	}
	if s.videoDisk != nil {
		if err := s.videoDisk.Clear(); err != nil {
			return err
		}
	}
	return s.thumbs.Clear()
}

// Stop 关闭常驻连接（登出/重登后会话失效时调用），下次查询自动重建。
func (s *ChatService) Stop() {
	s.prefetch.stop() // 会话即将失效，后台预取一并取消
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		logChat.Infof("停止常驻 Telegram 连接")
		s.cancel()
	}
}

// StopAndWait 停止常驻连接并等待执行协程退出（应用关闭编排用）。
func (s *ChatService) StopAndWait(timeout time.Duration) {
	s.prefetch.stop()
	s.mu.Lock()
	if s.cancel != nil {
		logChat.Infof("停止常驻 Telegram 连接")
		s.cancel()
	}
	dead := s.dead
	s.mu.Unlock()

	if dead == nil {
		return
	}
	select {
	case <-dead:
	case <-time.After(timeout):
		logChat.Warnf("等待 Telegram 连接退出超时（%s），继续关闭流程", timeout)
	}
}

// ListDialogs 返回当前账号的全部对话（私聊 / 群组 / 频道）。
func (s *ChatService) ListDialogs() ([]Dialog, error) {
	logChat.Debugf("开始拉取对话列表")
	ctx, cancel := context.WithTimeout(context.Background(), listDialogsTimeout)
	defer cancel()

	out := make([]Dialog, 0, 256)
	err := s.invoke(ctx, func(ctx context.Context, api *tg.Client) error {
		kvd, err := s.kv.Open(engine.Namespace)
		if err != nil {
			return errors.Wrap(err, "open kv")
		}
		manager := peers.Options{Storage: storage.NewPeers(kvd)}.Build(api)

		dialogs, err := fetchDialogs(ctx, api, manager)
		if err != nil {
			return err
		}
		out = dialogs
		return nil
	})
	if err != nil {
		logChat.Errorf("拉取对话列表失败: %v", err)
		return nil, err
	}
	logChat.Infof("对话列表拉取完成，共 %d 个对话", len(out))
	return out, nil
}

// ListMedia 游标分页查询对话内的媒体文件（视频 / 图片 / 音频 / 文档）。
func (s *ChatService) ListMedia(q MediaQuery) (*MediaPage, error) {
	logChat.Debugf("查询媒体: dialog=%d type=%s offset=%d limit=%d kinds=%v query=%q", q.DialogID, q.DialogType, q.OffsetID, q.Limit, q.Kinds, q.Query)
	if q.DialogID == 0 {
		return nil, errors.New("缺少对话 ID")
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.Limit > mediaPageLimitMax { // SVC-16：超限钳到上限而非重置为默认值
		q.Limit = mediaPageLimitMax
	}

	ctx, cancel := context.WithTimeout(context.Background(), listMediaTimeout)
	defer cancel()

	var page *MediaPage
	err := s.invoke(ctx, func(ctx context.Context, api *tg.Client) error {
		kvd, err := s.kv.Open(engine.Namespace)
		if err != nil {
			return errors.Wrap(err, "open kv")
		}
		manager := peers.Options{Storage: storage.NewPeers(kvd)}.Build(api)

		inputPeer, err := engine.ResolveDialogPeer(ctx, manager, q.DialogID, q.DialogType)
		if err != nil {
			return err
		}

		page, err = scanMedia(ctx, api, inputPeer, q)
		return err
	})
	if err != nil {
		logChat.Errorf("查询媒体失败: dialog=%d err=%v", q.DialogID, err)
		return nil, err
	}
	logChat.Debugf("媒体查询完成: dialog=%d items=%d nextOffset=%d", q.DialogID, len(page.Items), page.NextOffset)
	return page, nil
}

// ---- 常驻客户端执行器 ----

// invoke 将列表/查询闭包投递到串行队列执行，带超时与断线感知。
func (s *ChatService) invoke(ctx context.Context, fn func(ctx context.Context, api *tg.Client) error) error {
	return s.submit(ctx, queueList, fn)
}

// invokeThumb 将缩略图/预览图拉取投递到独立队列，不与列表查询互相阻塞。
func (s *ChatService) invokeThumb(ctx context.Context, fn func(ctx context.Context, api *tg.Client) error) error {
	return s.submit(ctx, queueThumb, fn)
}

// invokeVideo 将视频元信息解析/分段拉取投递到独立队列，不与列表、缩略图互相阻塞。
func (s *ChatService) invokeVideo(ctx context.Context, fn func(ctx context.Context, api *tg.Client) error) error {
	return s.submit(ctx, queueVideo, fn)
}

func (s *ChatService) submit(ctx context.Context, queue chatQueue, fn func(ctx context.Context, api *tg.Client) error) error {
	if err := s.ensureStarted(); err != nil {
		return err
	}

	s.mu.Lock()
	jobs, dead := s.jobs, s.dead
	switch queue {
	case queueThumb:
		jobs = s.thumbJobs
	case queueVideo:
		jobs = s.videoJobs
	case queueList:
	}
	s.mu.Unlock()

	j := chatJob{ctx: ctx, fn: fn, done: make(chan error, 1)}
	select {
	case jobs <- j:
	case <-dead:
		return errors.New("Telegram 连接已断开，请重试")
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-j.done:
		return err
	case <-dead:
		return errors.New("Telegram 连接已断开，请重试")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ensureStarted 懒启动常驻客户端。
// 不在持锁状态下等待建连结果，避免与执行协程的收尾逻辑互锁；
// 并发调用时仅有一个建连流程，其余调用等待其结果后重新检查。
func (s *ChatService) ensureStarted() error {
	for {
		s.mu.Lock()
		if s.running {
			s.mu.Unlock()
			return nil
		}
		if ch := s.starting; ch != nil {
			s.mu.Unlock()
			<-ch // 其他调用正在建连，结束后重新检查状态
			continue
		}
		starting := make(chan struct{})
		s.starting = starting
		s.mu.Unlock()

		err := s.start()

		s.mu.Lock()
		s.starting = nil
		s.mu.Unlock()
		close(starting)
		return err
	}
}

// start 建连 → 等待授权校验结果（带超时）→ 发布运行状态。
func (s *ChatService) start() error {
	logChat.Infof("启动常驻 Telegram 连接")
	runCtx, cancel := context.WithCancel(context.Background())
	jobs := make(chan chatJob)
	thumbJobs := make(chan chatJob)
	videoJobs := make(chan chatJob)
	dead := make(chan struct{})
	ready := make(chan error, 1)

	run := s.runClient
	if run == nil {
		run = s.runTelegramClient
	}

	go func() {
		defer cancel()
		run(runCtx, ready, jobs, thumbJobs, videoJobs)

		// 先宣告退出再抢锁复位，确保任何等待 dead 的一方不会与本协程互锁
		close(dead)
		s.mu.Lock()
		if s.dead == dead { // 仅清理本次连接发布的状态
			s.running = false
			s.cancel = nil
		}
		s.mu.Unlock()
	}()

	select {
	case err := <-ready:
		if err != nil {
			logChat.Errorf("Telegram 建连失败: %v", err)
			cancel()
			return err
		}
	case <-dead:
		cancel()
		// 优先拿到具体错误（ready 与 dead 可能同时就绪）
		select {
		case err := <-ready:
			if err != nil {
				return err
			}
		default:
		}
		return errors.New("Telegram 连接意外退出，请重试")
	case <-time.After(connectTimeout):
		cancel()
		logChat.Errorf("连接 Telegram 超时（%s）", connectTimeout)
		return errors.New("连接 Telegram 超时，请检查网络或在「设置」中配置代理")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-dead: // 就绪后立刻断开（如网络抖动）
		return errors.New("Telegram 连接已断开，请重试")
	default:
	}
	s.running = true
	s.cancel = cancel
	s.jobs = jobs
	s.thumbJobs = thumbJobs
	s.videoJobs = videoJobs
	s.dead = dead
	logChat.Infof("Telegram 连接就绪")
	return nil
}

// runTelegramClient 真实客户端：建连 → 校验登录态 → 执行查询任务直到 ctx 取消。
// 列表查询单 worker 串行；缩略图队列 2 个 worker，单张大图预览最多占用一个；
// 视频队列 2 个 worker，单次仅拉取一个 1MB 分段，暂停播放不会长期占用 worker。
// 就绪前的任何错误都会写入 ready，保证调用方能拿到具体原因。
func (s *ChatService) runTelegramClient(ctx context.Context, ready chan<- error, jobs, thumbJobs, videoJobs <-chan chatJob) {
	kvd, err := s.kv.Open(engine.Namespace)
	if err != nil {
		ready <- errors.Wrap(err, "open kv")
		return
	}

	c, err := pkgtclient.New(ctx, pkgtclient.Options{
		KV:               kvd,
		Proxy:            s.cfg.Get().Proxy,
		ReconnectTimeout: reconnectTimeout,
	}, false)
	if err != nil {
		ready <- errors.Wrap(err, "create client")
		return
	}

	err = c.Run(ctx, func(ctx context.Context) error {
		st, err := c.Auth().Status(ctx)
		if err != nil {
			ready <- errors.Wrap(err, "查询登录状态失败")
			return err
		}
		if !st.Authorized {
			ready <- errNotLoggedIn
			return errNotLoggedIn
		}
		ready <- nil

		// worker 消费指定队列：跳过调用方已放弃的任务，并把调用方取消传播到实际执行（SVC-01）
		worker := func(queue <-chan chatJob) {
			for {
				select {
				case j := <-queue:
					if j.ctx != nil && j.ctx.Err() != nil {
						j.done <- j.ctx.Err() // 调用方已超时/放弃，不再消耗 API 配额
						continue
					}
					jctx, jcancel := mergeContext(ctx, j.ctx)
					j.done <- j.fn(jctx, c.API())
					jcancel()
				case <-ctx.Done():
					return
				}
			}
		}

		var wg sync.WaitGroup
		wg.Add(4)
		go func() { defer wg.Done(); worker(thumbJobs) }()
		go func() { defer wg.Done(); worker(thumbJobs) }()
		go func() { defer wg.Done(); worker(videoJobs) }()
		go func() { defer wg.Done(); worker(videoJobs) }()
		worker(jobs)
		wg.Wait() // 确保所有 worker 退出后再关闭客户端
		return nil
	})
	if err != nil {
		// 就绪前建连失败（如直连被墙/代理不可用）：尽力把原因送给等待方
		select {
		case ready <- errors.Wrap(err, "连接 Telegram 失败"):
		default:
		}
	}
}

// mergeContext 返回同时受运行 ctx 与调用方 ctx 取消控制的派生 ctx。
func mergeContext(base, caller context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(base)
	if caller == nil {
		return ctx, cancel
	}
	stop := context.AfterFunc(caller, cancel)
	return ctx, func() { stop(); cancel() }
}

// ---- 对话列表 ----

// peerKey 对话去重键（user/chat/channel 的 ID 可能跨类型碰撞）。
type peerKey struct {
	typ string
	id  int64
}

// fetchDialogs 手动分页拉取全部对话（适配自 ref/tdl/app/chat/ls.go 的容错翻页：
// 跳过已删除/无权限的失效对话，而不是整体失败），并把 access hash 落盘供后续解析。
func fetchDialogs(ctx context.Context, api *tg.Client, manager *peers.Manager) ([]Dialog, error) {
	const batchSize = 100

	var (
		result     []Dialog
		offsetID   int
		offsetDate int
		offsetPeer tg.InputPeerClass = &tg.InputPeerEmpty{}
		seen                         = make(map[peerKey]struct{})
	)

	for {
		res, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetDate: offsetDate,
			OffsetID:   offsetID,
			OffsetPeer: offsetPeer,
			Limit:      batchSize,
		})
		if err != nil {
			return nil, errors.Wrap(err, "获取对话列表失败")
		}

		var (
			dialogsSlice []tg.DialogClass
			msgSlice     []tg.MessageClass
			users        []tg.UserClass
			chats        []tg.ChatClass
		)
		switch d := res.(type) {
		case *tg.MessagesDialogs:
			dialogsSlice, msgSlice, users, chats = d.Dialogs, d.Messages, d.Users, d.Chats
		case *tg.MessagesDialogsSlice:
			dialogsSlice, msgSlice, users, chats = d.Dialogs, d.Messages, d.Users, d.Chats
		case *tg.MessagesDialogsNotModified:
			return result, nil
		default:
			return result, nil
		}

		if len(dialogsSlice) == 0 {
			break
		}

		entities := newEntities(users, chats)
		messageMap := newMessageMap(msgSlice)

		// SVC-05：整页 users/chats 一次性批量落盘 access hash（单次 kv 事务），
		// 失败记日志，后续 ListMedia 解析 peer 失败时可从此追溯根因。
		if err := manager.Apply(ctx, users, chats); err != nil {
			logChat.Warnf("缓存对话 access hash 失败: %v", err)
		}

		for _, dc := range dialogsSlice {
			dialog, ok := dc.(*tg.Dialog)
			if !ok {
				continue
			}

			key := keyOfPeer(dialog.Peer)
			if key.id == 0 {
				continue
			}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}

			d := buildDialog(dialog, entities, messageMap)
			if d == nil { // 失效或不支持的对话，跳过
				continue
			}

			result = append(result, *d)
		}

		// 计算下一页游标
		last, ok := dialogsSlice[len(dialogsSlice)-1].(*tg.Dialog)
		if !ok {
			break
		}
		nextPeer, ok := nextOffsetPeer(entities, last.Peer)
		if !ok {
			break // 缺少 access hash，无法继续翻页
		}
		offsetPeer = nextPeer
		offsetID = last.TopMessage
		if m, found := messageMap[last.TopMessage]; found {
			offsetDate = m.GetDate()
		} else {
			offsetDate = 0
		}

		if len(dialogsSlice) < batchSize {
			break
		}
	}

	return result, nil
}

// newEntities 把响应里的用户/群组/频道组装成实体索引。
func newEntities(users []tg.UserClass, chats []tg.ChatClass) peer.Entities {
	userMap := make(map[int64]*tg.User, len(users))
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			userMap[user.ID] = user
		}
	}

	chatMap := make(map[int64]*tg.Chat, len(chats))
	channelMap := make(map[int64]*tg.Channel, len(chats))
	for _, c := range chats {
		switch chat := c.(type) {
		case *tg.Chat:
			chatMap[chat.ID] = chat
		case *tg.Channel:
			channelMap[chat.ID] = chat
		}
	}

	return peer.NewEntities(userMap, chatMap, channelMap)
}

// newMessageMap 建立消息 ID 索引（取最近消息时间用）。
func newMessageMap(messages []tg.MessageClass) map[int]tg.NotEmptyMessage {
	m := make(map[int]tg.NotEmptyMessage, len(messages))
	for _, msg := range messages {
		switch v := msg.(type) {
		case *tg.Message:
			m[v.ID] = v
		case *tg.MessageService:
			m[v.ID] = v
		}
	}
	return m
}

func keyOfPeer(p tg.PeerClass) peerKey {
	switch v := p.(type) {
	case *tg.PeerUser:
		return peerKey{DialogPrivate, v.UserID}
	case *tg.PeerChat:
		return peerKey{DialogGroup, v.ChatID}
	case *tg.PeerChannel:
		return peerKey{DialogChannel, v.ChannelID}
	}
	return peerKey{}
}

// buildDialog 由原始对话与实体构建视图；实体缺失或类型不支持时返回 nil。
func buildDialog(d *tg.Dialog, entities peer.Entities, messageMap map[int]tg.NotEmptyMessage) *Dialog {
	out := &Dialog{UnreadCount: d.UnreadCount}
	if m, ok := messageMap[d.TopMessage]; ok {
		out.LastMessageAt = int64(m.GetDate())
	}

	switch p := d.Peer.(type) {
	case *tg.PeerUser:
		u, ok := entities.User(p.UserID)
		if !ok {
			return nil
		}
		out.ID = u.ID
		out.Type = DialogPrivate
		out.Title = visibleName(u.FirstName, u.LastName)
		out.Username = u.Username
	case *tg.PeerChat:
		c, ok := entities.Chat(p.ChatID)
		if !ok {
			return nil
		}
		out.ID = c.ID
		out.Type = DialogGroup
		out.Title = c.Title
	case *tg.PeerChannel:
		c, ok := entities.Channel(p.ChannelID)
		if !ok {
			return nil
		}
		out.ID = c.ID
		out.Title = c.Title
		out.Username = c.Username
		switch {
		case c.Broadcast:
			out.Type = DialogChannel
		case c.Megagroup || c.Gigagroup:
			out.Type = DialogGroup
		default:
			return nil // 不支持的频道形态
		}
	default:
		return nil
	}

	if out.Title == "" {
		out.Title = "-"
	}
	return out
}

// nextOffsetPeer 生成翻页游标所需的输入 peer（带 access hash）。
func nextOffsetPeer(entities peer.Entities, p tg.PeerClass) (tg.InputPeerClass, bool) {
	switch v := p.(type) {
	case *tg.PeerUser:
		if u, ok := entities.User(v.UserID); ok {
			return &tg.InputPeerUser{UserID: v.UserID, AccessHash: u.AccessHash}, true
		}
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: v.ChatID}, true
	case *tg.PeerChannel:
		if c, ok := entities.Channel(v.ChannelID); ok {
			return &tg.InputPeerChannel{ChannelID: v.ChannelID, AccessHash: c.AccessHash}, true
		}
	}
	return nil, false
}

func visibleName(first, last string) string {
	switch {
	case first == "":
		return last
	case last == "":
		return first
	default:
		return first + " " + last
	}
}

// ---- 媒体查询 ----

// scanMedia 迭代对话消息，按条件收集一页媒体项。
// 单次调用最多扫描 maxScan 条消息，防止苛刻过滤条件下调用过久。
func scanMedia(ctx context.Context, api *tg.Client, inputPeer tg.InputPeerClass, q MediaQuery) (*MediaPage, error) {
	const maxScan = 2000

	q.Exts = normalizeExts(q.Exts)
	q.Kinds = normalizeKinds(q.Kinds)

	// 月份跳转：先把日期游标解析为消息 ID 游标，再走统一分页管线
	if q.OffsetDate > 0 && q.OffsetID == 0 {
		id, err := resolveOffsetDate(ctx, api, inputPeer, q.OffsetDate)
		if err != nil {
			return nil, err
		}
		q.OffsetID = id
	}

	it := newMediaIterator(api, inputPeer, q)

	page := &MediaPage{Items: make([]MediaItem, 0, q.Limit)}
	scanned, lastID := 0, 0
	exhausted := true

	for it.Next(ctx) {
		m, ok := it.Value().Msg.(*tg.Message)
		if !ok {
			continue
		}
		scanned++
		lastID = m.ID

		if item, ok := toMediaItem(q.DialogID, m); ok && matchesFilter(q, item) {
			page.Items = append(page.Items, item)
		}
		if len(page.Items) >= q.Limit || scanned >= maxScan {
			exhausted = false
			break
		}
	}
	if err := it.Err(); err != nil {
		return nil, errors.Wrap(err, "读取消息历史失败")
	}

	if !exhausted {
		page.NextOffset = lastID
	}
	return page, nil
}

// resolveOffsetDate 把日期游标解析为消息 ID 游标（含该日期附近的消息）：
// GetHistory{OffsetDate, Limit:1} 返回日期不晚于 OffsetDate 的最新一条，+1 使其被分页包含。
func resolveOffsetDate(ctx context.Context, api *tg.Client, inputPeer tg.InputPeerClass, date int64) (int, error) {
	res, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:       inputPeer,
		OffsetDate: int(date),
		Limit:      1,
	})
	if err != nil {
		return 0, errors.Wrap(err, "解析日期游标失败")
	}
	msgs, ok := res.AsModified()
	if !ok {
		return 1, nil
	}
	list := msgs.GetMessages()
	if len(list) == 0 {
		return 1, nil // 该日期之前无消息：OffsetID=1 使分页结果为空
	}
	return list[0].GetID() + 1, nil
}

// newMediaIterator 按类型条件选择消息源：
// 能映射到服务端过滤器的走 messages.search（减少扫描量），否则回退完整历史迭代。
func newMediaIterator(api *tg.Client, inputPeer tg.InputPeerClass, q MediaQuery) *messages.Iterator {
	const batchSize = 100

	qb := query.Messages(api)
	if f := serverFilter(q.Kinds); f != nil {
		return qb.Search(inputPeer).Filter(f).OffsetID(q.OffsetID).BatchSize(batchSize).Iter()
	}
	return qb.GetHistory(inputPeer).OffsetID(q.OffsetID).BatchSize(batchSize).Iter()
}

// serverFilter 计算服务端媒体类型过滤器；返回 nil 表示无法用服务端过滤（回退历史）。
func serverFilter(kinds []string) tg.MessagesFilterClass {
	if len(kinds) == 0 {
		return nil
	}

	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}

	switch {
	case len(set) == 1 && set[KindPhoto]:
		return &tg.InputMessagesFilterPhotos{}
	case len(set) == 1 && set[KindVideo]:
		return &tg.InputMessagesFilterVideo{}
	case len(set) == 2 && set[KindPhoto] && set[KindVideo]:
		return &tg.InputMessagesFilterPhotoVideo{}
	case !set[KindPhoto]:
		return &tg.InputMessagesFilterDocument{} // video/audio/file 任意组合均属文档
	default:
		return nil // photo 与文档类混合，只能回退历史迭代
	}
}

// toMediaItem 从消息提取媒体项；非图片/文档消息返回 false。
func toMediaItem(dialogID int64, m *tg.Message) (MediaItem, bool) {
	media, ok := m.GetMedia()
	if !ok {
		return MediaItem{}, false
	}

	switch md := media.(type) {
	case *tg.MessageMediaPhoto:
		item, ok := tmedia.GetPhotoInfo(md)
		if !ok {
			return MediaItem{}, false
		}
		out := MediaItem{
			DialogID:  dialogID,
			MessageID: m.ID,
			Name:      item.Name,
			Caption:   m.Message,
			Size:      item.Size,
			MIME:      "image/jpeg",
			Kind:      KindPhoto,
			Date:      int64(m.Date),
		}
		if p, ok := md.Photo.(*tg.Photo); ok {
			out.Thumb = strippedThumbURI(p.Sizes)
			out.Width, out.Height = photoDims(p.Sizes)
		}
		return out, true
	case *tg.MessageMediaDocument:
		doc, ok := md.Document.(*tg.Document)
		if !ok {
			return MediaItem{}, false
		}
		w, h := docDims(doc)
		return MediaItem{
			DialogID:  dialogID,
			MessageID: m.ID,
			Name:      tmedia.GetDocumentName(doc),
			Caption:   m.Message,
			Size:      doc.Size,
			MIME:      doc.MimeType,
			Kind:      kindOfDocument(doc),
			Date:      int64(m.Date),
			Width:     w,
			Height:    h,
			Thumb:     strippedThumbURI(doc.Thumbs),
		}, true
	}
	return MediaItem{}, false
}

// photoDims 取照片最大真实尺寸的像素宽高。
func photoDims(sizes []tg.PhotoSizeClass) (w, h int) {
	best := 0
	for _, sc := range sizes {
		var cw, ch int
		switch v := sc.(type) {
		case *tg.PhotoSize:
			cw, ch = v.W, v.H
		case *tg.PhotoSizeProgressive:
			cw, ch = v.W, v.H
		}
		if a := cw * ch; a > best {
			best, w, h = a, cw, ch
		}
	}
	return w, h
}

// docDims 从文档属性提取像素宽高（视频 / 图片文档）。
func docDims(doc *tg.Document) (w, h int) {
	for _, a := range doc.Attributes {
		switch v := a.(type) {
		case *tg.DocumentAttributeVideo:
			return v.W, v.H
		case *tg.DocumentAttributeImageSize:
			return v.W, v.H
		}
	}
	return 0, 0
}

// kindOfDocument 按 MIME 与文档属性判定媒体类型。
func kindOfDocument(doc *tg.Document) string {
	return classifyKind(doc.MimeType,
		hasAttr[*tg.DocumentAttributeVideo](doc),
		hasAttr[*tg.DocumentAttributeAudio](doc))
}

// classifyKind 类型判定纯函数：视频属性或 video/* → video；音频属性或 audio/* → audio；其余 → file。
func classifyKind(mime string, videoAttr, audioAttr bool) string {
	switch {
	case videoAttr || strings.HasPrefix(mime, "video/"):
		return KindVideo
	case audioAttr || strings.HasPrefix(mime, "audio/"):
		return KindAudio
	default:
		return KindFile
	}
}

func hasAttr[T tg.DocumentAttributeClass](doc *tg.Document) bool {
	for _, a := range doc.Attributes {
		if _, ok := a.(T); ok {
			return true
		}
	}
	return false
}

// matchesFilter 服务端逐条过滤：类型 / 扩展名 / 大小范围 / 关键词（文件名+caption 包含）。
func matchesFilter(q MediaQuery, it MediaItem) bool {
	if len(q.Kinds) > 0 && !contains(q.Kinds, it.Kind) {
		return false
	}
	if len(q.Exts) > 0 {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(it.Name), "."))
		if !contains(q.Exts, ext) {
			return false
		}
	}
	if q.MinSize > 0 && it.Size < q.MinSize {
		return false
	}
	if q.MaxSize > 0 && it.Size > q.MaxSize {
		return false
	}
	if needle := strings.ToLower(strings.TrimSpace(q.Query)); needle != "" {
		hay := strings.ToLower(it.Name + "\n" + it.Caption)
		if !strings.Contains(hay, needle) {
			return false
		}
	}
	return true
}

// normalizeExts 扩展名规范化：小写、去点、去空白、去重、去空。
func normalizeExts(exts []string) []string {
	out := make([]string, 0, len(exts))
	seen := make(map[string]struct{}, len(exts))
	for _, e := range exts {
		e = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(e, ".")))
		if e == "" {
			continue
		}
		if _, ok := seen[e]; ok {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	return out
}

// normalizeKinds 类型条件规范化：仅保留合法值并去重。
func normalizeKinds(kinds []string) []string {
	out := make([]string, 0, len(kinds))
	seen := make(map[string]struct{}, len(kinds))
	for _, k := range kinds {
		k = strings.ToLower(strings.TrimSpace(k))
		switch k {
		case KindVideo, KindPhoto, KindAudio, KindFile:
		default:
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
