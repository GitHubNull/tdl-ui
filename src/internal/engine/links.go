package engine

import (
	"net/url"
	"strings"

	"github.com/go-faster/errors"
)

// NormalizeURLs 清洗并预检消息链接：去空白与空行、去重，
// 并校验链接语法（t.me 域名 + 消息路径），格式错误在任务创建时即反馈，
// 无需等到连接 Telegram 后才由 tmessage 报错。
func NormalizeURLs(raw []string) ([]string, error) {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		u := strings.TrimSpace(line)
		if u == "" {
			continue
		}
		if err := checkMessageLink(u); err != nil {
			return nil, errors.Wrapf(err, "链接 %q 无效", u)
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	if len(out) == 0 {
		return nil, errors.New("至少需要一条消息链接")
	}
	return out, nil
}

// checkMessageLink 校验单条链接的语法形态（与上游 tutil.ParseMessageLink 支持的形式对齐）：
//   - https://t.me/<name>/<msgid>          公开频道/群组
//   - https://t.me/<name>/<topicid>/<msgid> 话题群
//   - https://t.me/c/<chatid>/<msgid>      私有频道
func checkMessageLink(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("缺少 https:// 前缀")
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host != "t.me" && host != "telegram.me" {
		return errors.New("仅支持 t.me / telegram.me 消息链接")
	}

	paths := strings.Split(strings.Trim(u.Path, "/"), "/")
	// ?comment= / ?thread= 形式最后一段消息 ID 在 query 中
	if u.Query().Get("comment") != "" || u.Query().Get("thread") != "" {
		if len(paths) < 1 || paths[0] == "" {
			return errors.New("缺少频道名")
		}
		return nil
	}
	if len(paths) < 2 {
		return errors.New("缺少消息 ID 路径")
	}
	if paths[0] == "c" && len(paths) < 3 {
		return errors.New("私有频道链接需要 t.me/c/<chatid>/<msgid> 形式")
	}
	// 末段必须是数字消息 ID
	last := paths[len(paths)-1]
	for _, r := range last {
		if r < '0' || r > '9' {
			return errors.New("消息 ID 必须是数字")
		}
	}
	return nil
}
