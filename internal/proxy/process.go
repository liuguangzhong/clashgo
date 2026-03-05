package proxy

// process.go — 进程溯源（P3）
//
// 参照 mihomo/component/process 实现。
// 通过连接的源端口反查发起连接的进程名（PID + 进程名）。
//
// 平台实现：
//   Windows: netstat -ano + tasklist（或 WMI）
//   Linux:   /proc/net/tcp + /proc/$pid/comm
//   macOS:   lsof -P -i TCP

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ProcessInfo 进程信息
type ProcessInfo struct {
	PID  int    `json:"pid"`
	Name string `json:"name"` // 进程名
	Path string `json:"path"` // 可执行文件路径
}

// LookupProcess 通过 src addr 查找进程（对应 mihomo/component/process.FindProcessName）
func LookupProcess(network string, srcAddr net.Addr) (*ProcessInfo, error) {
	switch runtime.GOOS {
	case "windows":
		return lookupProcessWindows(network, srcAddr)
	case "linux":
		return lookupProcessLinux(network, srcAddr)
	case "darwin":
		return lookupProcessDarwin(network, srcAddr)
	default:
		return nil, fmt.Errorf("process lookup not supported on %s", runtime.GOOS)
	}
}

// ── Windows ───────────────────────────────────────────────────────────────────

// lookupProcessWindows 使用 netstat -ano 查找进程
func lookupProcessWindows(network string, srcAddr net.Addr) (*ProcessInfo, error) {
	tcpAddr, ok := srcAddr.(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("not a TCP addr")
	}

	// netstat -ano：列出所有连接和 PID
	out, err := exec.Command("netstat", "-ano", "-p", "TCP").Output()
	if err != nil {
		return nil, err
	}

	srcPort := strconv.Itoa(tcpAddr.Port)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		// 格式：TCP  LOCAL_ADDR  REMOTE_ADDR  STATE  PID
		localAddr := fields[1]
		if !strings.HasSuffix(localAddr, ":"+srcPort) {
			continue
		}
		pid, err := strconv.Atoi(fields[4])
		if err != nil {
			continue
		}
		name := lookupPIDNameWindows(pid)
		return &ProcessInfo{PID: pid, Name: name, Path: ""}, nil
	}
	return nil, fmt.Errorf("process not found for port %s", srcPort)
}

// lookupPIDNameWindows 通过 tasklist 查进程名
func lookupPIDNameWindows(pid int) string {
	out, err := exec.Command("tasklist", "/fi",
		fmt.Sprintf("PID eq %d", pid), "/fo", "csv", "/nh").Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return ""
	}
	// CSV 格式："name.exe","pid","session",...
	parts := strings.SplitN(line, ",", 2)
	if len(parts) == 0 {
		return ""
	}
	return strings.Trim(parts[0], "\"")
}

// ── Linux ─────────────────────────────────────────────────────────────────────

// lookupProcessLinux 通过 /proc/net/tcp 查找进程
func lookupProcessLinux(network string, srcAddr net.Addr) (*ProcessInfo, error) {
	tcpAddr, ok := srcAddr.(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("not a TCP addr")
	}

	// 将源端口转为 hex（/proc/net/tcp 格式）
	srcPort := fmt.Sprintf("%04X", tcpAddr.Port)

	// 读 /proc/net/tcp
	data, err := os.ReadFile("/proc/net/tcp")
	if err != nil {
		return nil, err
	}

	var inode string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		// local_address 格式：IP:PORT（hex，网络字节序）
		localAddr := fields[1]
		parts := strings.SplitN(localAddr, ":", 2)
		if len(parts) != 2 || parts[1] != srcPort {
			continue
		}
		inode = fields[9]
		break
	}

	if inode == "" {
		return nil, fmt.Errorf("inode not found for port %s", srcPort)
	}

	// 搜索 /proc/*/fd/ 找到使用该 inode 的进程
	return lookupInodeOwner(inode)
}

func lookupInodeOwner(inode string) (*ProcessInfo, error) {
	socketLink := "socket:[" + inode + "]"
	procs, _ := os.ReadDir("/proc")
	for _, p := range procs {
		if !p.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(p.Name())
		if err != nil {
			continue
		}
		fds, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", pid, fd.Name()))
			if err != nil || link != socketLink {
				continue
			}
			// 找到了，读进程名
			comm, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
			name := strings.TrimSpace(string(comm))
			exePath, _ := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
			return &ProcessInfo{PID: pid, Name: name, Path: exePath}, nil
		}
	}
	return nil, fmt.Errorf("process not found for inode %s", inode)
}

// ── macOS ─────────────────────────────────────────────────────────────────────

// lookupProcessDarwin 通过 lsof 查找进程
func lookupProcessDarwin(network string, srcAddr net.Addr) (*ProcessInfo, error) {
	tcpAddr, ok := srcAddr.(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("not a TCP addr")
	}
	filter := fmt.Sprintf("TCP@%s:%d", tcpAddr.IP.String(), tcpAddr.Port)
	out, err := exec.Command("lsof", "-P", "-i", filter, "-F", "pcn").Output()
	if err != nil {
		return nil, err
	}
	// lsof -F 格式：每行前缀字母标识字段
	// p = PID, c = command, n = name
	var pid int
	var name string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
		case 'c':
			name = line[1:]
		}
	}
	if pid == 0 {
		return nil, fmt.Errorf("process not found")
	}
	return &ProcessInfo{PID: pid, Name: name}, nil
}

// ── ProcessRule：基于进程名的路由规则 ────────────────────────────────────────
// 对应 mihomo/rules/common/process.go

// ProcessRule 进程名匹配规则
type ProcessRule struct {
	process string // 进程名（大小写不敏感）
	adapter string
}

func NewProcessRule(process, adapter string) *ProcessRule {
	return &ProcessRule{process: strings.ToLower(process), adapter: adapter}
}

func (r *ProcessRule) RuleType() string { return "PROCESS-NAME" }
func (r *ProcessRule) Adapter() string  { return r.adapter }
func (r *ProcessRule) Payload() string  { return r.process }

func (r *ProcessRule) Match(metadata *Metadata) bool {
	if metadata.Process == "" {
		return false
	}
	name := strings.ToLower(filepath.Base(metadata.Process))
	return name == r.process || name == r.process+".exe"
}
