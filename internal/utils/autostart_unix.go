//go:build !windows

package utils

// regSetAutostart Linux/macOS 不使用注册表
func regSetAutostart(_ bool) error { return nil }

// regIsAutostartEnabled Linux/macOS 不使用注册表
func regIsAutostartEnabled() bool { return false }
