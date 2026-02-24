package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// AppDirs 所有应用目录路径（对应原 utils/dirs.rs）
type AppDirs struct {
	home      string // ~/.config/clashgo
	profiles  string // ~/.config/clashgo/profiles
	logs      string // ~/.config/clashgo/logs
	resources string // /usr/lib/clashgo
	portable  bool   // 便携模式标志
}

var (
	dirs     *AppDirs
	dirsOnce sync.Once
)

// Dirs 返回全局目录实例（单例）
func Dirs() *AppDirs {
	dirsOnce.Do(func() {
		dirs = &AppDirs{}
		dirs.init()
	})
	return dirs
}

// init 根据平台初始化目录
func (d *AppDirs) init() {
	// 可移植模式：如果可执行文件旁有 .portable 文件，使用可执行文件所在目录
	if exeDir, err := execDir(); err == nil {
		portableFlag := filepath.Join(exeDir, ".portable")
		if _, err := os.Stat(portableFlag); err == nil {
			d.portable = true
			d.home = filepath.Join(exeDir, "data")
			d.profiles = filepath.Join(d.home, "profiles")
			d.logs = filepath.Join(d.home, "logs")
			d.resources = filepath.Join(exeDir, "resources")
			return
		}
	}

	// 标准模式：平台数据目录
	switch runtime.GOOS {
	case "linux":
		// XDG_CONFIG_HOME 优先，否则 ~/.config
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			home, _ := os.UserHomeDir()
			configHome = filepath.Join(home, ".config")
		}
		d.home = filepath.Join(configHome, "clashgo")
		d.resources = "/usr/lib/clashgo"

	case "darwin":
		home, _ := os.UserHomeDir()
		d.home = filepath.Join(home, "Library", "Application Support", "clashgo")
		d.resources = "/Applications/clashgo.app/Contents/Resources"

	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		d.home = filepath.Join(appData, "clashgo")
		if exeDir, err := execDir(); err == nil {
			d.resources = filepath.Join(exeDir, "resources")
		}
	}

	d.profiles = filepath.Join(d.home, "profiles")
	d.logs = filepath.Join(d.home, "logs")
}

// ── 路径访问方法 ──────────────────────────────────────────────────────────────

func (d *AppDirs) HomeDir() string      { return d.home }
func (d *AppDirs) ProfilesDir() string  { return d.profiles }
func (d *AppDirs) LogsDir() string      { return d.logs }
func (d *AppDirs) ResourcesDir() string { return d.resources }
func (d *AppDirs) IsPortable() bool     { return d.portable }

// LatestLogPath 返回最新的应用日志文件路径
// 优先找 logs/ 目录下最近修改的 clashgo*.log，否则返回固定名
func (d *AppDirs) LatestLogPath() string {
	return latestLogFile(d.logs, "clashgo", "clashgo.log")
}

// LatestCoreLogPath 返回最新的核心（Mihomo）日志文件路径
// 优先找 logs/ 目录下最近修改的 mihomo*.log，否则返回固定名
func (d *AppDirs) LatestCoreLogPath() string {
	return latestLogFile(d.logs, "mihomo", "mihomo.log")
}

// latestLogFile 在 dir 下找前缀匹配的最新 .log 文件
// 若找不到则返回 dir/fallback
func latestLogFile(dir, prefix, fallback string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return filepath.Join(dir, fallback)
	}
	var bestPath string
	var bestTime time.Time
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(bestTime) {
			bestTime = info.ModTime()
			bestPath = filepath.Join(dir, name)
		}
	}
	if bestPath != "" {
		return bestPath
	}
	return filepath.Join(dir, fallback)
}

// VergePath verge.yaml 完整路径
func (d *AppDirs) VergePath() string {
	return filepath.Join(d.home, "verge.yaml")
}

// ClashPath clash.yaml 完整路径（Mihomo 基础配置）
func (d *AppDirs) ClashPath() string {
	return filepath.Join(d.home, "clash.yaml")
}

// ProfilesPath profiles.yaml 完整路径
func (d *AppDirs) ProfilesPath() string {
	return filepath.Join(d.home, "profiles.yaml")
}

// RuntimeConfigPath 增强后运行时配置路径
func (d *AppDirs) RuntimeConfigPath() string {
	return filepath.Join(d.home, "runtime.yaml")
}

// DNSConfigPath DNS 自定义配置路径
func (d *AppDirs) DNSConfigPath() string {
	return filepath.Join(d.home, "dns_config.yaml")
}

// WindowStatePath 窗口状态持久化路径
func (d *AppDirs) WindowStatePath() string {
	return filepath.Join(d.home, "window_state.json")
}

// BackupDir 本地备份目录
func (d *AppDirs) BackupDir() string {
	return filepath.Join(d.home, "backups")
}

// ServiceIPCPath Unix socket / Windows pipe 路径（与 Mihomo 通信）
// 注意：在 Go 嵌入模式下，此路径仅供外部工具（yacd/metacubex 面板）使用
func (d *AppDirs) ServiceIPCPath() string {
	switch runtime.GOOS {
	case "windows":
		return `\\.\pipe\clashgo-mihomo`
	default:
		return filepath.Join(d.home, "clashgo.ipc")
	}
}

// ProfileFile 返回某个订阅文件的完整路径
func (d *AppDirs) ProfileFile(filename string) string {
	return filepath.Join(d.profiles, filename)
}

// CoreBinaryPath 根据核心名返回二进制路径（与可执行文件同目录）
func (d *AppDirs) CoreBinaryPath(coreName string) string {
	exeDir, err := execDir()
	if err != nil {
		return coreName
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(exeDir, coreName+".exe")
	}
	return filepath.Join(exeDir, coreName)
}

// ── 工具函数 ──────────────────────────────────────────────────────────────────

// EnsureDirectories 创建所有必要目录（应用启动时调用一次）
func EnsureDirectories() error {
	d := Dirs()
	dirs := []string{
		d.home,
		d.profiles,
		d.logs,
		d.BackupDir(),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}
	return nil
}

// execDir 返回当前可执行文件所在目录
func execDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// PathToStr 将 path 转为字符串（Windows 长路径安全处理）
func PathToStr(path string) string {
	return filepath.ToSlash(path)
}
