package services

import (
	"os"
	"path/filepath"
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
