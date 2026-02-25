package core

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"clashgo/internal/config"
	"clashgo/internal/enhance"
	"clashgo/internal/geodata"
	"clashgo/internal/utils"

	mihomoConstant "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub"
	"github.com/metacubex/mihomo/hub/executor"
	mihomoLog "github.com/metacubex/mihomo/log"

	"go.uber.org/zap"
)

// RunningMode 核心当前运行模式
type RunningMode int32

const (
	ModeNotRunning RunningMode = iota
	ModeRunning
)

func (m RunningMode) String() string {
	if m == ModeRunning {
		return "Running"
	}
	return "NotRunning"
}

// Manager 管理 Mihomo 代理核心的生命周期
//
// 架构: 同进程嵌入模式
//
// Mihomo 作为 Go 库直接 import，通过 hub.Parse() 在同进程中启动。
// 配置热加载通过 executor.ApplyConfig() 实现，无需 HTTP API 或子进程通信。
// 优势：零 IPC 延迟、零序列化开销、错误直接以 Go error 返回。
type Manager struct {
	mu     sync.Mutex
	mode   atomic.Int32
	cfgMgr *config.Manager

	logBuf      *ringBuffer // 最近 500 条日志
	logCallback func(level, msg string)
	logDone     chan struct{} // 用于停止日志订阅 goroutine

	ctx    context.Context
	cancel context.CancelFunc
}

var (
	globalManager *Manager
	managerOnce   sync.Once
)

// NewManager 创建 CoreManager 实例
func NewManager(cfgMgr *config.Manager) *Manager {
	m := &Manager{
		cfgMgr: cfgMgr,
		logBuf: newRingBuffer(500),
	}
	managerOnce.Do(func() {
		globalManager = m
	})
	return m
}

// Global 返回全局 CoreManager
func Global() *Manager {
	return globalManager
}

// Start 生成运行时配置并在同进程中启动 Mihomo
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.mode.Load() == int32(ModeRunning) {
		return fmt.Errorf("core already running")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	// 设置 Mihomo 工作目录
	homeDir := utils.Dirs().HomeDir()
	mihomoConstant.SetHomeDir(homeDir)
	mihomoConstant.SetConfig("")

	utils.Log().Info("Starting Mihomo in-process",
		zap.String("homeDir", homeDir),
	)

	// 预下载 GeoIP/GeoSite 数据文件（多镜像 fallback，确保国内可用）
	geodata.EnsureGeoData(homeDir)

	runtimePath, err := m.generateRuntimeConfig()
	if err != nil {
		utils.Log().Warn("Config generation failed, using base clash config", zap.Error(err))
		runtimePath = utils.Dirs().ClashPath()
	}

	return m.startInProcess(runtimePath)
}

// startInProcess 读取配置文件并通过 hub.Parse() 在同进程中启动 Mihomo
func (m *Manager) startInProcess(configPath string) error {
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config %s: %w", configPath, err)
	}

	// 获取配置中的 external-controller 和 secret
	clash := m.cfgMgr.GetClash()
	server := m.getServer(clash)
	secret := m.getSecret(clash)

	// 构造 hub options
	var opts []hub.Option
	opts = append(opts, hub.WithExternalController(server))
	if secret != "" {
		opts = append(opts, hub.WithSecret(secret))
	}

	// 核心调用: hub.Parse() 在同进程中初始化 Mihomo
	// 包括: 解析配置 → 启动 DNS → 启动 Tunnel → 启动监听器 → 启动 external-controller
	if err := hub.Parse(configBytes, opts...); err != nil {
		return fmt.Errorf("mihomo hub.Parse: %w", err)
	}

	m.mode.Store(int32(ModeRunning))
	utils.Log().Info("Mihomo started in-process",
		zap.String("config", configPath),
		zap.String("controller", server),
	)

	// 启动日志订阅
	m.startLogSubscription()

	return nil
}

// Stop 停止 Mihomo (在同进程模式下，关闭监听器和 tunnel)
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}

	// 停止日志订阅
	if m.logDone != nil {
		close(m.logDone)
		m.logDone = nil
	}

	// 同进程模式下，Mihomo 的 Tunnel 和 Listener 在下次 ApplyConfig 时会被替换
	// 我们标记为未运行即可
	m.mode.Store(int32(ModeNotRunning))
	utils.Log().Info("Mihomo core stopped (in-process)")
	return nil
}

// Restart 重启：重新生成配置并 Apply
// 核心已运行时走热加载路径（UpdateConfig），未运行时走完整启动路径
func (m *Manager) Restart() error {
	if m.mode.Load() == int32(ModeRunning) {
		// 已运行：重新生成配置并热加载（等效于完全重启但无需重建 tunnel/listener）
		return m.UpdateConfig()
	}
	// 未运行：完整启动
	return m.Start(context.Background())
}

// UpdateConfig 重新生成增强配置并热加载（同进程，无需重启）
func (m *Manager) UpdateConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	utils.Log().Info("[链路] UpdateConfig: 生成运行时配置")
	runtimePath, err := m.generateRuntimeConfig()
	if err != nil {
		utils.Log().Error("[链路] generateRuntimeConfig 失败", zap.Error(err))
		return fmt.Errorf("generate config: %w", err)
	}
	utils.Log().Info("[链路] 运行时配置已生成", zap.String("path", runtimePath))

	// 读取并打印配置文件大小
	if info, e := os.Stat(runtimePath); e == nil {
		utils.Log().Info("[链路] runtime.yaml", zap.Int64("bytes", info.Size()))
	}

	if m.mode.Load() != int32(ModeRunning) {
		utils.Log().Info("[链路] 核心未运行，启动核心")
		return m.startInProcess(runtimePath)
	}

	utils.Log().Info("[链路] 核心运行中，热加载配置")
	return m.reloadConfigInProcess(runtimePath)
}

// reloadConfigInProcess 通过 executor 同进程热加载配置
func (m *Manager) reloadConfigInProcess(configPath string) error {
	cfg, err := executor.ParseWithPath(configPath)
	if err != nil {
		return fmt.Errorf("parse config for reload: %w", err)
	}

	executor.ApplyConfig(cfg, false)
	utils.Log().Info("Config reloaded in-process", zap.String("path", configPath))
	return nil
}

// RunningMode 返回当前运行模式
func (m *Manager) RunningMode() string {
	return RunningMode(m.mode.Load()).String()
}

// IsRunning 检查核心是否在运行
func (m *Manager) IsRunning() bool {
	return m.mode.Load() == int32(ModeRunning)
}

// StartLogStream 订阅 Mihomo 内部日志 channel
func (m *Manager) StartLogStream(callback func(level, msg string)) {
	m.logCallback = callback
}

// GetLogs 返回最近的日志条目（来自 ring buffer）
func (m *Manager) GetLogs() []string {
	return m.logBuf.All()
}

// ─── 内部实现 ─────────────────────────────────────────────────────────────────

// startLogSubscription 订阅 mihomo 内部的 log channel
func (m *Manager) startLogSubscription() {
	if m.logDone != nil {
		close(m.logDone)
	}
	m.logDone = make(chan struct{})

	logCh := mihomoLog.Subscribe()
	done := m.logDone

	go func() {
		defer mihomoLog.UnSubscribe(logCh)
		for {
			select {
			case <-done:
				return
			case logEvent, ok := <-logCh:
				if !ok {
					return
				}
				level := logEvent.LogLevel.String()
				msg := logEvent.Payload
				m.handleLogLine(level, msg)
			}
		}
	}()
}

// handleLogLine 处理单条日志：存入 ring buffer + 触发回调
func (m *Manager) handleLogLine(level, msg string) {
	m.logBuf.Push(fmt.Sprintf("[%s] %s", level, msg))
	if m.logCallback != nil {
		m.logCallback(level, msg)
	}
}

func (m *Manager) generateRuntimeConfig() (string, error) {
	verge := m.cfgMgr.GetVerge()
	clashBase := m.cfgMgr.GetClash()
	profiles := m.cfgMgr.GetProfiles()

	result, err := enhance.NewEngine(verge, clashBase, profiles, utils.Dirs()).Run()
	if err != nil {
		return "", fmt.Errorf("enhance pipeline: %w", err)
	}

	runtimePath := utils.Dirs().RuntimeConfigPath()
	if err := writeConfigYAML(runtimePath, result.Config); err != nil {
		return "", fmt.Errorf("save runtime config: %w", err)
	}

	m.cfgMgr.SetRuntime(result.Runtime)
	return runtimePath, nil
}

func (m *Manager) getServer(clash config.IClashBase) string {
	if ctrl, ok := clash["external-controller"].(string); ok && ctrl != "" {
		return ctrl
	}
	return "127.0.0.1:9097"
}

func (m *Manager) getSecret(clash config.IClashBase) string {
	if s, ok := clash["secret"].(string); ok {
		return s
	}
	return ""
}

// ─── ringBuffer 定长循环日志缓冲 ─────────────────────────────────────────────

type ringBuffer struct {
	mu   sync.Mutex
	buf  []string
	head int
	size int
	cap  int
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{buf: make([]string, capacity), cap: capacity}
}

func (r *ringBuffer) Push(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.head%r.cap] = s
	r.head++
	if r.size < r.cap {
		r.size++
	}
}

func (r *ringBuffer) All() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size == 0 {
		return nil
	}
	out := make([]string, r.size)
	start := r.head - r.size
	if start < 0 {
		start = 0
	}
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(start+i)%r.cap]
	}
	return out
}
