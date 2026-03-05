package core

// process.go — Mihomo 子进程（sidecar）生命周期管理
//
// 对应 Clash Verge Rev 原版的 CommandChild（tauri-plugin-shell 进程管理）。
// 完全使用 Go 标准库 os/exec 实现，无任何外部库依赖。

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// sidecarProcess 封装一个正在运行的 mihomo 子进程
// 对应原版 CommandChild（tauri-plugin-shell）
type sidecarProcess struct {
	cmd    *exec.Cmd
	mu     sync.Mutex
	killed bool
}

// spawnSidecar 启动 mihomo 二进制为子进程
//
// 参数：
//   - binPath：mihomo 可执行文件路径（由 CoreBinaryPath 获取）
//   - configPath：runtime.yaml 路径（-f 参数传入）
//   - homeDir：mihomo 工作目录（-d 参数传入）
//   - logCallback：每行日志回调，参数为 (level, message)
//
// 返回的 sidecarProcess 无需显式 Wait，goroutine 内部异步处理。
func spawnSidecar(
	binPath, configPath, homeDir string,
	logCallback func(level, msg string),
) (*sidecarProcess, error) {
	// 检查二进制存在
	if _, err := os.Stat(binPath); err != nil {
		return nil, fmt.Errorf("mihomo binary not found at %s: %w", binPath, err)
	}

	// 构造命令：mihomo -d <homeDir> -f <configPath>
	cmd := exec.Command(binPath, "-d", homeDir, "-f", configPath)

	// 从 stdout/stderr 捕获日志
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mihomo process: %w", err)
	}

	sp := &sidecarProcess{cmd: cmd}

	// goroutine：读取 stdout 并解析日志行
	go sp.pipeLog(stdout, logCallback)
	// goroutine：读取 stderr（错误输出）
	go sp.pipeLog(stderr, logCallback)

	// goroutine：等待进程退出，防止僵尸进程
	go func() {
		_ = cmd.Wait()
	}()

	return sp, nil
}

// kill 向子进程发送终止信号（幂等）
// 对应原版 CommandChild::kill()
func (sp *sidecarProcess) kill() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if sp.killed {
		return
	}
	sp.killed = true
	if sp.cmd != nil && sp.cmd.Process != nil {
		_ = sp.cmd.Process.Kill()
	}
}

// pid 返回子进程 PID（调试用）
func (sp *sidecarProcess) pid() int {
	if sp.cmd != nil && sp.cmd.Process != nil {
		return sp.cmd.Process.Pid
	}
	return -1
}

// pipeLog 逐行读取 reader，解析级别并回调
// Mihomo 日志格式示例：
//
//	time="2024-01-01T00:00:00Z" level=info msg="Start initial compatible provider default"
//	INFO[2024-01-01T00:00:00Z] Starting proxy...
func (sp *sidecarProcess) pipeLog(r io.Reader, callback func(level, msg string)) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		level, msg := parseMihomoLogLine(line)
		if callback != nil {
			callback(level, msg)
		}
	}
}

// parseMihomoLogLine 解析 mihomo 输出的日志行，返回 (level, message)
//
// mihomo 支持两种日志格式（取决于 log-level 配置）：
//
//	logrus 文本格式：time="..." level=info msg="..."
//	简洁格式：       INFO[timestamp] message...
func parseMihomoLogLine(line string) (level, msg string) {
	// 优先尝试解析 logrus key=value 格式
	// 示例：time="2024-01-01T00:00:00Z" level=info msg="DNS server started"
	if strings.Contains(line, "level=") {
		level = extractKV(line, "level")
		msg = extractKV(line, "msg")
		if level != "" && msg != "" {
			return strings.ToLower(level), msg
		}
	}

	// 尝试解析简洁前缀格式
	// 示例：INFO[0001] DNS server started
	upper := strings.ToUpper(line)
	for _, lvl := range []string{"ERRO", "WARN", "INFO", "DEBU", "TRAC"} {
		if strings.HasPrefix(upper, lvl) {
			// 跳过 "XXXX[timestamp] " 前缀
			rest := line
			if idx := strings.Index(rest, "] "); idx >= 0 {
				rest = rest[idx+2:]
			}
			return strings.ToLower(lvl[:4]), rest
		}
	}

	// 无法识别格式：整行作为 info 消息
	return "info", line
}

// extractKV 从 logrus 格式日志行中提取 key=value 或 key="value"
func extractKV(line, key string) string {
	prefix := key + "="
	idx := strings.Index(line, prefix)
	if idx < 0 {
		return ""
	}
	rest := line[idx+len(prefix):]
	if len(rest) == 0 {
		return ""
	}
	if rest[0] == '"' {
		// 带引号的值：找匹配的结束引号
		end := strings.Index(rest[1:], "\"")
		if end < 0 {
			return rest[1:]
		}
		return rest[1 : end+1]
	}
	// 无引号的值：以空格为结束
	end := strings.IndexByte(rest, ' ')
	if end < 0 {
		return rest
	}
	return rest[:end]
}
