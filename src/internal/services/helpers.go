package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/go-faster/errors"
)

// userHomeDir 便于测试替换的家目录获取。
var userHomeDir = os.UserHomeDir

// appendTData 补全 Telegram Desktop 数据目录的 tdata 后缀
// （对应 ref/tdl/app/login/desktop.go 的 appendTData）。
func appendTData(path string) string {
	if filepath.Base(path) != "tdata" {
		path = filepath.Join(path, "tdata")
	}
	return path
}

// openDirectory 用系统文件管理器打开目录。
func openDirectory(dir string) error {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return errors.Errorf("目录不存在: %s", dir)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// dev 环境子进程的 PATH 可能不含系统目录，使用绝对路径调用 explorer。
		explorer := "explorer.exe"
		if root := os.Getenv("SystemRoot"); root != "" {
			explorer = filepath.Join(root, "explorer.exe")
		}
		cmd = exec.Command(explorer, dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// SVC-11：回收子进程，避免 Unix 系上遗留 zombie
	go func() { _ = cmd.Wait() }()
	return nil
}
