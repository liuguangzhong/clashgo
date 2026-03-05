package core

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"clashgo/internal/config"
	"clashgo/internal/enhance"
	"clashgo/internal/geodata"
	"clashgo/internal/proxy"
	"clashgo/internal/utils"

	"go.uber.org/zap"
)

// ── 运行状态 ──────────────────────────────────────────────────────────────────

// RunningMode 对应 Clash Verge Rev 的 RunningMode 枚举
//
//	原版 Rust：Service / Sidecar / NotRunning
//	本实现：  Sidecar / NotRunning（无系统服务路径，直接管理子进程）
type RunningMode int32

const (
	ModeNotRunning RunningMode = iota
	ModeSidecar                // mihomo 以子进程（sidecar）方式运行
)

func (m RunningMode) String() string {
	switch m {
	case ModeSidecar:
		return "Sidecar"
	default:
		return "NotRunning"
	}
}

// configUpdateDebounce 对应原版 timing::CONFIG_UPDATE_DEBOUNCE
// 防止配置变更事件风暴导致频繁重载
const configUpdateDebounce = 500 * time.Millisecond

// apiReadyTimeout 等待 mihomo HTTP API 就绪的超时时间
const apiReadyTimeout = 15 * time.Second

// ── Manager ───────────────────────────────────────────────────────────────────

// Manager 管理自实现代理内核的生命周期
//
// 架构：自实现内核模式（internal/proxy 包）
//
//	不依赖 github.com/metacubex/mihomo 库，
//	自己实现 Tunnel / Mixed Listener / Rules / Outbound。
//
// 生命周期（对照原版 lifecycle.rs）：
//
//	Start  → generateRuntime → proxy.Parse(runtime.yaml)
//	Reload → generateRuntime → proxy.ApplyConfig  [debounce]
//	Stop   → proxy.Kernel.Stop()
type Manager struct {
	mu   sync.Mutex
	mode atomic.Int32

	cfgMgr *config.Manager
	kernel *proxy.Kernel // 自实现代理内核

	// 日志（对应原版 CLASH_LOGGER AsyncLogger）
	logBuf      *ringBuffer
	logCallback func(level, msg string)

	// 防抖（对应原版 last_update + timing::CONFIG_UPDATE_DEBOUNCE）
	lastUpdate time.Time

	ctx    context.Context
	cancel context.CancelFunc
}

var (
	globalManager *Manager
	managerOnce   sync.Once
)

// NewManager 创建 CoreManager 单例（对应原版 CoreManager::new）
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

// ── 生命周期（对照原版 lifecycle.rs）────────────────────────────────────────

// Start 启动自实现代理内核
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.mode.Load() == int32(ModeSidecar) {
		return fmt.Errorf("core already running")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	homeDir := utils.Dirs().HomeDir()
	utils.Log().Info("启动自实现代理内核", zap.String("homeDir", homeDir))

	// 预下载 GeoData
	geodata.EnsureGeoData(homeDir)

	// 生成运行时配置
	runtimePath, err := m.generateRuntimeConfig()
	if err != nil {
		utils.Log().Warn("配置生成失败", zap.Error(err))
		runtimePath = utils.Dirs().ClashPath()
	}

	// 读取 runtime.yaml 并启动自实现内核
	configBytes, err := os.ReadFile(runtimePath)
	if err != nil {
		return fmt.Errorf("read runtime config: %w", err)
	}

	if err := proxy.Parse(configBytes, utils.Log()); err != nil {
		return fmt.Errorf("proxy kernel parse: %w", err)
	}

	m.kernel = proxy.GlobalKernel()
	m.mode.Store(int32(ModeSidecar))
	utils.Log().Info("自实现代理内核已启动")
	return nil
}

// Stop 停止内核
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}

	if m.kernel != nil {
		m.kernel.Stop()
		m.kernel = nil
	}

	m.mode.Store(int32(ModeNotRunning))
	utils.Log().Info("自实现代理内核已停止")
	return nil
}

// Restart 重启内核（对应原版 restart_core = stop_core + start_core）
func (m *Manager) Restart() error {
	utils.Log().Info("Restarting Mihomo sidecar")
	if err := m.Stop(); err != nil {
		utils.Log().Warn("Stop failed during restart", zap.Error(err))
	}
	return m.Start(context.Background())
}

// UpdateConfig 重新生成增强配置并热加载（对应原版 update_config → apply_config → reload_config）
//
// 包含 debounce 防抖：短时间内重复调用直接跳过。
// 用于配置变更事件（如开关系统代理），用户显式切换订阅请使用 ForceUpdateConfig。
func (m *Manager) UpdateConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Debounce（对应原版 should_update_config）
	now := time.Now()
	if !m.lastUpdate.IsZero() && now.Sub(m.lastUpdate) < configUpdateDebounce {
		utils.Log().Info("UpdateConfig debounced（跳过）",
			zap.Duration("elapsed", now.Sub(m.lastUpdate)),
		)
		return nil
	}
	m.lastUpdate = now
	return m.doUpdate()
}

// ForceUpdateConfig 强制重新生成配置并热加载，忽略 debounce。
// 用于用户显式切换/激活订阅，必须保证执行。
func (m *Manager) ForceUpdateConfig() error {
	m.mu.Lock()
	m.lastUpdate = time.Time{} // 重置 debounce 计时器
	m.mu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.doUpdate()
}

// doUpdate 执行实际的配置生成+热加载（必须在 m.mu.Lock() 下调用）
func (m *Manager) doUpdate() error {
	utils.Log().Info("UpdateConfig: generating runtime config")

	runtimePath, err := m.generateRuntimeConfig()
	if err != nil {
		utils.Log().Error("generateRuntimeConfig failed", zap.Error(err))
		return fmt.Errorf("generate config: %w", err)
	}

	if info, e := os.Stat(runtimePath); e == nil {
		utils.Log().Info("runtime.yaml ready", zap.Int64("bytes", info.Size()))
	}

	// 核心未运行：完整启动
	if m.mode.Load() != int32(ModeSidecar) {
		utils.Log().Info("内核未运行，执行完整启动")
		return m.Start(context.Background())
	}

	// 内核运行中：热加载
	utils.Log().Info("内核运行中，热加载配置")
	return m.reloadViaKernel(runtimePath)
}

// ── 子进程管理（内部）────────────────────────────────────────────────────────

// (startSidecar removed — using internal proxy kernel instead)

// reloadViaKernel 通过自实现内核热加载
func (m *Manager) reloadViaKernel(runtimePath string) error {
	if m.kernel == nil {
		return fmt.Errorf("kernel not initialized")
	}
	configBytes, err := os.ReadFile(runtimePath)
	if err != nil {
		return fmt.Errorf("read runtime config: %w", err)
	}
	cfg, err := proxy.ParseConfig(configBytes)
	if err != nil {
		return fmt.Errorf("parse kernel config: %w", err)
	}
	if err := m.kernel.ApplyConfig(cfg); err != nil {
		return fmt.Errorf("apply kernel config: %w", err)
	}
	utils.Log().Info("内核配置热加载成功", zap.String("path", runtimePath))
	return nil
}

// generateRuntimeConfig 运行增强流水线，生成 runtime.yaml
// 对应原版 Config::generate → save_yaml(runtime.yaml)
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
		return "", fmt.Errorf("write runtime.yaml: %w", err)
	}

	m.cfgMgr.SetRuntime(result.Runtime)
	return runtimePath, nil
}

// getCoreName 返回当前配置的核心名称（默认 verge-mihomo）
func (m *Manager) getCoreName() string {
	verge := m.cfgMgr.GetVerge()
	return verge.GetClashCore()
}

// ── 状态查询 ──────────────────────────────────────────────────────────────────

// RunningMode 返回当前运行模式字符串
func (m *Manager) RunningMode() string {
	return RunningMode(m.mode.Load()).String()
}

// IsRunning 返回核心是否在运行
func (m *Manager) IsRunning() bool {
	return m.mode.Load() == int32(ModeSidecar)
}

// ── 日志 API ──────────────────────────────────────────────────────────────────

// StartLogStream 注册前端日志实时回调
func (m *Manager) StartLogStream(callback func(level, msg string)) {
	m.logCallback = callback
}

// GetLogs 返回最近日志条目
func (m *Manager) GetLogs() []string {
	return m.logBuf.All()
}

// ── 工具函数 ──────────────────────────────────────────────────────────────────

// extractString 从 map 安全读取字符串，不存在时返回 defaultVal
func extractString(m map[string]interface{}, key, defaultVal string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return defaultVal
}

// ── ringBuffer：定长循环日志缓冲 ─────────────────────────────────────────────
//
// 数据结构：环形数组（ring buffer）
//   - head：下一次写入槽位的绝对偏移（单调递增，不对 cap 取模）
//   - size：当前有效条目数，上限 = cap
//
// 读取 oldest 槽位：
//   - 未满（size < cap）：oldest = 0
//   - 已满（size == cap）：oldest = head % cap

type ringBuffer struct {
	mu   sync.Mutex
	buf  []string
	head int // 下一次写入的绝对偏移（单调递增）
	size int // 当前有效条目数
	cap  int // 容量上限
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{buf: make([]string, capacity), cap: capacity}
}

// Push 写入一条日志；buffer 满时覆盖最旧条目
func (r *ringBuffer) Push(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.head%r.cap] = s
	r.head++
	if r.size < r.cap {
		r.size++
	}
}

// All 按时间顺序（最旧→最新）返回全部条目的深拷贝
func (r *ringBuffer) All() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size == 0 {
		return nil
	}
	out := make([]string, r.size)
	var oldest int
	if r.size < r.cap {
		oldest = 0 // 未满：从 0 顺序读
	} else {
		oldest = r.head % r.cap // 已满：head 指向即将被覆盖的最旧条目
	}
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(oldest+i)%r.cap]
	}
	return out
}
