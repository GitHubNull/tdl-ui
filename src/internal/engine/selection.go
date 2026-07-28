package engine

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/pkg/tmessage"
)

// 对话类型（与 ref/tdl/app/chat 一致），services 层复用。
const (
	DialogPrivate = "private"
	DialogGroup   = "group"
	DialogChannel = "channel"
)

// Selection 按「对话 + 消息 ID 集合」直接指定下载内容，
// 无需 t.me 链接（私聊 / 普通群组没有消息链接也可下载）。
type Selection struct {
	DialogID   int64  `json:"dialogId"`
	DialogType string `json:"dialogType"`
	MessageIDs []int  `json:"messageIds"`
}

// ResolveDialogPeer 按对话类型解析输入 peer（依赖对话列表落盘的 access hash）。
func ResolveDialogPeer(ctx context.Context, manager *peers.Manager, id int64, typ string) (tg.InputPeerClass, error) {
	switch typ {
	case DialogPrivate:
		if u, err := manager.ResolveUserID(ctx, id); err == nil {
			return u.InputPeer(), nil
		}
	case DialogChannel:
		if ch, err := manager.ResolveChannelID(ctx, id); err == nil {
			return ch.InputPeer(), nil
		}
	case DialogGroup:
		if ch, err := manager.ResolveChannelID(ctx, id); err == nil {
			return ch.InputPeer(), nil
		}
		if c, err := manager.ResolveChatID(ctx, id); err == nil {
			return c.InputPeer(), nil
		}
	}

	// 兜底：依次尝试各类型
	if ch, err := manager.ResolveChannelID(ctx, id); err == nil {
		return ch.InputPeer(), nil
	}
	if u, err := manager.ResolveUserID(ctx, id); err == nil {
		return u.InputPeer(), nil
	}
	if c, err := manager.ResolveChatID(ctx, id); err == nil {
		return c.InputPeer(), nil
	}
	return nil, errors.New("无法解析该对话，请回到对话列表刷新后重试")
}

// validateSelections 任务创建时的选集预检：格式错误立即反馈。
func validateSelections(sels []Selection) error {
	for _, sel := range sels {
		if sel.DialogID == 0 {
			return errors.New("选集缺少对话 ID")
		}
		if len(sel.MessageIDs) == 0 {
			return errors.Errorf("对话 %d 的选集缺少消息 ID", sel.DialogID)
		}
	}
	return nil
}

// selectionsToDialogs 把选集解析为下载迭代器所需的 tmessage.Dialog。
func selectionsToDialogs(ctx context.Context, manager *peers.Manager, sels []Selection) ([]*tmessage.Dialog, error) {
	out := make([]*tmessage.Dialog, 0, len(sels))
	for _, sel := range sels {
		p, err := ResolveDialogPeer(ctx, manager, sel.DialogID, sel.DialogType)
		if err != nil {
			return nil, errors.Wrapf(err, "解析对话 %d 失败", sel.DialogID)
		}
		out = append(out, &tmessage.Dialog{Peer: p, Messages: sel.MessageIDs})
	}
	return out, nil
}
