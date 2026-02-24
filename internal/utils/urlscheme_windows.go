//go:build windows

package utils

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

func registerURLSchemeWindows_impl() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	schemes := []string{"clash", "clash-verge"}
	for _, scheme := range schemes {
		// HKCU\Software\Classes\clash\
		k, _, err := registry.CreateKey(registry.CURRENT_USER,
			`Software\Classes\`+scheme, registry.ALL_ACCESS)
		if err != nil {
			continue
		}
		_ = k.SetStringValue("", "URL:"+scheme+" Protocol")
		_ = k.SetStringValue("URL Protocol", "")
		k.Close()

		// HKCU\Software\Classes\clash\shell\open\command
		cmd, _, err := registry.CreateKey(registry.CURRENT_USER,
			`Software\Classes\`+scheme+`\shell\open\command`, registry.ALL_ACCESS)
		if err != nil {
			continue
		}
		_ = cmd.SetStringValue("", `"`+exePath+`" --url-handler "%1"`)
		cmd.Close()
	}
	return nil
}
