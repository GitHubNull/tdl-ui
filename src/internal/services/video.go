package services

import (
	"container/list"
	"context"
	"mime"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/core/storage"
	"github.com/iyear/tdl/core/tmedia"
	"github.com/iyear/tdl/core/util/tutil"

	"tdl-ui/internal/engine"
)

// 视频流式播放参数（SVC-17 风格：集中具名常量便于调优）。
const (
	// videoPartSize Telegram upload.getFile 单段大小；1MB 天然满足
	// offset/limit 的 4096 对齐且不跨 1MB 边界的约束。
	videoPartSize = 1 << 20
	// videoRangeMax 单次 HTTP 响应最大字节数：超出部分由浏览器续发 Range 请求，
	// 使每个请求都短时完成（流式 + 懒加载，避免单请求长时间占用队列）。
	videoRangeMax = 4 << 20
	// videoPartCacheMax 分段内存缓存条数上限（≈48MB），回拖 seek 免重复拉流。
	videoPartCacheMax = 48
	// videoMetaTTL 元信息缓存有效期，兜底 file_reference 过期。
	videoMetaTTL = 30 * time.Minute
	// videoFetchTimeout 单次元信息解析 / 单段拉取的等待上限。
	videoFetchTimeout = 30 * time.Second
)

// errNotVideo 该消息不含可播放视频（handler 映射为 404）。
var errNotVideo = errors.New("该消息没有可播放的视频")

// videoMeta 播放所需的视频文档元信息。
type videoMeta struct {
	Size int64
	MIME string
	Name string
	Loc  *tg.InputDocumentFileLocation
}

// ---- 元信息与分段缓存 ----

type videoMetaEntry struct {
	meta videoMeta
	at   time.Time
}

type videoPartEntry struct {
	key  string
	data []byte
}

// videoCache 进程内视频缓存：元信息按 TTL 失效，分段按 LRU 淘汰。
// 仅存内存，不落磁盘（视频体积远大于缩略图，落盘会与下载目录职责重叠）。
type videoCache struct {
	mu    sync.Mutex
	meta  map[string]videoMetaEntry
	parts map[string]*list.Element
	order *list.List // front 为最近使用
}

func newVideoCache() *videoCache {
	return &videoCache{
		meta:  make(map[string]videoMetaEntry),
		parts: make(map[string]*list.Element),
		order: list.New(),
	}
}

func (c *videoCache) getMeta(key string) (videoMeta, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.meta[key]
	if !ok || time.Since(e.at) > videoMetaTTL {
		delete(c.meta, key)
		return videoMeta{}, false
	}
	return e.meta, true
}

func (c *videoCache) putMeta(key string, m videoMeta) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.meta[key] = videoMetaEntry{meta: m, at: time.Now()}
}

// dropMeta 丢弃元信息（file_reference 失效时下次请求重新解析）。
func (c *videoCache) dropMeta(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.meta, key)
}

func (c *videoCache) getPart(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.parts[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*videoPartEntry).data, true
}

func (c *videoCache) putPart(key string, b []byte) {
	if len(b) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.parts[key]; ok {
		el.Value.(*videoPartEntry).data = b
		c.order.MoveToFront(el)
		return
	}
	c.parts[key] = c.order.PushFront(&videoPartEntry{key: key, data: b})
	for c.order.Len() > videoPartCacheMax {
		last := c.order.Back()
		if last == nil {
			break
		}
		c.order.Remove(last)
		delete(c.parts, last.Value.(*videoPartEntry).key)
	}
}

// Clear 清空全部元信息与分段（设置页「清空缓存」入口）。
func (c *videoCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.meta = make(map[string]videoMetaEntry)
	c.parts = make(map[string]*list.Element)
	c.order.Init()
}

func videoKey(dialogID int64, messageID int) string {
	return strconv.FormatInt(dialogID, 10) + ":" + strconv.Itoa(messageID)
}

func videoPartKey(dialogID int64, messageID int, partIdx int64) string {
	return videoKey(dialogID, messageID) + ":" + strconv.FormatInt(partIdx, 10)
}

// ---- 元信息解析与分段拉取 ----

// VideoMeta 解析消息内视频文档的播放元信息（带 TTL 缓存）；
// 消息不含视频时返回 errNotVideo。
func (s *ChatService) VideoMeta(dialogID int64, dialogType string, messageID int) (videoMeta, error) {
	if dialogID == 0 || messageID == 0 {
		return videoMeta{}, errors.New("缺少对话或消息 ID")
	}
	key := videoKey(dialogID, messageID)
	if m, ok := s.videos.getMeta(key); ok {
		return m, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), videoFetchTimeout)
	defer cancel()

	var out videoMeta
	err := s.invokeVideo(ctx, func(ctx context.Context, api *tg.Client) error {
		kvd, err := s.kv.Open(engine.Namespace)
		if err != nil {
			return errors.Wrap(err, "open kv")
		}
		manager := peers.Options{Storage: storage.NewPeers(kvd)}.Build(api)

		inputPeer, err := engine.ResolveDialogPeer(ctx, manager, dialogID, dialogType)
		if err != nil {
			return err
		}

		msg, err := tutil.GetSingleMessage(ctx, api, inputPeer, messageID)
		if err != nil {
			return errors.Wrap(err, "获取消息失败")
		}

		m, ok := videoMetaOf(msg)
		if !ok {
			return errNotVideo
		}
		out = m
		return nil
	})
	if err != nil {
		return videoMeta{}, err
	}

	s.videos.putMeta(key, out)
	return out, nil
}

// videoMetaOf 从消息提取视频文档元信息；非视频消息返回 false。
func videoMetaOf(msg *tg.Message) (videoMeta, bool) {
	media, ok := msg.GetMedia()
	if !ok {
		return videoMeta{}, false
	}
	md, ok := media.(*tg.MessageMediaDocument)
	if !ok {
		return videoMeta{}, false
	}
	doc, ok := md.Document.(*tg.Document)
	if !ok {
		return videoMeta{}, false
	}
	if kindOfDocument(doc) != KindVideo {
		return videoMeta{}, false
	}
	return videoMeta{
		Size: doc.Size,
		MIME: doc.MimeType,
		Name: tmedia.GetDocumentName(doc),
		Loc: &tg.InputDocumentFileLocation{
			ID:            doc.ID,
			AccessHash:    doc.AccessHash,
			FileReference: doc.FileReference,
		},
	}, true
}

// videoPart 取第 partIdx 段（1MB 对齐）视频字节，查找链：内存 LRU → 磁盘分段 → Telegram 拉取；
// 拉取成功后同时回写两级缓存（重看/回拖免重复拉流）。
// file_reference 失效时丢弃元信息缓存，使下次请求重新解析。
func (s *ChatService) videoPart(ctx context.Context, dialogID int64, messageID int, meta videoMeta, partIdx int64) ([]byte, error) {
	key := videoPartKey(dialogID, messageID, partIdx)
	if b, ok := s.videos.getPart(key); ok {
		return b, nil
	}
	if s.videoDisk != nil {
		if b, ok := s.videoDisk.get(dialogID, messageID, partIdx); ok {
			s.videos.putPart(key, b) // 磁盘命中同步提升到内存，后续连续读免磁盘 IO
			return b, nil
		}
	}

	fetchCtx, cancel := context.WithTimeout(ctx, videoFetchTimeout)
	defer cancel()

	var out []byte
	err := s.invokeVideo(fetchCtx, func(ctx context.Context, api *tg.Client) error {
		res, err := api.UploadGetFile(ctx, &tg.UploadGetFileRequest{
			Location: meta.Loc,
			Offset:   partIdx * videoPartSize,
			Limit:    videoPartSize,
		})
		if err != nil {
			return errors.Wrap(err, "拉取视频分段失败")
		}
		f, ok := res.(*tg.UploadFile)
		if !ok {
			return errors.Errorf("不支持的响应类型: %T", res)
		}
		out = f.Bytes
		return nil
	})
	if err != nil {
		if isFileRefExpired(err) {
			s.videos.dropMeta(videoKey(dialogID, messageID))
		}
		return nil, err
	}

	s.videos.putPart(key, out)
	if s.videoDisk != nil {
		s.videoDisk.put(dialogID, messageID, partIdx, out)
	}
	return out, nil
}

// ---- 后台顺序预取（Telegram 式边下边播） ----

// videoPrefetchYield 两次网络拉段之间的让步间隔，
// 避免预取长期占满 queueVideo 拖慢交互式 Range 请求。
const videoPrefetchYield = 50 * time.Millisecond

// videoPrefetcher 后台预取控制器：单活跃视频，切换视频即取消上一个预取协程。
type videoPrefetcher struct {
	mu     sync.Mutex
	key    string
	cancel context.CancelFunc
}

// stop 取消当前预取（关闭预览/会话变更时调用）。
func (p *videoPrefetcher) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
		p.key = ""
	}
}

// StopVideoPrefetch 停止后台顺序预取（前端关闭预览时调用，避免继续拉流耗费流量）。
func (s *ChatService) StopVideoPrefetch() {
	s.prefetch.stop()
}

// startVideoPrefetch 从 fromPart 起后台顺序预取到文件末尾；
// 同一视频重复触发不重启（seek 回拖依赖磁盘命中），切换视频时取消旧预取。
func (s *ChatService) startVideoPrefetch(dialogID int64, messageID int, meta videoMeta, fromPart int64) {
	key := videoKey(dialogID, messageID)

	s.prefetch.mu.Lock()
	if s.prefetch.key == key && s.prefetch.cancel != nil {
		s.prefetch.mu.Unlock()
		return
	}
	if s.prefetch.cancel != nil {
		s.prefetch.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.prefetch.key = key
	s.prefetch.cancel = cancel
	s.prefetch.mu.Unlock()

	go s.runVideoPrefetch(ctx, dialogID, messageID, meta, fromPart)
}

// runVideoPrefetch 预取执行体：已缓存分段直接跳过，网络拉段之间让步；
// 出错或被取消即退出（下次播放请求会重新触发）。
func (s *ChatService) runVideoPrefetch(ctx context.Context, dialogID int64, messageID int, meta videoMeta, fromPart int64) {
	totalParts := (meta.Size + videoPartSize - 1) / videoPartSize
	logMedia.Debugf("视频预取启动: dialog=%d msg=%d from=%d total=%d", dialogID, messageID, fromPart, totalParts)

	for idx := fromPart; idx < totalParts; idx++ {
		if ctx.Err() != nil {
			return
		}
		// 两级缓存已有的分段快进跳过，不占用拉取队列也不让步
		if _, ok := s.videos.getPart(videoPartKey(dialogID, messageID, idx)); ok {
			continue
		}
		if s.videoDisk != nil && s.videoDisk.has(dialogID, messageID, idx) {
			continue
		}

		if _, err := s.videoPart(ctx, dialogID, messageID, meta, idx); err != nil {
			logMedia.Debugf("视频预取中止: dialog=%d msg=%d part=%d err=%v", dialogID, messageID, idx, err)
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(videoPrefetchYield):
		}
	}
	logMedia.Debugf("视频预取完成: dialog=%d msg=%d parts=%d", dialogID, messageID, totalParts)
}

// isFileRefExpired 判定是否为 file_reference 失效类错误（需重新解析消息）。
func isFileRefExpired(err error) bool {
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "FILE_REFERENCE")
}

// ---- Range 计算（纯函数，便于单测） ----

// rangeStatus Range 请求头的解析结论。
type rangeStatus int

const (
	rangeNone          rangeStatus = iota // 无 Range 或语法非法：按全量响应
	rangePartial                          // 有效单段范围：206 响应
	rangeUnsatisfiable                    // 语法合法但越界：416 响应
)

// parseRangeHeader 解析单段 Range 头（bytes=a-b / bytes=a- / bytes=-n）。
// 多段范围与非 bytes 单位一律按无 Range 处理（回全量，浏览器可自行续发）。
func parseRangeHeader(h string, size int64) (start, end int64, status rangeStatus) {
	h = strings.TrimSpace(h)
	if h == "" || size <= 0 {
		return 0, 0, rangeNone
	}
	const prefix = "bytes="
	if !strings.HasPrefix(strings.ToLower(h), prefix) {
		return 0, 0, rangeNone
	}
	spec := strings.TrimSpace(h[len(prefix):])
	if spec == "" || strings.Contains(spec, ",") {
		return 0, 0, rangeNone
	}

	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return 0, 0, rangeNone
	}
	first := strings.TrimSpace(spec[:dash])
	last := strings.TrimSpace(spec[dash+1:])

	if first == "" { // bytes=-n：末尾 n 字节
		n, err := strconv.ParseInt(last, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, rangeNone
		}
		if n > size {
			n = size
		}
		return size - n, size - 1, rangePartial
	}

	s, err := strconv.ParseInt(first, 10, 64)
	if err != nil || s < 0 {
		return 0, 0, rangeNone
	}
	if s >= size {
		return 0, 0, rangeUnsatisfiable
	}
	if last == "" { // bytes=a-：到文件末尾
		return s, size - 1, rangePartial
	}
	e, err := strconv.ParseInt(last, 10, 64)
	if err != nil || e < s {
		return 0, 0, rangeNone
	}
	if e >= size {
		e = size - 1
	}
	return s, e, rangePartial
}

// clampRange 把 [start,end] 收敛进文件范围，并把单次响应长度限制在 maxLen；
// 余下部分交由浏览器后续 Range 请求继续拉取。
func clampRange(start, end, size, maxLen int64) (int64, int64) {
	if start < 0 {
		start = 0
	}
	if end >= size {
		end = size - 1
	}
	if maxLen > 0 && end-start+1 > maxLen {
		end = start + maxLen - 1
	}
	return start, end
}

// videoMIMEByExt 常见视频扩展名的 MIME 兜底表
//（不依赖系统注册表，保证跨平台与测试结果一致）。
var videoMIMEByExt = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".mov":  "video/quicktime",
	".webm": "video/webm",
	".ogv":  "video/ogg",
	".ogg":  "video/ogg",
	".mkv":  "video/x-matroska",
	".avi":  "video/x-msvideo",
}

// contentTypeFor 计算响应 Content-Type：文档自带 MIME 优先，其次按扩展名回退。
func contentTypeFor(mimeType, name string) string {
	if mimeType = strings.TrimSpace(mimeType); mimeType != "" {
		return mimeType
	}
	ext := strings.ToLower(filepath.Ext(name))
	if t, ok := videoMIMEByExt[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}
