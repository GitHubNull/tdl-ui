package script

import (
	"strings"
	"testing"
)

// TestTemplatesLoadable 每个内置模板都应带完整元信息且能被 Yaegi 加载出契约函数。
func TestTemplatesLoadable(t *testing.T) {
	tpls := Templates()
	if len(tpls) == 0 {
		t.Fatal("内置模板列表不应为空")
	}

	for _, tpl := range tpls {
		t.Run(tpl.ID, func(t *testing.T) {
			if tpl.Name == "" || tpl.Category == "" || tpl.Description == "" || tpl.Notes == "" {
				t.Fatalf("模板元信息不完整: %+v", tpl)
			}
			if strings.TrimSpace(tpl.Source) == "" {
				t.Fatal("模板源码不应为空")
			}

			// 头部注释块须齐备三要素，保存为脚本后即成为说明来源
			head := tpl.Source
			if idx := strings.Index(head, "package main"); idx > 0 {
				head = head[:idx]
			}
			for _, marker := range []string{"【功能】", "【参数】", "【注意】"} {
				if !strings.Contains(head, marker) {
					t.Errorf("模板头部注释缺少 %s", marker)
				}
			}

			c, err := Load(tpl.Source)
			if err != nil {
				t.Fatalf("模板加载失败: %v", err)
			}
			if c.Filter == nil && c.Rename == nil && c.OnTaskStart == nil && c.OnFileDone == nil && c.OnTaskDone == nil {
				t.Fatal("模板未定义任何契约函数")
			}
		})
	}
}

// TestSeedTemplatesSkipsExisting 播种写入全部模板，重复播种不覆盖已有脚本。
func TestSeedTemplatesSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	n, err := s.SeedTemplates()
	if err != nil {
		t.Fatalf("SeedTemplates 失败: %v", err)
	}
	if n != len(Templates()) {
		t.Fatalf("首次播种应写入 %d 个模板，实际 %d", len(Templates()), n)
	}

	// 用户改过的脚本不应被二次播种覆盖
	first := Templates()[0].ID
	if err := s.Write(first, "// 用户自定义\npackage main\n"); err != nil {
		t.Fatalf("Write 失败: %v", err)
	}
	n2, err := s.SeedTemplates()
	if err != nil {
		t.Fatalf("二次 SeedTemplates 失败: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("二次播种应跳过全部已存在脚本，实际新建 %d", n2)
	}
	src, err := s.Read(first)
	if err != nil {
		t.Fatalf("Read 失败: %v", err)
	}
	if !strings.Contains(src, "用户自定义") {
		t.Fatal("已存在脚本不应被播种覆盖")
	}
}

// TestListDescriptionFromHeader List 从源码头部注释提取脚本说明。
func TestListDescriptionFromHeader(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.SeedTemplates(); err != nil {
		t.Fatalf("SeedTemplates 失败: %v", err)
	}

	metas, err := s.List()
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(metas) != len(Templates()) {
		t.Fatalf("应列出 %d 个脚本，实际 %d", len(Templates()), len(metas))
	}
	for _, m := range metas {
		if !strings.Contains(m.Description, "【功能】") {
			t.Errorf("脚本 %s 说明应取自头部注释，实际 %q", m.Name, m.Description)
		}
		if m.Enabled {
			t.Errorf("Store 层不填充启用状态，脚本 %s 应为 false", m.Name)
		}
	}
}
