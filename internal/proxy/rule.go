package proxy

// rule.go — 规则引擎
//
// 参照 mihomo/rules 实现。
// 支持大量规则类型：DOMAIN / DOMAIN-SUFFIX / DOMAIN-KEYWORD /
//   IP-CIDR / SRC-IP-CIDR / DST-PORT / SRC-PORT /
//   GEOIP / GEOSITE / PROCESS-NAME / MATCH

import (
	"fmt"
	"net"
	"strings"
)

// Rule 单条路由规则接口（对应 mihomo/constant.Rule）
type Rule interface {
	Match(metadata *Metadata) bool
	Adapter() string
	Payload() string
	RuleType() string
}

// ── DomainRule ──────────────────────────────────────────────────────────────

type DomainRule struct {
	domain  string
	adapter string
}

func NewDomainRule(domain, adapter string) *DomainRule {
	return &DomainRule{domain: strings.ToLower(domain), adapter: adapter}
}

func (r *DomainRule) Match(m *Metadata) bool { return strings.ToLower(m.DstHost) == r.domain }
func (r *DomainRule) Adapter() string        { return r.adapter }
func (r *DomainRule) Payload() string        { return r.domain }
func (r *DomainRule) RuleType() string       { return "DOMAIN" }

// ── DomainSuffixRule ─────────────────────────────────────────────────────────

type DomainSuffixRule struct {
	suffix  string
	adapter string
}

func NewDomainSuffixRule(suffix, adapter string) *DomainSuffixRule {
	return &DomainSuffixRule{
		suffix:  strings.ToLower(strings.TrimPrefix(suffix, ".")),
		adapter: adapter,
	}
}

func (r *DomainSuffixRule) Match(m *Metadata) bool {
	host := strings.ToLower(m.DstHost)
	return host == r.suffix || strings.HasSuffix(host, "."+r.suffix)
}
func (r *DomainSuffixRule) Adapter() string  { return r.adapter }
func (r *DomainSuffixRule) Payload() string  { return r.suffix }
func (r *DomainSuffixRule) RuleType() string { return "DOMAIN-SUFFIX" }

// ── DomainKeywordRule ────────────────────────────────────────────────────────

type DomainKeywordRule struct {
	keyword string
	adapter string
}

func NewDomainKeywordRule(keyword, adapter string) *DomainKeywordRule {
	return &DomainKeywordRule{keyword: strings.ToLower(keyword), adapter: adapter}
}

func (r *DomainKeywordRule) Match(m *Metadata) bool {
	return strings.Contains(strings.ToLower(m.DstHost), r.keyword)
}
func (r *DomainKeywordRule) Adapter() string  { return r.adapter }
func (r *DomainKeywordRule) Payload() string  { return r.keyword }
func (r *DomainKeywordRule) RuleType() string { return "DOMAIN-KEYWORD" }

// ── IPCIDRRule ───────────────────────────────────────────────────────────────

type IPCIDRRule struct {
	network *net.IPNet
	adapter string
}

func NewIPCIDRRule(cidr, adapter string) (*IPCIDRRule, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	return &IPCIDRRule{network: network, adapter: adapter}, nil
}

func (r *IPCIDRRule) Match(m *Metadata) bool {
	return len(m.DstIP) > 0 && r.network.Contains(m.DstIP)
}
func (r *IPCIDRRule) Adapter() string  { return r.adapter }
func (r *IPCIDRRule) Payload() string  { return r.network.String() }
func (r *IPCIDRRule) RuleType() string { return "IP-CIDR" }

// ── SrcIPCIDRRule ────────────────────────────────────────────────────────────

type SrcIPCIDRRule struct {
	network *net.IPNet
	adapter string
}

func NewSrcIPCIDRRule(cidr, adapter string) (*SrcIPCIDRRule, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	return &SrcIPCIDRRule{network: network, adapter: adapter}, nil
}

func (r *SrcIPCIDRRule) Match(m *Metadata) bool {
	return len(m.SrcIP) > 0 && r.network.Contains(m.SrcIP)
}
func (r *SrcIPCIDRRule) Adapter() string  { return r.adapter }
func (r *SrcIPCIDRRule) Payload() string  { return r.network.String() }
func (r *SrcIPCIDRRule) RuleType() string { return "SRC-IP-CIDR" }

// ── PortRule ─────────────────────────────────────────────────────────────────

type PortRule struct {
	ruleType string // "DST-PORT" | "SRC-PORT"
	port     uint16
	adapter  string
}

func NewPortRule(ruleType, portStr, adapter string) *PortRule {
	var port uint64
	_, _ = fmt.Sscanf(portStr, "%d", &port)
	return &PortRule{ruleType: ruleType, port: uint16(port), adapter: adapter}
}

func (r *PortRule) Match(m *Metadata) bool {
	if r.ruleType == "DST-PORT" {
		return m.DstPort == r.port
	}
	return m.SrcPort == r.port
}
func (r *PortRule) Adapter() string  { return r.adapter }
func (r *PortRule) Payload() string  { return fmt.Sprintf("%d", r.port) }
func (r *PortRule) RuleType() string { return r.ruleType }

// ── MatchRule：兜底匹配 ───────────────────────────────────────────────────────

type MatchRule struct {
	adapter string
}

func NewMatchRule(adapter string) *MatchRule { return &MatchRule{adapter: adapter} }

func (r *MatchRule) Match(_ *Metadata) bool { return true }
func (r *MatchRule) Adapter() string        { return r.adapter }
func (r *MatchRule) Payload() string        { return "" }
func (r *MatchRule) RuleType() string       { return "MATCH" }

// ── ParseRule：从字符串解析规则 ───────────────────────────────────────────────
// 格式：TYPE,VALUE,ADAPTER  或  MATCH,ADAPTER
func ParseRule(ruleStr string, _ map[string]bool) (Rule, error) {
	parts := strings.Split(ruleStr, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) < 2 {
		return nil, nil
	}

	ruleType := strings.ToUpper(parts[0])
	switch ruleType {

	case "MATCH", "FINAL":
		return NewMatchRule(parts[1]), nil

	case "DOMAIN":
		if len(parts) < 3 {
			return nil, nil
		}
		return NewDomainRule(parts[1], parts[2]), nil

	case "DOMAIN-SUFFIX":
		if len(parts) < 3 {
			return nil, nil
		}
		return NewDomainSuffixRule(parts[1], parts[2]), nil

	case "DOMAIN-KEYWORD":
		if len(parts) < 3 {
			return nil, nil
		}
		return NewDomainKeywordRule(parts[1], parts[2]), nil

	case "IP-CIDR", "IP-CIDR6":
		if len(parts) < 3 {
			return nil, nil
		}
		return NewIPCIDRRule(parts[1], parts[2])

	case "SRC-IP-CIDR":
		if len(parts) < 3 {
			return nil, nil
		}
		return NewSrcIPCIDRRule(parts[1], parts[2])

	case "DST-PORT":
		if len(parts) < 3 {
			return nil, nil
		}
		return NewPortRule("DST-PORT", parts[1], parts[2]), nil

	case "SRC-PORT":
		if len(parts) < 3 {
			return nil, nil
		}
		return NewPortRule("SRC-PORT", parts[1], parts[2]), nil

	case "GEOIP":
		if len(parts) < 3 {
			return nil, nil
		}
		noResolve := len(parts) >= 4 && strings.EqualFold(parts[3], "no-resolve")
		return NewGeoIPRule(parts[1], parts[2], noResolve), nil

	case "GEOSITE":
		if len(parts) < 3 {
			return nil, nil
		}
		return NewGeoSiteRule(parts[1], parts[2]), nil

	case "PROCESS-NAME", "PROCESS-PATH":
		if len(parts) < 3 {
			return nil, nil
		}
		return NewProcessRule(parts[1], parts[2]), nil

	default:
		// 未知规则类型跳过（如 AND/OR/NOT 复合规则）
		return nil, nil
	}
}
