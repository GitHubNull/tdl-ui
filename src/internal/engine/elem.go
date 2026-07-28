// Package engine 下载引擎 GUI 适配层。
// 迭代器/进度逻辑参考 ref/tdl/app/dl 复制改写：去除 viper/survey/终端进度条，
// 接入 Yaegi 脚本过滤命名与 Wails 事件进度上报。
package engine

import (
	"io"
	"os"

	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/core/downloader"
	"github.com/iyear/tdl/core/tmedia"

	"tdl-ui/internal/scriptapi"
)

// iterElem 实现 core/downloader 的 Elem 接口（对应 ref/tdl/app/dl/elem.go）。
type iterElem struct {
	id        int    // 进度跟踪 ID
	resumeKey string // 断点坐标 "dialogID:messageID"，用于断点续传

	from    peers.Peer
	fromMsg *tg.Message
	file    *tmedia.Media
	info    scriptapi.FileInfo // 供脚本钩子使用的元信息

	to *os.File

	takeout bool
}

func (i *iterElem) File() downloader.File { return i }

func (i *iterElem) To() io.WriterAt { return i.to }

func (i *iterElem) AsTakeout() bool { return i.takeout }

func (i *iterElem) Location() tg.InputFileLocationClass { return i.file.InputFileLoc }

func (i *iterElem) Name() string { return i.file.Name }

func (i *iterElem) Size() int64 { return i.file.Size }

func (i *iterElem) DC() int { return i.file.DC }
