// Package scriptapi 定义暴露给用户 Yaegi 脚本的数据类型与辅助函数。
// 脚本通过 import "tdlui/api" 使用本包内容。
package scriptapi

import (
	"fmt"
	"sync"
)

// FileInfo 单个待下载文件的元信息，供过滤/命名脚本使用。
type FileInfo struct {
	// DialogID 会话（频道/群组/私聊）ID
	DialogID int64
	// MessageID 消息 ID
	MessageID int
	// MessageDate 消息发送时间（Unix 秒）
	MessageDate int64
	// FileName 原始文件名
	FileName string
	// FileCaption 消息文本/媒体说明
	FileCaption string
	// FileSize 文件大小（字节）
	FileSize int64
}

// TaskInfo 下载任务的整体信息，供生命周期钩子使用。
type TaskInfo struct {
	// ID 任务 ID
	ID string
	// Dir 下载目录
	Dir string
	// Total 文件总数
	Total int
	// Finished 已完成文件数
	Finished int
	// Failed 失败文件数
	Failed int
	// Status 任务状态：running / done / failed / canceled
	Status string
}

// logSink 收集脚本日志的回调，由宿主应用注入。
var (
	logMu   sync.RWMutex
	logSink func(msg string)
)

// SetLogSink 由宿主应用设置脚本日志的接收器。
func SetLogSink(fn func(msg string)) {
	logMu.Lock()
	defer logMu.Unlock()
	logSink = fn
}

// Log 脚本内打印日志，输出转发到 GUI 的脚本日志面板。
func Log(args ...any) {
	logMu.RLock()
	sink := logSink
	logMu.RUnlock()
	if sink != nil {
		sink(fmt.Sprintln(args...))
	}
}

// Logf 脚本内格式化打印日志。
func Logf(format string, args ...any) {
	logMu.RLock()
	sink := logSink
	logMu.RUnlock()
	if sink != nil {
		sink(fmt.Sprintf(format, args...))
	}
}
