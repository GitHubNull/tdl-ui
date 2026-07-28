package services

import (
	"fmt"

	"tdl-ui/internal/logging"
	"tdl-ui/internal/script"
)

var logScript = logging.L("script")

// ScriptService 用户脚本管理：CRUD、校验与试运行。
type ScriptService struct {
	store *script.Store
}

// NewScriptService 创建脚本服务。
func NewScriptService(store *script.Store) *ScriptService {
	return &ScriptService{store: store}
}

// List 列出全部脚本。
func (s *ScriptService) List() ([]script.Meta, error) { return s.store.List() }

// Read 读取脚本源码。
func (s *ScriptService) Read(name string) (string, error) { return s.store.Read(name) }

// Save 保存脚本源码。
func (s *ScriptService) Save(name, src string) error {
	logScript.Infof("保存脚本: %s（%d 字节）", name, len(src))
	if err := s.store.Write(name, src); err != nil {
		logScript.Errorf("保存脚本失败: %s err=%v", name, err)
		return err
	}
	return nil
}

// Delete 删除脚本。
func (s *ScriptService) Delete(name string) error {
	logScript.Infof("删除脚本: %s", name)
	if err := s.store.Delete(name); err != nil {
		logScript.Errorf("删除脚本失败: %s err=%v", name, err)
		return err
	}
	return nil
}

// ValidateResult 脚本校验结果。
type ValidateResult struct {
	OK    bool     `json:"ok"`
	Error string   `json:"error,omitempty"`
	Funcs []string `json:"funcs"` // 检测到的契约函数
}

// Validate 编译校验脚本并列出检测到的契约函数。
func (s *ScriptService) Validate(src string) ValidateResult {
	c, err := script.Load(src)
	if err != nil {
		logScript.Warnf("脚本校验失败: %v", err)
		return ValidateResult{OK: false, Error: err.Error()}
	}

	funcs := make([]string, 0, 5)
	if c.Filter != nil {
		funcs = append(funcs, "Filter")
	}
	if c.Rename != nil {
		funcs = append(funcs, "Rename")
	}
	if c.OnTaskStart != nil {
		funcs = append(funcs, "OnTaskStart")
	}
	if c.OnFileDone != nil {
		funcs = append(funcs, "OnFileDone")
	}
	if c.OnTaskDone != nil {
		funcs = append(funcs, "OnTaskDone")
	}
	return ValidateResult{OK: true, Funcs: funcs}
}

// TestRunResult 试运行结果。
type TestRunResult struct {
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	SampleFile string `json:"sampleFile"`           // 示例文件名
	FilterKeep *bool  `json:"filterKeep,omitempty"` // Filter 结果
	RenameTo   string `json:"renameTo,omitempty"`   // Rename 结果
}

// TestRun 用内置示例文件信息试运行 Filter/Rename。
func (s *ScriptService) TestRun(src string) TestRunResult {
	logScript.Debugf("脚本试运行（%d 字节）", len(src))
	c, err := script.Load(src)
	if err != nil {
		logScript.Warnf("脚本试运行编译失败: %v", err)
		return TestRunResult{OK: false, Error: err.Error()}
	}

	sample := script.SampleFileInfo()
	res := TestRunResult{OK: true, SampleFile: sample.FileName}

	if c.HasFilter() {
		keep, ferr := c.SafeFilter(sample)
		if ferr != nil {
			return TestRunResult{OK: false, Error: fmt.Sprintf("Filter 执行出错: %v", ferr), SampleFile: sample.FileName}
		}
		res.FilterKeep = &keep
	}
	if c.HasRename() {
		name, rerr := c.SafeRename(sample)
		if rerr != nil {
			return TestRunResult{OK: false, Error: fmt.Sprintf("Rename 执行出错: %v", rerr), SampleFile: sample.FileName}
		}
		res.RenameTo = name
	}
	return res
}

// StarterTemplate 返回新建脚本的起始模板。
func (s *ScriptService) StarterTemplate() string {
	return `package main

import (
	"strings"

	"tdlui/api"
)

// Filter 返回 false 跳过该文件（可选）。
func Filter(f api.FileInfo) bool {
	// 示例：只下载视频文件
	return strings.HasSuffix(f.FileName, ".mp4")
}

// Rename 返回新文件名，空串回退默认模板（可选，可含子目录）。
func Rename(f api.FileInfo) string {
	return ""
}

// OnTaskStart 任务开始时调用（可选）。
func OnTaskStart(t api.TaskInfo) {
	api.Logf("任务 %s 开始，共 %d 个文件", t.ID, t.Total)
}

// OnFileDone 每个文件完成时调用（可选）。
func OnFileDone(f api.FileInfo) {
	api.Log("完成:", f.FileName)
}

// OnTaskDone 任务结束时调用（可选）。
func OnTaskDone(t api.TaskInfo) {
	api.Logf("任务 %s 结束：成功 %d，失败 %d", t.ID, t.Finished, t.Failed)
}
`
}
