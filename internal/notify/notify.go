// Package notify 系统通知
package notify

import (
	"os/exec"
	"runtime"
)

// Send 发送系统通知
func Send(title, body string) {
	switch runtime.GOOS {
	case "linux":
		_ = exec.Command("notify-send", title, body).Start()
	case "darwin":
		_ = exec.Command("osascript", "-e",
			`display notification "`+body+`" with title "`+title+`"`).Start()
	case "windows":
		// Windows 通过 Wails 事件或 toast 实现
	}
}
