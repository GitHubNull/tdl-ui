package script

import (
	"strings"
	"testing"

	"tdl-ui/internal/scriptapi"
)

const fullScript = `package main

import (
	"strings"

	"tdlui/api"
)

func Filter(f api.FileInfo) bool {
	return strings.HasSuffix(f.FileName, ".mp4")
}

func Rename(f api.FileInfo) string {
	return "video/" + f.FileName
}

func OnTaskStart(t api.TaskInfo) {
	api.Logf("start %s total=%d", t.ID, t.Total)
}

func OnFileDone(f api.FileInfo) {
	api.Log("done", f.FileName)
}

func OnTaskDone(t api.TaskInfo) {
	api.Logf("end %s ok=%d fail=%d", t.ID, t.Finished, t.Failed)
}
`

func TestLoadFullContracts(t *testing.T) {
	c, err := Load(fullScript)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !c.HasFilter() || !c.HasRename() {
		t.Fatal("Filter/Rename 未检测到")
	}
	if c.OnTaskStart == nil || c.OnFileDone == nil || c.OnTaskDone == nil {
		t.Fatal("生命周期钩子未检测到")
	}
}

func TestFilterAndRename(t *testing.T) {
	c, err := Load(fullScript)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	mp4 := scriptapi.FileInfo{FileName: "movie.mp4"}
	keep, err := c.SafeFilter(mp4)
	if err != nil || !keep {
		t.Fatalf("SafeFilter(mp4) = %v, %v; want true, nil", keep, err)
	}

	jpg := scriptapi.FileInfo{FileName: "photo.jpg"}
	keep, err = c.SafeFilter(jpg)
	if err != nil || keep {
		t.Fatalf("SafeFilter(jpg) = %v, %v; want false, nil", keep, err)
	}

	name, err := c.SafeRename(mp4)
	if err != nil || name != "video/movie.mp4" {
		t.Fatalf("SafeRename = %q, %v", name, err)
	}
}

func TestLifecycleHooks(t *testing.T) {
	var logs []string
	scriptapi.SetLogSink(func(msg string) { logs = append(logs, msg) })
	defer scriptapi.SetLogSink(nil)

	c, err := Load(fullScript)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	task := scriptapi.TaskInfo{ID: "t1", Total: 3, Finished: 2, Failed: 1}
	if err = c.SafeOnTaskStart(task); err != nil {
		t.Fatalf("SafeOnTaskStart: %v", err)
	}
	if err = c.SafeOnFileDone(scriptapi.FileInfo{FileName: "a.mp4"}); err != nil {
		t.Fatalf("SafeOnFileDone: %v", err)
	}
	if err = c.SafeOnTaskDone(task); err != nil {
		t.Fatalf("SafeOnTaskDone: %v", err)
	}

	if len(logs) != 3 {
		t.Fatalf("日志条数 = %d, want 3: %v", len(logs), logs)
	}
	if !strings.Contains(logs[0], "start t1 total=3") {
		t.Errorf("OnTaskStart 日志异常: %q", logs[0])
	}
	if !strings.Contains(logs[2], "ok=2 fail=1") {
		t.Errorf("OnTaskDone 日志异常: %q", logs[2])
	}
}

func TestPanicRecovery(t *testing.T) {
	src := `package main

import "tdlui/api"

func Filter(f api.FileInfo) bool {
	panic("boom")
}
`
	c, err := Load(src)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if _, err = c.SafeFilter(scriptapi.FileInfo{FileName: "x.mp4"}); err == nil {
		t.Fatal("脚本 panic 应转化为 error")
	}
}

func TestSyntaxError(t *testing.T) {
	if _, err := Load("package main\nfunc broken("); err == nil {
		t.Fatal("语法错误应返回 error")
	}
}

func TestEmptyScriptRejected(t *testing.T) {
	// 未定义任何契约函数的脚本应报错，提示用户修正
	if _, err := Load("package main\n"); err == nil {
		t.Fatal("空脚本应返回 error")
	}
}

func TestSandboxBlocksExec(t *testing.T) {
	src := `package main

import "os/exec"

func Filter(f interface{}) bool {
	_ = exec.Command
	return true
}
`
	if _, err := Load(src); err == nil {
		t.Fatal("沙箱应禁止导入 os/exec")
	}
}

func TestSandboxWhitelistBlocksDangerousPackages(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"os", `package main

import "os"

func Filter(f interface{}) bool {
	_ = os.StartProcess
	return true
}
`},
		{"net", `package main

import "net"

func Filter(f interface{}) bool {
	_ = net.Dial
	return true
}
`},
		{"net/http", `package main

import "net/http"

func Filter(f interface{}) bool {
	_ = http.Get
	return true
}
`},
		{"syscall", `package main

import "syscall"

func Filter(f interface{}) bool {
	_ = syscall.Exit
	return true
}
`},
		{"reflect", `package main

import "reflect"

func Filter(f interface{}) bool {
	_ = reflect.ValueOf
	return true
}
`},
		{"io/ioutil", `package main

import "io/ioutil"

func Filter(f interface{}) bool {
	_ = ioutil.WriteFile
	return true
}
`},
	}

	for _, tc := range cases {
		if _, err := Load(tc.src); err == nil {
			t.Errorf("沙箱应禁止导入 %s", tc.name)
		}
	}
}

func TestSandboxAllowsPureComputation(t *testing.T) {
	src := `package main

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"tdlui/api"
)

var re = regexp.MustCompile(` + "`" + `\d+` + "`" + `)

func Rename(f api.FileInfo) string {
	ext := filepath.Ext(f.FileName)
	return strings.ToLower(re.ReplaceAllString(f.FileName, strconv.Itoa(1))) + ext
}
`
	if _, err := Load(src); err != nil {
		t.Fatalf("白名单内的纯计算包应可用: %v", err)
	}
}
