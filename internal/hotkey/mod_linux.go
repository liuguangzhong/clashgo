//go:build linux
// +build linux

package hotkey

import hotkey "golang.design/x/hotkey"

// Linux (X11) 上没有 ModAlt/ModWin，对应关系:
//   Alt   → Mod1 (X11 Mod1Mask)
//   Super → Mod4 (X11 Mod4Mask)
// 参考: /usr/include/X11/X.h
var (
	modAlt = hotkey.Mod1
	modWin = hotkey.Mod4
)
