//go:build windows || darwin
// +build windows darwin

package hotkey

import hotkey "golang.design/x/hotkey"

// Windows / macOS 上 ModAlt 和 ModWin 是直接导出的
var (
	modAlt = hotkey.ModAlt
	modWin = hotkey.ModWin
)
