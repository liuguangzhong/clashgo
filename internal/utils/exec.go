package utils

import "os/exec"

// execRun 执行命令（工具函数，供 urlscheme.go 等使用）
func execRun(name string, args []string) error {
	return exec.Command(name, args...).Run()
}

// regSetURLScheme 注册 Windows URL scheme（build tag 控制平台）
func regSetURLScheme() error {
	return registerURLSchemeWindows_impl()
}
