// Package proxy 是 ClashGo 自实现的代理内核
//
// 完全参照 mihomo（github.com/MetaCubeX/mihomo）的架构，
// 从标准库写起，不依赖 mihomo 任何代码。
//
// 模块布局：
//   tunnel.go   — 核心路由 Tunnel（对应 mihomo/tunnel）
//   listener.go — Mixed HTTP/SOCKS5 监听器（对应 mihomo/listener/mixed）
//   http.go     — HTTP 代理处理（对应 mihomo/listener/http）
//   socks5.go   — SOCKS5 代理处理（对应 mihomo/transport/socks5）
//   rule.go     — 规则引擎（对应 mihomo/rules）
//   dns.go      — DNS 解析（对应 mihomo/dns）
//   outbound.go — 出口代理（对应 mihomo/adapter/outbound）
//   kernel.go   — 启动/停止入口（对应 mihomo/hub）

package proxy
