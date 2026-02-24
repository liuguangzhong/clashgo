package utils

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// RegisterURLScheme 注册 clash:// / clash-verge:// URL scheme
func RegisterURLScheme() error {
	switch runtime.GOOS {
	case "linux":
		return registerURLSchemeLinux()
	case "windows":
		return registerURLSchemeWindows_impl()
	case "darwin":
		return nil // macOS 通过 Info.plist 静态注册
	}
	return nil
}

func registerURLSchemeLinux() error {
	home, _ := os.UserHomeDir()
	appsDir := home + "/.local/share/applications"
	if err := os.MkdirAll(appsDir, 0755); err != nil {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}

	lines := []string{
		"[Desktop Entry]",
		"Name=ClashGo URL Handler",
		"Exec=" + exePath + " --url-handler %u",
		"Type=Application",
		"NoDisplay=true",
		"MimeType=x-scheme-handler/clash;x-scheme-handler/clash-verge;",
	}
	content := strings.Join(lines, "\n") + "\n"

	desktopPath := appsDir + "/clashgo-url-handler.desktop"
	if err := os.WriteFile(desktopPath, []byte(content), 0644); err != nil {
		return err
	}

	for _, scheme := range []string{"clash", "clash-verge"} {
		_ = execRun("xdg-mime", []string{"default",
			"clashgo-url-handler.desktop",
			"x-scheme-handler/" + scheme})
	}
	_ = execRun("update-desktop-database", []string{appsDir})
	return nil
}
