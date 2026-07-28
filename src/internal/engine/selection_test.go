package engine

import (
	"strings"
	"testing"
)

func TestCreateRequiresSource(t *testing.T) {
	m := NewManager(Deps{})
	if _, err := m.Create(TaskOptions{}); err == nil {
		t.Fatal("URLs 与 Selections 均为空时应报错")
	}
}

func TestValidateSelections(t *testing.T) {
	cases := []struct {
		name    string
		sels    []Selection
		wantErr string // 空串表示不应出错
	}{
		{"空选集合法", nil, ""},
		{"正常选集", []Selection{{DialogID: 1, DialogType: DialogChannel, MessageIDs: []int{1, 2}}}, ""},
		{"缺对话ID", []Selection{{MessageIDs: []int{1}}}, "缺少对话 ID"},
		{"缺消息ID", []Selection{{DialogID: 5, DialogType: DialogPrivate}}, "缺少消息 ID"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateSelections(c.sels)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("预期成功，实际 %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("预期包含 %q 的错误，实际 %v", c.wantErr, err)
			}
		})
	}
}
