package proxy

// ssh.go — SSH 出站代理（P3）
//
// 参照 mihomo/adapter/outbound/ssh.go 实现。
// 使用 golang.org/x/crypto/ssh（标准扩展库，非竞争对手代码）
// 通过 SSH 隧道代理 TCP 连接（等同于 ssh -D SOCKS5 的 CONNECT 方式）

import (
	"context"
	"fmt"
	"net"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// SSHOutbound SSH 出站代理
type SSHOutbound struct {
	name       string
	server     string // "host:22"
	user       string
	password   string
	privateKey string // PEM 格式私钥

	// 连接池（复用 SSH 连接）
	client   *gossh.Client
	clientCh chan *gossh.Client
}

func NewSSHOutbound(name, server, user, password, privateKey string) *SSHOutbound {
	ob := &SSHOutbound{
		name:       name,
		server:     server,
		user:       user,
		password:   password,
		privateKey: privateKey,
		clientCh:   make(chan *gossh.Client, 1),
	}
	return ob
}

func (s *SSHOutbound) Name() string { return s.name }

func (s *SSHOutbound) DialTCP(ctx context.Context, metadata *Metadata) (net.Conn, error) {
	// 获取或建立 SSH 连接
	sshClient, err := s.getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("SSH connect to %s: %w", s.server, err)
	}
	// 通过 SSH 隧道 CONNECT 目标地址（对应 ssh -W host:port）
	conn, err := sshClient.Dial("tcp", metadata.RemoteAddress())
	if err != nil {
		// 连接可能已失效，重建
		_ = sshClient.Close()
		s.client = nil
		xClient, xerr := s.newClient(ctx)
		if xerr != nil {
			return nil, fmt.Errorf("rebuild SSH client: %w", xerr)
		}
		conn, err = xClient.Dial("tcp", metadata.RemoteAddress())
		if err != nil {
			return nil, fmt.Errorf("SSH dial %s: %w", metadata.RemoteAddress(), err)
		}
	}
	return conn, nil
}

// getClient 获取可用的 SSH 客户端（懒初始化 + 缓存复用）
func (s *SSHOutbound) getClient(ctx context.Context) (*gossh.Client, error) {
	if s.client != nil {
		// 简单存活检测：发送空请求
		_, _, err := s.client.SendRequest("ClashGo-keepalive", true, nil)
		if err == nil {
			return s.client, nil
		}
		_ = s.client.Close()
		s.client = nil
	}
	client, err := s.newClient(ctx)
	if err != nil {
		return nil, err
	}
	s.client = client
	return client, nil
}

// newClient 建立新的 SSH 连接
func (s *SSHOutbound) newClient(ctx context.Context) (*gossh.Client, error) {
	cfg := &gossh.ClientConfig{
		User:            s.user,
		Timeout:         15 * time.Second,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec
	}

	// 认证方式
	if s.privateKey != "" {
		signer, err := gossh.ParsePrivateKey([]byte(s.privateKey))
		if err != nil {
			return nil, fmt.Errorf("parse SSH private key: %w", err)
		}
		cfg.Auth = append(cfg.Auth, gossh.PublicKeys(signer))
	}
	if s.password != "" {
		cfg.Auth = append(cfg.Auth, gossh.Password(s.password))
	}

	// 通过 ctx 感知的 TCP 拨号（对应 mihomo SSH 实现）
	dialer := &net.Dialer{}
	tcpConn, err := dialer.DialContext(ctx, "tcp", s.server)
	if err != nil {
		return nil, err
	}

	// SSH 握手
	c, chans, reqs, err := gossh.NewClientConn(tcpConn, s.server, cfg)
	if err != nil {
		_ = tcpConn.Close()
		return nil, err
	}
	return gossh.NewClient(c, chans, reqs), nil
}
