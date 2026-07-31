// Package script 的文件存储部分：脚本以 .go 文件形式存放在数据目录 scripts/ 下。
package script

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"tdl-ui/internal/scriptapi"
)

// Meta 脚本元信息（列表展示用）。
type Meta struct {
	Name        string `json:"name"`        // 不含扩展名的脚本名
	Size        int64  `json:"size"`        // 文件大小（字节）
	UpdatedAt   string `json:"updatedAt"`   // 最后修改时间（YYYY-MM-DD HH:MM:SS）
	Description string `json:"description"` // 从源码头部注释提取的说明
	Enabled     bool   `json:"enabled"`     // 是否启用（Store 不维护，始终 false）
}

// Store 管理脚本目录下的 .go 文件。
type Store struct {
	dir string
}

// NewStore 创建脚本存储，dir 必须已存在。
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// 脚本名仅允许字母、数字、下划线、中划线与中文，防止路径穿越。
var nameRe = regexp.MustCompile(`^[\w\p{Han}-]+$`)

// windowsReservedNames Windows 保留设备名（不区分大小写），
// 作为文件名时创建/删除行为异常，需拒绝（SCR-05）。
var windowsReservedNames = map[string]struct{}{
	"con": {}, "prn": {}, "aux": {}, "nul": {},
	"com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {},
	"com6": {}, "com7": {}, "com8": {}, "com9": {},
	"lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {},
	"lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {},
}

func (s *Store) path(name string) (string, error) {
	if !nameRe.MatchString(name) {
		return "", fmt.Errorf("非法脚本名: %q（仅允许字母、数字、下划线、中划线与中文）", name)
	}
	if _, ok := windowsReservedNames[strings.ToLower(name)]; ok {
		return "", fmt.Errorf("非法脚本名: %q（Windows 保留设备名）", name)
	}
	return filepath.Join(s.dir, name+".go"), nil
}

// List 列出所有脚本，按名称排序，说明取自源码头部注释。
func (s *Store) List() ([]Meta, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	metas := make([]Meta, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".go")
		desc := extractDescription(filepath.Join(s.dir, e.Name()))
		metas = append(metas, Meta{
			Name:        name,
			Size:        info.Size(),
			UpdatedAt:   info.ModTime().Format("2006-01-02 15:04:05"),
			Description: desc,
			Enabled:     false, // Store 层不维护启用状态
		})
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Name < metas[j].Name })
	return metas, nil
}

// extractDescription 从脚本文件头部注释提取说明。
// 读取 package 之前连续 // 行首段（截断 200 字符）作为 Description。
func extractDescription(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	// 只读前 4KB，避免大文件
	if len(b) > 4096 {
		b = b[:4096]
	}
	lines := strings.Split(string(b), "\n")
	var descLines []string
	inBlock := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "//") {
			inBlock = true
			text := strings.TrimSpace(strings.TrimPrefix(line, "//"))
			if text != "" {
				descLines = append(descLines, text)
			}
			continue
		}
		if strings.HasPrefix(line, "package ") {
			break
		}
		if inBlock {
			break
		}
	}
	desc := strings.Join(descLines, " ")
	if len(desc) > 200 {
		desc = desc[:200]
	}
	return desc
}

// Read 读取脚本源码。
func (s *Store) Read(name string) (string, error) {
	p, err := s.path(name)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Write 保存脚本源码。
func (s *Store) Write(name, src string) error {
	p, err := s.path(name)
	if err != nil {
		return err
	}
	return os.WriteFile(p, []byte(src), 0o644)
}

// Delete 删除脚本。
func (s *Store) Delete(name string) error {
	p, err := s.path(name)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// LoadByName 读取并解释指定脚本。
func (s *Store) LoadByName(name string) (*Contracts, error) {
	src, err := s.Read(name)
	if err != nil {
		return nil, err
	}
	return Load(src)
}

// SampleFileInfo 试运行用的示例文件信息。
func SampleFileInfo() scriptapi.FileInfo {
	return scriptapi.FileInfo{
		DialogID:    1234567890,
		MessageID:   42,
		MessageDate: time.Now().Unix(),
		FileName:    "example_video.mp4",
		FileCaption: "示例消息文本",
		FileSize:    1024 * 1024 * 128,
	}
}

// SeedTemplates 首次启动时将内置模板写入脚本目录，已有脚本不会被覆盖。
// 返回新写入的模板数量。
func (s *Store) SeedTemplates() (int, error) {
	n := 0
	for _, tpl := range Templates() {
		if _, err := os.Stat(filepath.Join(s.dir, tpl.ID+".go")); err == nil {
			continue // 已有脚本不覆盖
		}
		if err := s.Write(tpl.ID, tpl.Source); err != nil {
			return n, fmt.Errorf("写入模板 %s 失败: %w", tpl.ID, err)
		}
		n++
	}
	return n, nil
}
