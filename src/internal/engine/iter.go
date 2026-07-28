package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"text/template"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/core/dcpool"
	"github.com/iyear/tdl/core/downloader"
	"github.com/iyear/tdl/core/tmedia"
	"github.com/iyear/tdl/core/util/fsutil"
	"github.com/iyear/tdl/core/util/tutil"
	"github.com/iyear/tdl/pkg/tmessage"
	"github.com/iyear/tdl/pkg/tplfunc"
	"github.com/iyear/tdl/pkg/utils"

	"tdl-ui/internal/script"
	"tdl-ui/internal/scriptapi"
)

const tempExt = ".tmp"

// fileTemplate 命名模板的数据模型，与 tdl 保持一致。
type fileTemplate struct {
	DialogID     int64
	MessageID    int
	MessageDate  int64
	FileName     string
	FileCaption  string
	FileSize     string
	DownloadDate int64
}

// IterOptions 迭代器装配参数。
type IterOptions struct {
	Dir        string
	RewriteExt bool
	SkipSame   bool
	Template   string
	Group      bool
	Contracts  *script.Contracts // 可选：过滤/命名脚本契约
	OnSkip     func(info scriptapi.FileInfo, reason string)
}

// iter 下载元素迭代器（对应 ref/tdl/app/dl/iter.go，去除 include/exclude，
// 由脚本 Filter 接管过滤，Rename 接管命名）。
type iter struct {
	pool    dcpool.Pool
	manager *peers.Manager
	dialogs []*tmessage.Dialog
	tpl     *template.Template
	opts    IterOptions

	mu           *sync.Mutex
	finished     map[int]struct{}
	fingerprint  string
	logicalPos   int
	dialogIndex  int
	messageIndex int

	counter int
	elem    chan downloader.Elem
	err     error
}

func newIter(pool dcpool.Pool, manager *peers.Manager, dialogs []*tmessage.Dialog, opts IterOptions) (*iter, error) {
	tpl, err := template.New("dl").
		Funcs(tplfunc.FuncMap(tplfunc.All...)).
		Parse(opts.Template)
	if err != nil {
		return nil, errors.Wrap(err, "parse template")
	}

	if len(dialogs) == 0 {
		return nil, errors.New("至少需要一条有效的消息链接")
	}

	// 保持 fingerprint 稳定（与 tdl 相同的排序规则）
	sortDialogs(dialogs)

	return &iter{
		pool:    pool,
		manager: manager,
		dialogs: dialogs,
		tpl:     tpl,
		opts:    opts,

		mu:          &sync.Mutex{},
		finished:    make(map[int]struct{}),
		fingerprint: fingerprint(dialogs),
		counter:     -1,
		elem:        make(chan downloader.Elem, 10), // 分组消息缓冲
	}, nil
}

func (i *iter) Next(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		i.err = ctx.Err()
		return false
	default:
	}

	if len(i.elem) > 0 { // 分组消息尚未消费完
		return true
	}

	for {
		ok, skip := i.process(ctx)
		if skip {
			continue
		}
		return ok
	}
}

func (i *iter) process(ctx context.Context) (ret bool, skip bool) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.dialogIndex >= len(i.dialogs) || i.messageIndex >= len(i.dialogs[i.dialogIndex].Messages) || i.err != nil {
		return false, false
	}

	peer, msg := i.dialogs[i.dialogIndex].Peer, i.dialogs[i.dialogIndex].Messages[i.messageIndex]
	startLogicalPos := i.logicalPos

	defer func() {
		if i.messageIndex++; i.dialogIndex < len(i.dialogs) && i.messageIndex >= len(i.dialogs[i.dialogIndex].Messages) {
			i.dialogIndex++
			i.messageIndex = 0
		}
	}()

	from, err := i.manager.FromInputPeer(ctx, peer)
	if err != nil {
		i.err = errors.Wrap(err, "resolve from input peer")
		return false, false
	}
	message, err := tutil.GetSingleMessage(ctx, i.pool.Default(ctx), peer, msg)
	if err != nil {
		if errors.Is(err, tutil.ErrMessageDeleted) { // 已删除消息直接跳过
			i.logicalPos++
			return false, true
		}
		i.err = errors.Wrap(err, "resolve message")
		return false, false
	}

	if _, ok := message.GetGroupedID(); ok && i.opts.Group {
		return i.processGrouped(ctx, message, from, startLogicalPos)
	}

	if _, ok := i.finished[startLogicalPos]; ok { // 断点续传：已完成
		i.logicalPos++
		return false, true
	}

	ret, skip = i.processSingle(ctx, message, from, startLogicalPos)
	i.logicalPos++
	return ret, skip
}

func (i *iter) processSingle(ctx context.Context, message *tg.Message, from peers.Peer, logicalPos int) (bool, bool) {
	item, ok := tmedia.GetMedia(message)
	if !ok { // 无媒体消息
		return false, true
	}

	info := scriptapi.FileInfo{
		DialogID:    from.ID(),
		MessageID:   message.ID,
		MessageDate: int64(message.Date),
		FileName:    item.Name,
		FileCaption: message.Message,
		FileSize:    item.Size,
	}

	// 脚本过滤：返回 false 则跳过该文件
	if keep, err := i.opts.Contracts.SafeFilter(info); err != nil {
		i.notifySkip(info, fmt.Sprintf("过滤脚本出错，默认保留: %v", err))
	} else if !keep {
		i.notifySkip(info, "被过滤脚本跳过")
		return false, true
	}

	// 命名：优先脚本 Rename，出错或空串回退默认模板
	name := ""
	if i.opts.Contracts.HasRename() {
		if n, err := i.opts.Contracts.SafeRename(info); err != nil {
			i.notifySkip(info, fmt.Sprintf("命名脚本出错，回退默认模板: %v", err))
		} else {
			name = n
		}
	}
	if name == "" {
		toName := bytes.Buffer{}
		if err := i.tpl.Execute(&toName, &fileTemplate{
			DialogID:     info.DialogID,
			MessageID:    info.MessageID,
			MessageDate:  info.MessageDate,
			FileName:     info.FileName,
			FileCaption:  info.FileCaption,
			FileSize:     utils.Byte.FormatBinaryBytes(info.FileSize),
			DownloadDate: time.Now().Unix(),
		}); err != nil {
			i.err = errors.Wrap(err, "execute template")
			return false, false
		}
		name = toName.String()
	}

	if i.opts.SkipSame {
		if stat, err := os.Stat(filepath.Join(i.opts.Dir, name)); err == nil {
			if fsutil.GetNameWithoutExt(name) == fsutil.GetNameWithoutExt(stat.Name()) &&
				stat.Size() == item.Size {
				i.notifySkip(info, "本地已存在同名同大小文件")
				return false, true
			}
		}
	}

	path := filepath.Join(i.opts.Dir, name+tempExt)

	// 命名结果可包含子目录，按需创建
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		i.err = errors.Wrap(err, "create dir")
		return false, false
	}

	to, err := os.Create(path)
	if err != nil {
		i.err = errors.Wrap(err, "create file")
		return false, false
	}

	i.counter++
	i.elem <- &iterElem{
		id:         i.counter,
		logicalPos: logicalPos,

		from:    from,
		fromMsg: message,
		file:    item,
		info:    info,

		to: to,
	}

	return true, false
}

func (i *iter) processGrouped(ctx context.Context, message *tg.Message, from peers.Peer, startLogicalPos int) (bool, bool) {
	grouped, err := tutil.GetGroupedMessages(ctx, i.pool.Default(ctx), from.InputPeer(), message)
	if err != nil {
		i.err = errors.Wrapf(err, "resolve grouped message %d/%d", from.ID(), message.ID)
		return false, false
	}

	hasValid := false
	for idx, msg := range grouped {
		logicalPos := startLogicalPos + idx
		if _, ok := i.finished[logicalPos]; ok {
			continue
		}

		ret, skip := i.processSingle(ctx, msg, from, logicalPos)
		if !ret && !skip {
			return false, false
		}
		if ret {
			hasValid = true
		}
	}

	i.logicalPos += len(grouped)
	return hasValid, !hasValid
}

func (i *iter) notifySkip(info scriptapi.FileInfo, reason string) {
	if i.opts.OnSkip != nil {
		i.opts.OnSkip(info, reason)
	}
}

func (i *iter) Value() downloader.Elem { return <-i.elem }

func (i *iter) Err() error { return i.err }

func (i *iter) SetFinished(finished map[int]struct{}) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.finished = finished
}

func (i *iter) Finished() map[int]struct{} {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.finished
}

func (i *iter) Fingerprint() string { return i.fingerprint }

func (i *iter) Finish(id int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.finished[id] = struct{}{}
}

func (i *iter) Total() int {
	i.mu.Lock()
	defer i.mu.Unlock()

	total := 0
	for _, m := range i.dialogs {
		total += len(m.Messages)
	}
	return total
}

func sortDialogs(dialogs []*tmessage.Dialog) {
	sort.Slice(dialogs, func(i, j int) bool {
		return tutil.GetInputPeerID(dialogs[i].Peer) <
			tutil.GetInputPeerID(dialogs[j].Peer)
	})

	for _, m := range dialogs {
		sort.Ints(m.Messages)
	}
}

func fingerprint(dialogs []*tmessage.Dialog) string {
	endian := binary.BigEndian
	buf, b := &bytes.Buffer{}, make([]byte, 8)
	for _, m := range dialogs {
		endian.PutUint64(b, uint64(tutil.GetInputPeerID(m.Peer)))
		buf.Write(b)
		for _, msg := range m.Messages {
			endian.PutUint64(b, uint64(msg))
			buf.Write(b)
		}
	}
	return fmt.Sprintf("%x", sha256.Sum256(buf.Bytes()))
}
