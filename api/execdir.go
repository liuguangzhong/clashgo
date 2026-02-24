package api

import "os"

// getExecDir 返回当前可执行文件所在目录
func getExecDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	info, err := os.Stat(exe)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return exe, nil
	}
	// filepath.Dir(exe)
	for i := len(exe) - 1; i >= 0; i-- {
		if exe[i] == '/' || exe[i] == '\\' {
			return exe[:i], nil
		}
	}
	return ".", nil
}
