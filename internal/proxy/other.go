//go:build linux

package proxy

func newPlatformProxy() SysProxy { return newLinuxProxy() }
