// Package script 的内置模板部分：模板源码以 .go.txt 形式嵌入二进制，
// 避免被 Go 工具链当作本包源文件编译。
package script

import (
	"embed"
	"fmt"
)

//go:embed templates/*.go.txt
var templateFS embed.FS

// Template 内置脚本模板（脚本页「模板」弹窗与首次启动播种共用）。
type Template struct {
	// ID 模板标识，同时作为播种时的脚本名
	ID string `json:"id"`
	// Name 展示名
	Name string `json:"name"`
	// Category 分类：命名 / 过滤 / 自动化
	Category string `json:"category"`
	// Description 功能说明
	Description string `json:"description"`
	// Notes 使用注意事项
	Notes string `json:"notes"`
	// Source 完整可编辑的脚本源码
	Source string `json:"source"`
}

// templateManifest 模板清单（元信息内联，源码从 embed 读取）。
var templateManifest = []Template{
	{
		ID:          "rename-by-date",
		Name:        "按日期归档重命名",
		Category:    "文件重命名",
		Description: "按消息日期生成「年/年-月」子目录，文件名前缀带上对话 ID 与消息 ID，便于按时间检索。",
		Notes:       "Rename 返回相对下载目录的路径（用 / 分隔）；返回空串则回退设置页的全局命名模板。",
	},
	{
		ID:          "filter-media",
		Name:        "媒体类型与大小过滤",
		Category:    "过滤规则",
		Description: "按扩展名白名单、文件大小区间与标题关键词筛选待下载文件，只保留命中规则的文件。",
		Notes:       "Filter 返回 false 表示跳过；脚本执行出错时引擎默认保留文件，避免误跳过。",
	},
	{
		ID:          "skip-duplicates",
		Name:        "重复与黑名单跳过",
		Category:    "过滤规则",
		Description: "按扩展名/关键词黑名单排除无用文件，并在同一任务内按对话 ID + 消息 ID + 文件名去重。",
		Notes:       "去重表为脚本内存态，任务重启或应用重启后重置，不做跨会话持久化。",
	},
	{
		ID:          "auto-archive",
		Name:        "任务生命周期汇总",
		Category:    "自动化处理",
		Description: "演示 OnTaskStart / OnFileDone / OnTaskDone 三个钩子，统计各类型文件数量与体积并在任务结束时汇总输出。",
		Notes:       "沙箱只开放纯计算类标准库，钩子内无法读写文件与网络；汇总结果输出到日志面板。",
	},
}

// Templates 返回内置模板列表（含源码）。
// 源码缺失属于构建期错误，此处直接以错误占位提示，不静默丢弃条目。
func Templates() []Template {
	out := make([]Template, 0, len(templateManifest))
	for _, t := range templateManifest {
		b, err := templateFS.ReadFile("templates/" + t.ID + ".go.txt")
		if err != nil {
			t.Source = fmt.Sprintf("// 内置模板 %s 读取失败: %v\n", t.ID, err)
		} else {
			t.Source = string(b)
		}
		out = append(out, t)
	}
	return out
}
