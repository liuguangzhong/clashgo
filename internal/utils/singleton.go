package utils

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"go.uber.org/zap"
)

// singletonData 保存单例锁相关信息
type singletonData struct {
	listener net.Listener
	path     string
}

var singleton *singletonData

// onActivate 外部注入的激活回调（App.Startup 中设置）
var onActivate func()

// SetActivationCallback 供 App.Startup 注入“显示主窗口”回调
func SetActivationCallback(fn func()) {
	onActivate = fn
}

// AcquireSingleton 尝试获取单例锁
func AcquireSingleton() (unlock func(), err error) {
	sockPath := singletonSocketPath()

	_ = os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		_ = os.Remove(sockPath)
		ln, err = net.Listen("unix", sockPath)
		if err != nil {
			return nil, fmt.Errorf("singleton: already running or cannot bind: %w", err)
		}
	}

	singleton = &singletonData{listener: ln, path: sockPath}

	// 接受来自后续实例的激活信号
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
			Log().Info("Received activation signal from new instance")
			// 调用外部注入的回调显示主窗口
			if onActivate != nil {
				onActivate()
			}
		}
	}()

	return func() {
		ln.Close()
		os.Remove(sockPath)
	}, nil
}

// NotifyExistingInstance 通知已运行的实例激活（显示主窗口）
func NotifyExistingInstance() {
	sockPath := singletonSocketPath()
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		Log().Warn("Failed to notify existing instance", zap.Error(err))
		return
	}
	conn.Close()
	Log().Info("Notified existing instance to activate")
}

// singletonSocketPath 返回单例 socket 路径（跨平台）
func singletonSocketPath() string {
	switch runtime.GOOS {
	case "windows":
		// Windows 使用命名管道替代 Unix socket
		// net.Listen("unix", ...) 在 Windows Go 1.23+ 实际支持 AF_UNIX
		appData := os.Getenv("APPDATA")
		return filepath.Join(appData, "clashgo", ".singleton")
	default:
		// Linux / macOS 使用 XDG_RUNTIME_DIR 或 /tmp
		runDir := os.Getenv("XDG_RUNTIME_DIR")
		if runDir == "" {
			runDir = "/tmp"
		}
		return filepath.Join(runDir, "clashgo.singleton")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Logger 初始化（全局 zap logger，模块共用）
// ─────────────────────────────────────────────────────────────────────────────

var (
	globalLog     *zap.Logger
	globalLogOnce sync.Once
)

// InitLogger 初始化全局 logger（应在程序最早期调用）
func InitLogger() *zap.Logger {
	globalLogOnce.Do(func() {
		cfg := zap.NewProductionConfig()
		cfg.OutputPaths = []string{
			"stdout",
			filepath.Join(Dirs().LogsDir(), "clashgo.log"),
		}
		cfg.ErrorOutputPaths = []string{"stderr"}

		var err error
		globalLog, err = cfg.Build(zap.AddCallerSkip(0))
		if err != nil {
			// fallback 到 nop logger
			globalLog = zap.NewNop()
		}
	})
	return globalLog
}

// Log 返回全局 logger（若未初始化则返回 Nop logger）
func Log() *zap.Logger {
	if globalLog == nil {
		return zap.NewNop()
	}
	return globalLog
}
