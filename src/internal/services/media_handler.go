package services

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-faster/errors"

	"tdl-ui/internal/logging"
)

var logMedia = logging.L("media")

// LocalPathFunc 解析某条消息已下载完成的本地文件路径；未下载时第二返回值为 false。
type LocalPathFunc func(dialogID int64, messageID int) (string, bool)

// NewMediaHandler 媒体字节服务（挂载于 Wails assetserver 的兜底 Handler）：
//
//	GET /media/thumb?d=<dialogId>&m=<msgId>&t=<dialogType>    缩略图 JPEG
//	GET /media/preview?d=<dialogId>&m=<msgId>&t=<dialogType>  预览大图 JPEG
//	GET /media/local?d=<dialogId>&m=<msgId>                   已下载文件（原生 Range/seek）
//	GET /media/video?d=<dialogId>&m=<msgId>&t=<dialogType>    在线视频分段流（HTTP 206）
//
// 图片命中磁盘缓存直接返回，未命中经 Telegram 拉取；浏览器侧由 Cache-Control 长缓存接管。
// 视频优先由前端选择 /media/local（零 API 消耗），未下载时回落 /media/video 分段拉流。
func NewMediaHandler(s *ChatService, localPath LocalPathFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/media/thumb", func(w http.ResponseWriter, r *http.Request) {
		serveMediaImage(s, w, r, false)
	})
	mux.HandleFunc("/media/preview", func(w http.ResponseWriter, r *http.Request) {
		serveMediaImage(s, w, r, true)
	})
	mux.HandleFunc("/media/local", func(w http.ResponseWriter, r *http.Request) {
		serveLocalMedia(localPath, w, r)
	})
	mux.HandleFunc("/media/video", func(w http.ResponseWriter, r *http.Request) {
		serveVideoStream(s, w, r)
	})
	return mux
}

// mediaParams 解析公共查询参数；缺失时已写出 400。
func mediaParams(w http.ResponseWriter, r *http.Request) (dialogID int64, msgID int, ok bool) {
	q := r.URL.Query()
	dialogID, _ = strconv.ParseInt(q.Get("d"), 10, 64)
	msgID, _ = strconv.Atoi(q.Get("m"))
	if dialogID == 0 || msgID == 0 {
		http.Error(w, "缺少 d/m 参数", http.StatusBadRequest)
		return 0, 0, false
	}
	return dialogID, msgID, true
}

func serveMediaImage(s *ChatService, w http.ResponseWriter, r *http.Request, preview bool) {
	dialogID, msgID, ok := mediaParams(w, r)
	if !ok {
		return
	}

	b, err := s.thumbJPEG(dialogID, r.URL.Query().Get("t"), msgID, preview)
	if err != nil {
		// SVC-15：详情走日志，不把内部错误链（kv 路径、gotd 错误）发给前端
		logMedia.Warnf("缩略图拉取失败: dialog=%d msg=%d preview=%v err=%v", dialogID, msgID, preview, err)
		http.Error(w, "拉取媒体图失败，详情见应用日志", http.StatusInternalServerError)
		return
	}
	if len(b) == 0 {
		http.NotFound(w, r) // 无可用缩略图（如普通文件）
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(b)
}

// serveLocalMedia 直接回传已下载到磁盘的文件；
// http.ServeContent 天然支持 Range/If-Modified-Since，播放器可自由 seek。
func serveLocalMedia(localPath LocalPathFunc, w http.ResponseWriter, r *http.Request) {
	dialogID, msgID, ok := mediaParams(w, r)
	if !ok {
		return
	}
	if localPath == nil {
		http.NotFound(w, r)
		return
	}

	p, ok := localPath(dialogID, msgID)
	if !ok || p == "" {
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(filepath.Clean(p))
	if err != nil {
		// 记录已被删除/无权限的情况，前端据 404 回落在线流
		logMedia.Debugf("本地文件不可读: dialog=%d msg=%d path=%s err=%v", dialogID, msgID, p, err)
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}

	// 预置 Content-Type，避免依赖系统 MIME 注册表（Windows 上可能缺失视频映射）
	w.Header().Set("Content-Type", contentTypeFor("", p))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, filepath.Base(p), st.ModTime(), f)
}

// serveVideoStream 在线视频分段流：按 Range 请求从 Telegram 拉取 1MB 对齐分段回写，
// 单次响应长度上限 videoRangeMax，其余由浏览器续发 Range（流式 + 懒加载）。
func serveVideoStream(s *ChatService, w http.ResponseWriter, r *http.Request) {
	dialogID, msgID, ok := mediaParams(w, r)
	if !ok {
		return
	}

	meta, err := s.VideoMeta(dialogID, r.URL.Query().Get("t"), msgID)
	switch {
	case errors.Is(err, errNotVideo):
		http.NotFound(w, r)
		return
	case err != nil:
		// SVC-15：内部错误链只进日志
		logMedia.Warnf("视频元信息获取失败: dialog=%d msg=%d err=%v", dialogID, msgID, err)
		http.Error(w, "获取视频信息失败，详情见应用日志", http.StatusInternalServerError)
		return
	}
	if meta.Size <= 0 {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", contentTypeFor(meta.MIME, meta.Name))
	w.Header().Set("Accept-Ranges", "bytes")
	// 分段响应不进 webview 磁盘缓存，命中复用交由后端内存分段 LRU
	w.Header().Set("Cache-Control", "no-store")

	start, end, status := parseRangeHeader(r.Header.Get("Range"), meta.Size)
	switch status {
	case rangeUnsatisfiable:
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", meta.Size))
		http.Error(w, "请求范围越界", http.StatusRequestedRangeNotSatisfiable)
		return
	case rangeNone:
		start, end = 0, meta.Size-1
	case rangePartial:
		start, end = clampRange(start, end, meta.Size, videoRangeMax)
	}

	// 边下边播：从本次请求起始分段触发后台顺序预取到文件末尾
	//（同一视频重复触发不重启，HEAD 探测不触发）
	if r.Method != http.MethodHead {
		s.startVideoPrefetch(dialogID, msgID, meta, start/videoPartSize)
	}

	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	if status == rangePartial {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, meta.Size))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if r.Method == http.MethodHead {
		return
	}

	if err := streamVideoRange(r.Context(), s, w, dialogID, msgID, meta, start, end); err != nil {
		// 播放器 seek / 关闭预览会主动断开，属正常路径，只记 debug
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			logMedia.Debugf("视频推流中断: dialog=%d msg=%d range=%d-%d err=%v", dialogID, msgID, start, end, err)
			return
		}
		logMedia.Warnf("视频推流失败: dialog=%d msg=%d range=%d-%d err=%v", dialogID, msgID, start, end, err)
	}
}

// streamVideoRange 逐段拉取 [start,end] 区间字节并即时 flush；
// 首段裁掉头部偏移、末段裁掉尾部多余字节，客户端断开即退出以释放后端拉取。
func streamVideoRange(
	ctx context.Context,
	s *ChatService,
	w http.ResponseWriter,
	dialogID int64,
	msgID int,
	meta videoMeta,
	start, end int64,
) error {
	flusher, _ := w.(http.Flusher)

	for offset := start; offset <= end; {
		if err := ctx.Err(); err != nil {
			return err
		}

		partIdx := offset / videoPartSize
		part, err := s.videoPart(ctx, dialogID, msgID, meta, partIdx)
		if err != nil {
			return err
		}

		lo := offset - partIdx*videoPartSize
		if lo >= int64(len(part)) {
			return nil // 服务端返回的分段短于预期：文件已结束
		}
		hi := int64(len(part))
		if remain := end - offset + 1; hi-lo > remain {
			hi = lo + remain
		}

		if _, err := w.Write(part[lo:hi]); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		offset += hi - lo
	}
	return nil
}
