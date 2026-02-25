package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"clashgo/internal/utils"

	wailsLogger "github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"go.uber.org/zap"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 确保所有数据目录存在（首次运行时创建）
	if err := utils.EnsureDirectories(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create data directories: %v\n", err)
		os.Exit(1)
	}

	// 检查单例（防止多开）
	unlock, err := utils.AcquireSingleton()
	if err != nil {
		// 已有实例运行，向其发送激活信号后退出
		utils.NotifyExistingInstance()
		os.Exit(0)
	}
	defer unlock()

	// 初始化 logger
	log := utils.InitLogger()
	defer func() { _ = log.Sync() }()

	log.Info("ClashGo starting...")

	// NVIDIA GPU workaround for WebKit DMABUF
	applyNvidiaWorkaround()

	// 构建 App（所有模块在 App.startup() 中初始化）
	app := NewApp()

	// 优雅退出信号处理
	go handleSignals(app)

	// 启动 Wails
	err = wails.Run(&options.App{
		Title:             "ClashGo",
		Width:             1000,
		Height:            700,
		MinWidth:          600,
		MinHeight:         500,
		DisableResize:     false,
		Fullscreen:        false,
		Frameless:         false,
		StartHidden:       app.StartHidden(),
		HideWindowOnClose: true, // 关闭窗口时最小化到托盘

		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 18, G: 18, B: 18, A: 255},

		// 绑定所有后端方法到前端
		Bind: app.Bindings(),

		// 生命周期钩子
		OnStartup:  app.Startup,
		OnDomReady: app.DomReady,
		OnShutdown: app.Shutdown,

		// Linux 特有配置
		Linux: &linux.Options{
			ProgramName:         "clashgo",
			Icon:                readIcon(),
			WebviewGpuPolicy:    linux.WebviewGpuPolicyOnDemand,
			WindowIsTranslucent: false,
		},

		// 日志级别
		LogLevel:           wailsLogger.DEBUG,
		LogLevelProduction: wailsLogger.INFO,
	})

	if err != nil {
		log.Fatal("Failed to start Wails application", zap.Error(err))
	}
}

// handleSignals 处理系统信号，优雅退出
func handleSignals(app *App) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	sig := <-c
	utils.Log().Info("Received signal, shutting down...", zap.String("signal", sig.String()))
	app.gracefulShutdown()
	os.Exit(0)
}

// applyNvidiaWorkaround 检测 NVIDIA GPU 并设置 WebKit workaround
// 参考: src-tauri/src/utils/linux/workarounds.rs
func applyNvidiaWorkaround() {
	if _, ok := os.LookupEnv("WEBKIT_DISABLE_DMABUF_RENDERER"); ok {
		return
	}
	if hasNvidiaGPU() {
		os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
		utils.Log().Info("Detected NVIDIA GPU, set WEBKIT_DISABLE_DMABUF_RENDERER=1")
	}
}

// hasNvidiaGPU 检测当前系统是否有 NVIDIA GPU
func hasNvidiaGPU() bool {
	paths := []string{
		"/proc/driver/nvidia/version",
		"/sys/module/nvidia",
		"/sys/module/nvidia_drm",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}

	entries, err := os.ReadDir("/sys/class/drm")
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if len(name) < 4 || name[:4] != "card" {
			continue
		}
		vendorPath := "/sys/class/drm/" + name + "/device/vendor"
		data, err := os.ReadFile(vendorPath)
		if err != nil {
			continue
		}
		vendor := string(data)
		// NVIDIA vendor ID: 0x10de
		if len(vendor) >= 6 && (vendor[:6] == "0x10de" || vendor[:6] == "0x10DE") {
			return true
		}
	}
	return false
}

// readIcon 读取应用图标（用于 Linux 系统托盘）
func readIcon() []byte {
	data, err := os.ReadFile("./assets/icon.png")
	if err != nil {
		return nil
	}
	return data
}

// logger 包装（防止 Wails 日志级别冲突）
// 已移除，使用 wailsLogger.DEBUG 和 wailsLogger.INFO

// context 用于依赖注入（供 app.go 使用）
var _ = context.Background
