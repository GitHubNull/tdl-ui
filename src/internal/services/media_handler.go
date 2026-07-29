package services

import (
	"net/http"
	"strconv"

	"tdl-ui/internal/logging"
)

var logMedia = logging.L("media")

// NewMediaHandler 缩略图/预览图 HTTP 服务（挂载于 Wails assetserver 的兜底 Handler）：
//
//	GET /media/thumb?d=<dialogId>&m=<msgId>&t=<dialogType>
//	GET /media/preview?d=<dialogId>&m=<msgId>&t=<dialogType>
//
// 命中磁盘缓存直接返回，未命中经 Telegram 拉取；浏览器侧由 Cache-Control 长缓存接管。
func NewMediaHandler(s *ChatService) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/media/thumb", func(w http.ResponseWriter, r *http.Request) {
		serveMediaImage(s, w, r, false)
	})
	mux.HandleFunc("/media/preview", func(w http.ResponseWriter, r *http.Request) {
		serveMediaImage(s, w, r, true)
	})
	return mux
}

func serveMediaImage(s *ChatService, w http.ResponseWriter, r *http.Request, preview bool) {
	q := r.URL.Query()
	dialogID, _ := strconv.ParseInt(q.Get("d"), 10, 64)
	msgID, _ := strconv.Atoi(q.Get("m"))
	if dialogID == 0 || msgID == 0 {
		http.Error(w, "缺少 d/m 参数", http.StatusBadRequest)
		return
	}

	b, err := s.thumbJPEG(dialogID, q.Get("t"), msgID, preview)
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
