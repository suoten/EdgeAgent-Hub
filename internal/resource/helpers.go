package resource

import (
	"os"
	"os/exec"
)

// osStat 包装 os.Stat 以便测试
func osStat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// osReadFile 包装 os.ReadFile
func osReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// execLookPath 包装 exec.LookPath
func execLookPath(cmd string) (string, error) {
	return exec.LookPath(cmd)
}
