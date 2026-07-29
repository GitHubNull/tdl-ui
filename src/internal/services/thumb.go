package services

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/core/storage"
	"github.com/iyear/tdl/core/util/tutil"

	"tdl-ui/internal/engine"
)

// strippedJPEGHeaderHex Telegram stripped 缩略图的公共 JPEG 头
//（各官方客户端一致），高度/宽度字节分别位于偏移 164 / 166。
const strippedJPEGHeaderHex = "ffd8ffe000104a46494600010100000100010000ffdb004300281c1e231e19282321232d2b28303c64413c37373c7b585d4964918099968f808c8aa0b4e6c3a0aadaad8a8cc8ffcbdaeef5ffffff9bc1fffffffaffe6fdfff8ffdb0043012b2d2d3c353c76414176f8a58ca5f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8f8ffc00011080000000003012200021101031101ffc4001f0000010501010101010100000000000000000102030405060708090a0bffc400b5100002010303020403050504040000017d01020300041105122131410613516107227114328191a1082342b1c11552d1f02433627282090a161718191a25262728292a3435363738393a434445464748494a535455565758595a636465666768696a737475767778797a838485868788898a92939495969798999aa2a3a4a5a6a7a8a9aab2b3b4b5b6b7b8b9bac2c3c4c5c6c7c8c9cad2d3d4d5d6d7d8d9dae1e2e3e4e5e6e7e8e9eaf1f2f3f4f5f6f7f8f9faffc4001f0100030101010101010101010000000000000102030405060708090a0bffc400b51100020102040403040705040400010277000102031104052131061241510761711322328108144291a1b1c109233352f0156272d10a162434e125f11718191a262728292a35363738393a434445464748494a535455565758595a636465666768696a737475767778797a82838485868788898a92939495969798999aa2a3a4a5a6a7a8a9aab2b3b4b5b6b7b8b9bac2c3c4c5c6c7c8c9cad2d3d4d5d6d7d8d9dae2e3e4e5e6e7e8e9eaf2f3f4f5f6f7f8f9faffda000c03010002110311003f00"

var (
	strippedHeaderOnce sync.Once
	strippedHeader     []byte
)

// expandStrippedThumb 把 Telegram stripped 缩略图（0x01 + 高 + 宽 + JPEG 体）
// 展开为完整 JPEG；格式不符时返回 nil。
func expandStrippedThumb(b []byte) []byte {
	if len(b) < 4 || b[0] != 0x01 {
		return nil
	}
	strippedHeaderOnce.Do(func() {
		strippedHeader, _ = hex.DecodeString(strippedJPEGHeaderHex)
	})
	if len(strippedHeader) == 0 {
		return nil
	}

	out := make([]byte, 0, len(strippedHeader)+len(b)-3+2)
	out = append(out, strippedHeader...)
	out[164] = b[1] // 高度低字节
	out[166] = b[2] // 宽度低字节
	out = append(out, b[3:]...)
	return append(out, 0xFF, 0xD9)
}

// strippedThumbURI 从尺寸列表提取 stripped 占位图并编码为 data URI；无则返回空串。
func strippedThumbURI(sizes []tg.PhotoSizeClass) string {
	for _, sc := range sizes {
		if s, ok := sc.(*tg.PhotoStrippedSize); ok {
			if jpg := expandStrippedThumb(s.Bytes); jpg != nil {
				return jpegDataURI(jpg)
			}
		}
	}
	return ""
}

func jpegDataURI(b []byte) string {
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b)
}

// pickThumbSize 选择用于清晰预览的缩略图规格：
// 优先 "m"（约 320px），其次最小的真实尺寸；PhotoCachedSize 自带字节可直接使用。
func pickThumbSize(sizes []tg.PhotoSizeClass) (thumbType string, inline []byte) {
	var smallest string
	smallestBytes := 0
	for _, sc := range sizes {
		switch v := sc.(type) {
		case *tg.PhotoSize:
			if v.Type == "m" {
				return v.Type, nil
			}
			if smallest == "" || v.Size < smallestBytes {
				smallest, smallestBytes = v.Type, v.Size
			}
		case *tg.PhotoSizeProgressive:
			if v.Type == "m" {
				return v.Type, nil
			}
			if smallest == "" {
				smallest, smallestBytes = v.Type, v.Sizes[len(v.Sizes)-1]
			}
		case *tg.PhotoCachedSize:
			inline = v.Bytes
		}
	}
	return smallest, inline
}

// pickPreviewSize 选择预览大图规格：取像素面积最大的真实尺寸；
// 无真实尺寸时回退 PhotoCachedSize 内联字节。
func pickPreviewSize(sizes []tg.PhotoSizeClass) (thumbType string, inline []byte) {
	bestArea := 0
	for _, sc := range sizes {
		switch v := sc.(type) {
		case *tg.PhotoSize:
			if a := v.W * v.H; a > bestArea {
				thumbType, bestArea = v.Type, a
			}
		case *tg.PhotoSizeProgressive:
			if a := v.W * v.H; a > bestArea {
				thumbType, bestArea = v.Type, a
			}
		case *tg.PhotoCachedSize:
			inline = v.Bytes
		}
	}
	return thumbType, inline
}

// thumbJPEG 拉取消息媒体的缩略图（preview=true 取最大尺寸预览图），
// 经磁盘缓存返回 JPEG 字节；无可用缩略图（如普通文件）返回空切片。
func (s *ChatService) thumbJPEG(dialogID int64, dialogType string, messageID int, preview bool) ([]byte, error) {
	if dialogID == 0 || messageID == 0 {
		return nil, errors.New("缺少对话或消息 ID")
	}

	kind := cacheKindThumb
	if preview {
		kind = cacheKindPreview
	}
	return s.thumbs.Get(kind, dialogID, messageID, func() ([]byte, error) {
		// 独立缩略图队列排队短，worker 会跳过已超时的任务，30s 足够覆盖单次拉取
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var out []byte
		err := s.invokeThumb(ctx, func(ctx context.Context, api *tg.Client) error {
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

			out, err = fetchThumb(ctx, api, msg, preview)
			return err
		})
		if err != nil {
			return nil, err
		}
		return out, nil
	})
}

// fetchThumb 从消息媒体定位缩略图并下载；无可用缩略图返回空切片。
func fetchThumb(ctx context.Context, api *tg.Client, msg *tg.Message, preview bool) ([]byte, error) {
	media, ok := msg.GetMedia()
	if !ok {
		return nil, nil
	}

	pick := pickThumbSize
	if preview {
		pick = pickPreviewSize
	}

	var (
		loc    tg.InputFileLocationClass
		inline []byte
	)
	switch md := media.(type) {
	case *tg.MessageMediaPhoto:
		p, ok := md.Photo.(*tg.Photo)
		if !ok {
			return nil, nil
		}
		typ, cached := pick(p.Sizes)
		if typ == "" {
			inline = cached
			break
		}
		loc = &tg.InputPhotoFileLocation{
			ID:            p.ID,
			AccessHash:    p.AccessHash,
			FileReference: p.FileReference,
			ThumbSize:     typ,
		}
	case *tg.MessageMediaDocument:
		doc, ok := md.Document.(*tg.Document)
		if !ok {
			return nil, nil
		}
		typ, cached := pick(doc.Thumbs)
		if typ == "" {
			inline = cached
			break
		}
		loc = &tg.InputDocumentFileLocation{
			ID:            doc.ID,
			AccessHash:    doc.AccessHash,
			FileReference: doc.FileReference,
			ThumbSize:     typ,
		}
	default:
		return nil, nil
	}

	if loc == nil {
		return inline, nil
	}
	return downloadSmallFile(ctx, api, loc)
}

// downloadSmallFile 分块拉取小文件（缩略图通常一次即完成）。
func downloadSmallFile(ctx context.Context, api *tg.Client, loc tg.InputFileLocationClass) ([]byte, error) {
	const chunk = 64 * 1024 // 4096 对齐

	out := make([]byte, 0, chunk)
	for offset := int64(0); ; offset += chunk {
		res, err := api.UploadGetFile(ctx, &tg.UploadGetFileRequest{
			Location: loc,
			Offset:   offset,
			Limit:    chunk,
		})
		if err != nil {
			return nil, errors.Wrap(err, "下载缩略图失败")
		}
		f, ok := res.(*tg.UploadFile)
		if !ok {
			return nil, errors.Errorf("不支持的响应类型: %T", res)
		}
		out = append(out, f.Bytes...)
		if len(f.Bytes) < chunk {
			return out, nil
		}
	}
}
