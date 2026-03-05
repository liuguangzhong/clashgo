package proxy

// http.go — HTTP 代理处理
//
// 参照 mihomo/listener/http/proxy.go 实现。
// 支持：
//   HTTP CONNECT（HTTPS 隧道）→ handleHTTPS
//   普通 HTTP GET/POST 等  → 暂不支持转发（仅 CONNECT 隧道）

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// handleHTTP 处理 HTTP 代理请求（对应 mihomo/listener/http.HandleConn）
func handleHTTP(conn net.Conn, tunnel *Tunnel) {
	defer conn.Close()

	for {
		req, err := readHTTPRequest(conn)
		if err != nil {
			return
		}

		if req.Method == "CONNECT" {
			// HTTPS 隧道：回复 200 后转交给 Tunnel
			host, portStr, err := net.SplitHostPort(req.Host)
			if err != nil {
				// 没有端口号，默认 443
				host = req.Host
				portStr = "443"
			}
			port, _ := strconv.ParseUint(portStr, 10, 16)

			// 回复 200 Connection Established
			_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")

			metadata := &Metadata{
				Network: "tcp",
				Type:    "HTTPS",
				DstHost: host,
				DstPort: uint16(port),
			}
			tunnel.HandleTCPConn(conn, metadata)
			return // 连接已被 Tunnel 接管，退出循环
		}

		// 普通 HTTP 请求：解析目标地址并转发
		if err := handlePlainHTTP(conn, req, tunnel); err != nil {
			return
		}
	}
}

// HTTPRequest 简化的 HTTP 请求结构
type HTTPRequest struct {
	Method  string
	Host    string
	Path    string
	Version string
	Headers map[string]string
	Body    io.Reader
	raw     []string // 原始请求行（用于转发）
}

// readHTTPRequest 从连接中读取并解析 HTTP 请求行和头部
func readHTTPRequest(conn net.Conn) (*HTTPRequest, error) {
	req := &HTTPRequest{Headers: make(map[string]string)}

	// 读请求行（METHOD URL VERSION）
	requestLine, err := readLinefromConn(conn)
	if err != nil {
		return nil, err
	}
	if requestLine == "" {
		return nil, io.EOF
	}
	req.raw = append(req.raw, requestLine)

	parts := strings.SplitN(requestLine, " ", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid request line: %s", requestLine)
	}
	req.Method = parts[0]
	rawURL := parts[1]
	req.Version = parts[2]

	// 解析 URL
	if req.Method == "CONNECT" {
		req.Host = rawURL
	} else {
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, fmt.Errorf("parse URL: %w", err)
		}
		req.Host = u.Host
		req.Path = u.RequestURI()
	}

	// 读头部
	for {
		line, err := readLinefromConn(conn)
		if err != nil {
			return nil, err
		}
		req.raw = append(req.raw, line)
		if line == "" {
			break // 空行：头部结束
		}
		idx := strings.Index(line, ":")
		if idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			req.Headers[key] = val
			// 从 Host 头覆盖
			if strings.EqualFold(key, "Host") && req.Host == "" {
				req.Host = val
			}
		}
	}

	// 修正 Host（可能来自头部而非 URL）
	if h, ok := req.Headers["Host"]; ok && req.Host == "" {
		req.Host = h
	}

	return req, nil
}

// handlePlainHTTP 处理普通 HTTP GET/POST 等，将请求转发到目标服务器
func handlePlainHTTP(clientConn net.Conn, req *HTTPRequest, tunnel *Tunnel) error {
	host := req.Host
	if !strings.Contains(host, ":") {
		host += ":80"
	}
	hostOnly, portStr, _ := net.SplitHostPort(host)
	port, _ := strconv.ParseUint(portStr, 10, 16)

	metadata := &Metadata{
		Network: "tcp",
		Type:    "HTTP",
		DstHost: hostOnly,
		DstPort: uint16(port),
	}

	// 通过 Tunnel 选出出口并建立连接
	outbound, err := tunnel.pickOutbound(metadata)
	if err != nil {
		_, _ = fmt.Fprintf(clientConn, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
		return err
	}

	ctx := contextWithTimeout30s()
	serverConn, err := outbound.DialTCP(ctx, metadata)
	if err != nil {
		_, _ = fmt.Fprintf(clientConn, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
		return err
	}
	defer serverConn.Close()

	// 将原始请求转发给目标服务器
	for _, line := range req.raw {
		if _, err := fmt.Fprintf(serverConn, "%s\r\n", line); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintf(serverConn, "\r\n")

	// 双向 relay
	relay(clientConn, serverConn)
	return nil
}

// readLinefromConn 从连接逐字节读取一行（\r\n 或 \n 结尾）
func readLinefromConn(conn net.Conn) (string, error) {
	var line []byte
	buf := make([]byte, 1)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			if len(line) > 0 {
				return string(line), nil
			}
			return "", err
		}
		b := buf[0]
		if b == '\n' {
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			return string(line), nil
		}
		line = append(line, b)
	}
}
