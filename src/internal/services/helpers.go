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

// openFile 用系统默认关联程序打开单个文件（如视频调用默认播放器）。
func openFile(path string) error {
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return errors.Errorf("文件不存在: %s", path)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// 与 openDirectory 同策略：绝对路径调 explorer，传文件即按默认关联程序打开。
		explorer := "explorer.exe"
		if root := os.Getenv("SystemRoot"); root != "" {
			explorer = filepath.Join(root, "explorer.exe")
		}
		cmd = exec.Command(explorer, path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// SVC-11：回收子进程，避免 Unix 系上遗留 zombie
	go func() { _ = cmd.Wait() }()
	return nil
}

// revealFile 在系统文件管理器中打开目录并选中指定文件（类似鼠标选中的效果）。
func revealFile(path string) error {
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return errors.Errorf("文件不存在: %s", path)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Windows: explorer /select,"filepath"
		explorer := "explorer.exe"
		if root := os.Getenv("SystemRoot"); root != "" {
			explorer = filepath.Join(root, "explorer.exe")
		}
		// /select 参数需要紧跟逗号，然后是文件路径
		cmd = exec.Command(explorer, "/select,", path)
	case "darwin":
		// macOS: open -R filepath (Reveal in Finder)
		cmd = exec.Command("open", "-R", path)
	default:
		// Linux: 优先使用 nautilus，其次 dolphin，最后回退到打开目录
		if _, err := exec.LookPath("nautilus"); err == nil {
			cmd = exec.Command("nautilus", "--select", path)
		} else if _, err := exec.LookPath("dolphin"); err == nil {
			cmd = exec.Command("dolphin", "--select", path)
		} else {
			// 回退到打开所在目录
			return openDirectory(filepath.Dir(path))
		}
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// SVC-11：回收子进程，避免 Unix 系上遗留 zombie
	go func() { _ = cmd.Wait() }()
	return nil
}
