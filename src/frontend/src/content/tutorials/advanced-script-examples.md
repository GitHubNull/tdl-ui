# 脚本实战示例

难度：高级 · 前置：脚本语法基础与编写规范

本章提供三个完整可用的脚本案例，均可直接粘贴到「脚本」页使用。每个案例都包含完整代码与逐步操作说明。

## 案例一：只下载大于 10MB 的视频文件

**场景**：频道里混杂着图片、贴纸和视频，只想批量下载有价值的大视频。

**完整代码**：

```go
package main

import (
	"regexp"

	"tdlui/api"
)

// 常见视频扩展名（不区分大小写）
var videoExt = regexp.MustCompile(`(?i)\.(mp4|mkv|avi|mov|webm)$`)

// 最小体积：10MB
const minSize = 10 * 1024 * 1024

func Filter(f api.FileInfo) bool {
	if !videoExt.MatchString(f.FileName) {
		api.Logf("跳过非视频: %s", f.FileName)
		return false
	}
	if f.FileSize < minSize {
		api.Logf("跳过小文件: %s (%d 字节)", f.FileName, f.FileSize)
		return false
	}
	return true
}
```

**操作步骤**：

1. 打开「脚本」页，点击工具栏的 **+**（新建脚本）按钮
2. 脚本名填 `big-video-only`，粘贴上面的代码
3. 点击「校验」—— 状态栏应显示识别到 `Filter` 函数
4. 点击「试运行」—— 观察日志面板中示例文件的过滤结果
5. 点击「保存」
6. 到「对话」页勾选媒体 → 「下载选中」→ 在弹窗的脚本下拉框选择 `big-video-only` → 创建任务
7. 「下载」页只会出现符合条件的视频，被跳过的文件在脚本日志中可见

## 案例二：按消息日期分目录 + 说明文字做文件名

**场景**：长期归档频道内容，希望文件按 `2026-07-30/说明文字.mp4` 的目录结构存放，便于检索。

**完整代码**：

```go
package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"tdlui/api"
)

func Rename(f api.FileInfo) string {
	// 消息时间戳（Unix 秒）→ 日期目录名
	day := time.Unix(f.MessageDate, 0).Format("2006-01-02")

	name := strings.TrimSpace(f.FileCaption)
	if name == "" {
		// 无说明文字时保留原名，仅归入日期目录
		return filepath.Join(day, f.FileName)
	}

	// 说明文字过长时截断，避免超出文件系统限制
	runes := []rune(name)
	if len(runes) > 40 {
		name = string(runes[:40])
	}

	api.Logf("重命名: %s -> %s/%s", f.FileName, day, name)
	return filepath.Join(day, fmt.Sprintf("%s%s", name, filepath.Ext(f.FileName)))
}
```

**操作步骤**：

1. 新建脚本 `archive-by-date`，粘贴代码并「校验」
2. 点击「试运行」—— 日志面板会显示示例文件的重命名结果预览
3. 保存后在创建任务时选择该脚本
4. 下载完成后，打开保存目录即可看到按日期分层的结构：

```
Downloads/tdl-ui/
├── 2026-07-29/
│   ├── 产品发布会回放.mp4
│   └── IMG_2233.jpg
└── 2026-07-30/
    └── 周报数据图表.png
```

> 文件名中的非法字符（如 `/ \ : * ?`）会被自动清理，无需手动处理。

## 案例三：任务生命周期统计通知

**场景**：批量下载几百个文件时，想在脚本日志里实时掌握任务进展，并在结束时输出成功率汇总。

**完整代码**：

```go
package main

import (
	"time"

	"tdlui/api"
)

// 记录任务开始时间，用于计算耗时
var startAt time.Time

func OnTaskStart(t api.TaskInfo) {
	startAt = time.Now()
	api.Logf("[%s] 任务 %s 开始，共 %d 个文件，保存到 %s",
		startAt.Format("15:04:05"), t.ID, t.Total, t.Dir)
}

func OnFileDone(f api.FileInfo) {
	api.Logf("完成 %s（%.1f MB）", f.FileName, float64(f.FileSize)/1024/1024)
}

func OnTaskDone(t api.TaskInfo) {
	elapsed := time.Since(startAt).Round(time.Second)
	rate := 0.0
	if t.Total > 0 {
		rate = float64(t.Finished) / float64(t.Total) * 100
	}
	api.Logf("任务结束（%s）：状态=%s 成功=%d 失败=%d 成功率=%.1f%% 耗时=%s",
		t.ID, t.Status, t.Finished, t.Failed, rate, elapsed)
}
```

**操作步骤**：

1. 新建脚本 `task-stats`，粘贴代码，「校验」后保存
2. 创建任务时选择该脚本
3. 切到「脚本」页，日志面板会实时滚动输出：

```
[14:32:01] 任务 a1b2c3 开始，共 58 个文件，保存到 D:\Downloads\tdl-ui
完成 video_001.mp4（35.2 MB）
完成 video_002.mp4（18.7 MB）
...
任务结束（a1b2c3）：状态=done 成功=57 失败=1 成功率=98.3% 耗时=6m12s
```

> 任务被取消或失败时同样会触发 `OnTaskDone`，可通过 `t.Status` 区分（`done` / `failed` / `canceled`）。

## 组合使用

三类函数可以写进**同一个脚本**：Filter 负责挑文件、Rename 负责归档、钩子负责统计，创建任务时选择一个脚本即可全部生效。
