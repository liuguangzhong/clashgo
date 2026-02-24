package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"clashgo/internal/config"
	"clashgo/internal/enhance"
	"clashgo/internal/utils"

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
// 架构决策：使用 Sidecar 模式（外部进程）
//
// 原因：Mihomo 将自身作为独立进程设计，其 hub 包的初始化
// 依赖全局状态（init() 副作用），在同一进程内多次初始化
// 会产生竞争。Sidecar 模式通过成熟的 REST API 通信，
// 与 Mihomo 官方维护的 external-controller 对齐，是最稳定的选择。
type Manager struct {
	mu     sync.Mutex
	mode   atomic.Int32
	cfgMgr *config.Manager

	cmd    *exec.Cmd
	logBuf *ringBuffer // 最近 500 条日志

	logCallback func(level, msg string)

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

// Start 生成运行时配置并启动 Mihomo 子进程
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.mode.Load() == int32(ModeRunning) {
		return fmt.Errorf("core already running")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	runtimePath, err := m.generateRuntimeConfig()
	if err != nil {
		utils.Log().Warn("Config generation failed, using base clash config", zap.Error(err))
		runtimePath = utils.Dirs().ClashPath()
	}

	return m.startProcess(runtimePath)
}

// Stop 向 Mihomo 进程发送终止信号并等待退出
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}

	if m.cmd != nil && m.cmd.Process != nil {
		if err := m.cmd.Process.Kill(); err != nil {
			utils.Log().Warn("Kill mihomo failed", zap.Error(err))
		}
		m.cmd = nil
	}

	m.mode.Store(int32(ModeNotRunning))
	utils.Log().Info("Mihomo core stopped")
	return nil
}

// Restart 重启：先停止，等待 500ms 确保端口释放，再启动
func (m *Manager) Restart() error {
	if err := m.Stop(); err != nil {
		utils.Log().Warn("Stop failed during restart", zap.Error(err))
	}
	time.Sleep(500 * time.Millisecond)
	return m.Start(context.Background())
}

// UpdateConfig 重新生成增强配置，通过 Mihomo HTTP API 热加载（无需重启）
func (m *Manager) UpdateConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	runtimePath, err := m.generateRuntimeConfig()
	if err != nil {
		return fmt.Errorf("generate config: %w", err)
	}

	if m.mode.Load() != int32(ModeRunning) {
		// 核心未运行时，直接启动
		return m.startProcess(runtimePath)
	}

	return m.reloadConfig(runtimePath)
}

// RunningMode 返回当前运行模式
func (m *Manager) RunningMode() string {
	return RunningMode(m.mode.Load()).String()
}

// IsRunning 检查进程是否在运行
func (m *Manager) IsRunning() bool {
	return m.mode.Load() == int32(ModeRunning)
}

// StartLogStream 通过 Mihomo /logs WebSocket 端点订阅实时日志
// Mihomo 日志格式: {"type":"info","payload":"..."}
func (m *Manager) StartLogStream(callback func(level, msg string)) {
	m.logCallback = callback

	clash := m.cfgMgr.GetClash()
	server := m.getServer(clash)
	secret := m.getSecret(clash)

	go m.streamLogs(server, secret)
}

// GetLogs 返回最近的日志条目（来自 ring buffer）
func (m *Manager) GetLogs() []string {
	return m.logBuf.All()
}

// ─── 内部实现 ─────────────────────────────────────────────────────────────────

func (m *Manager) startProcess(configPath string) error {
	verge := m.cfgMgr.GetVerge()
	coreName := verge.GetClashCore()
	corePath := utils.Dirs().CoreBinaryPath(coreName)

	args := []string{
		"-d", utils.Dirs().HomeDir(),
		"-f", configPath,
	}

	// 根据平台选择 IPC 方式
	switch runtime.GOOS {
	case "windows":
		// Windows 使用命名管道
		args = append(args, "-ext-ctl-pipe", utils.Dirs().ServiceIPCPath())
	default:
		// Linux/macOS 使用 Unix Domain Socket
		args = append(args, "-ext-ctl-unix", utils.Dirs().ServiceIPCPath())
	}

	m.cmd = exec.CommandContext(m.ctx, corePath, args...)
	m.cmd.Stdout = &logWriter{callback: m.handleLogLine, level: "info"}
	m.cmd.Stderr = &logWriter{callback: m.handleLogLine, level: "error"}

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", corePath, err)
	}

	m.mode.Store(int32(ModeRunning))
	utils.Log().Info("Mihomo started",
		zap.String("core", coreName),
		zap.String("path", corePath),
		zap.Int("pid", m.cmd.Process.Pid),
	)

	// 等待进程退出，自动更新状态
	go func() {
		err := m.cmd.Wait()
		m.mode.Store(int32(ModeNotRunning))
		if err != nil {
			utils.Log().Warn("Mihomo process exited", zap.Error(err))
		} else {
			utils.Log().Info("Mihomo process exited cleanly")
		}
	}()

	return nil
}

// reloadConfig 通过 Mihomo REST API 热加载配置（无需重启进程）
func (m *Manager) reloadConfig(configPath string) error {
	clash := m.cfgMgr.GetClash()
	return reloadMihomoConfig(m.getServer(clash), m.getSecret(clash), configPath)
}

// streamLogs 长轮询 Mihomo /logs 端点，解析 NDJSON 日志流
func (m *Manager) streamLogs(server, secret string) {
	url := "http://" + server + "/logs"
	if secret != "" {
		url += "?token=" + secret
	}

	for {
		if m.ctx == nil {
			return
		}
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		req, err := http.NewRequestWithContext(m.ctx, http.MethodGet, url, nil)
		if err != nil {
			return
		}
		if secret != "" {
			req.Header.Set("Authorization", "Bearer "+secret)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			// 连接失败（核心尚未就绪），等待后重试
			time.Sleep(2 * time.Second)
			continue
		}

		m.readLogStream(resp)
		resp.Body.Close()

		// 连接断开后等待重连
		time.Sleep(1 * time.Second)
	}
}

// readLogStream 逐行读取 NDJSON 日志流
func (m *Manager) readLogStream(resp *http.Response) {
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry struct {
			Type    string `json:"type"`
			Payload string `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		m.handleLogLine(entry.Type, entry.Payload)
	}
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

// ─── logWriter ─────────────────────────────────────────────────────────────

type logWriter struct {
	callback func(level, msg string)
	level    string
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	if w.callback != nil {
		w.callback(w.level, string(p))
	}
	return len(p), nil
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
