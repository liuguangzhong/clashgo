//go:build windows

package utils

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

const regRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const regAppName = "ClashGo"

func regSetAutostart(enable bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, regRunKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	if enable {
		exePath, err := os.Executable()
		if err != nil {
			return err
		}
		return k.SetStringValue(regAppName, `"`+exePath+`" --start-hidden`)
	}
	return k.DeleteValue(regAppName)
}

func regIsAutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, regRunKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(regAppName)
	return err == nil
}
