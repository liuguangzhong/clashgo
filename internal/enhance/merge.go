package enhance

import (
	"fmt"
	"os"

	"clashgo/internal/config"
	"clashgo/internal/utils"

	"gopkg.in/yaml.v3"
)

// ChainType 定义 chain item 类型
type ChainType int

const (
	ChainTypeMerge   ChainType = iota // YAML Merge 策略
	ChainTypeScript                    // JavaScript 脚本
	ChainTypeRules                     // Rules 注入
	ChainTypeProxies                   // Proxies 注入
	ChainTypeGroups                    // Groups 注入
)

// ChainItem 代表一个处理节点
type ChainItem struct {
	UID  string
	Type ChainType
	Data interface{} // Merge: map, Script: string, Rules/Proxies/Groups: SeqMap
}

// SeqMap 有序 append/prepend 操作（对应原 seq.rs）
type SeqMap struct {
	Prepend []interface{} `yaml:"prepend"`
	Append  []interface{} `yaml:"append"`
}

// loadChainItem 根据 IProfile 类型加载 ChainItem 内容
func loadChainItem(p *config.IProfile, dirs *utils.AppDirs) (*ChainItem, error) {
	if p.UID == nil || p.Type == nil || p.File == nil {
		return nil, fmt.Errorf("profile missing required fields")
	}

	uid := *p.UID
	filePath := dirs.ProfileFile(*p.File)

	switch *p.Type {
	case "merge":
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read merge file: %w", err)
		}
		var m map[string]interface{}
		if err := yaml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parse merge yaml: %w", err)
		}
		if m == nil {
			m = make(map[string]interface{})
		}
		return &ChainItem{UID: uid, Type: ChainTypeMerge, Data: m}, nil

	case "script":
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read script file: %w", err)
		}
		return &ChainItem{UID: uid, Type: ChainTypeScript, Data: string(data)}, nil

	default:
		return nil, fmt.Errorf("unknown chain type: %s", *p.Type)
	}
}

// ApplyMerge 将 merge map 深度合并到 base config
// 对应原 enhance/merge.rs 的 use_merge()
func ApplyMerge(merge, base map[string]interface{}) map[string]interface{} {
	if base == nil {
		base = make(map[string]interface{})
	}
	for key, mergeVal := range merge {
		if baseVal, exists := base[key]; exists {
			// 如果两者都是 map，递归合并
			if baseMap, ok := toStringMap(baseVal); ok {
				if mergeMap, ok := toStringMap(mergeVal); ok {
					base[key] = ApplyMerge(mergeMap, baseMap)
					continue
				}
			}
			// 如果两者都是 slice，拼接
			if baseSlice, ok := toSlice(baseVal); ok {
				if mergeSlice, ok := toSlice(mergeVal); ok {
					base[key] = append(mergeSlice, baseSlice...)
					continue
				}
			}
		}
		base[key] = mergeVal
	}
	return base
}

// InjectSeq 向配置的指定字段注入 prepend/append 条目
// 对应原 enhance/seq.rs 的 use_seq()
func InjectSeq(cfg map[string]interface{}, seq SeqMap, field string) map[string]interface{} {
	existing := []interface{}{}
	if v, ok := cfg[field]; ok {
		if slice, ok := toSlice(v); ok {
			existing = slice
		}
	}

	result := make([]interface{}, 0, len(seq.Prepend)+len(existing)+len(seq.Append))
	result = append(result, seq.Prepend...)
	result = append(result, existing...)
	result = append(result, seq.Append...)

	cfg[field] = result
	return cfg
}

// MergeClashBaseOpts 控制哪些端口/特性被启用
type MergeClashBaseOpts struct {
	SocksEnabled        bool
	HTTPEnabled         bool
	RedirEnabled        bool
	TProxyEnabled       bool
	ExternalCtrlEnabled bool
}

// MergeClashBase 将 Clash 基础配置合并到增强后的 Profile 配置
// 对应原 merge_default_config()
func MergeClashBase(cfg map[string]interface{}, base config.IClashBase, opts MergeClashBaseOpts) map[string]interface{} {
	for key, val := range base {
		switch key {
		case "tun":
			// TUN 配置：深度合并
			existing := map[string]interface{}{}
			if v, ok := cfg[key]; ok {
				if m, ok := toStringMap(v); ok {
					existing = m
				}
			}
			if patchMap, ok := toStringMap(val); ok {
				for k, v := range patchMap {
					existing[k] = v
				}
			}
			cfg[key] = existing

		case "socks-port":
			if !opts.SocksEnabled {
				delete(cfg, "socks-port")
				continue
			}
			cfg[key] = val

		case "port":
			if !opts.HTTPEnabled {
				delete(cfg, "port")
				continue
			}
			cfg[key] = val

		case "redir-port":
			if !opts.RedirEnabled {
				delete(cfg, "redir-port")
				continue
			}
			cfg[key] = val

		case "tproxy-port":
			if !opts.TProxyEnabled {
				delete(cfg, "tproxy-port")
				continue
			}
			cfg[key] = val

		case "external-controller":
			if opts.ExternalCtrlEnabled {
				cfg[key] = val
			} else {
				cfg[key] = "" // 禁用时设为空字符串
			}

		case "external-controller-unix", "external-controller-pipe":
			cfg[key] = val

		default:
			cfg[key] = val
		}
	}
	return cfg
}

// CleanupProxyGroups 清理 proxy-groups 中引用了不存在代理/提供者的条目
// 对应原 cleanup_proxy_groups()
func CleanupProxyGroups(cfg map[string]interface{}) map[string]interface{} {
	builtinPolicies := map[string]bool{"DIRECT": true, "REJECT": true, "REJECT-DROP": true, "PASS": true}

	// 收集所有合法代理名
	proxyNames := map[string]bool{}
	if proxies, ok := cfg["proxies"]; ok {
		if slice, ok := toSlice(proxies); ok {
			for _, p := range slice {
				if m, ok := toStringMap(p); ok {
					if name, ok := m["name"].(string); ok {
						proxyNames[name] = true
					}
				} else if name, ok := p.(string); ok {
					proxyNames[name] = true
				}
			}
		}
	}

	// 收集所有提供者名
	providerNames := map[string]bool{}
	if providers, ok := cfg["proxy-providers"]; ok {
		if m, ok := toStringMap(providers); ok {
			for name := range m {
				providerNames[name] = true
			}
		}
	}

	// 收集所有组名
	groupNames := map[string]bool{}
	if groups, ok := cfg["proxy-groups"]; ok {
		if slice, ok := toSlice(groups); ok {
			for _, g := range slice {
				if m, ok := toStringMap(g); ok {
					if name, ok := m["name"].(string); ok {
						groupNames[name] = true
					}
				}
			}
		}
	}

	// 合并允许引用的名称
	allowed := map[string]bool{}
	for k := range proxyNames {
		allowed[k] = true
	}
	for k := range groupNames {
		allowed[k] = true
	}
	for k := range providerNames {
		allowed[k] = true
	}
	for k := range builtinPolicies {
		allowed[k] = true
	}

	// 清理 proxy-groups 中的无效引用
	if groups, ok := cfg["proxy-groups"]; ok {
		if slice, ok := toSlice(groups); ok {
			for _, g := range slice {
				if gMap, ok := toStringMap(g); ok {
					// 清理 use（proxy-providers）
					hasValidProvider := false
					if uses, ok := gMap["use"]; ok {
						if useSlice, ok := toSlice(uses); ok {
							filtered := useSlice[:0]
							for _, u := range useSlice {
								if name, ok := u.(string); ok && providerNames[name] {
									filtered = append(filtered, u)
									hasValidProvider = true
								}
							}
							gMap["use"] = filtered
						}
					}

					// 清理 proxies（无效代理引用）
					if proxies, ok := gMap["proxies"]; ok {
						if pSlice, ok := toSlice(proxies); ok {
							filtered := pSlice[:0]
							for _, p := range pSlice {
								if name, ok := p.(string); ok {
									if allowed[name] || hasValidProvider {
										filtered = append(filtered, p)
									}
								} else {
									filtered = append(filtered, p)
								}
							}
							gMap["proxies"] = filtered
						}
					}
				}
			}
		}
	}

	return cfg
}

// SortFields 按照 Mihomo 配置约定排序顶级字段
// 对应原 use_sort()
func SortFields(cfg map[string]interface{}) map[string]interface{} {
	// Go map 无序，但 yaml.v3 序列化时可以控制顺序
	// 这里不做额外排序，yaml.Marshal 会按字母序输出
	// 如需强制顺序，使用 yaml.Node 构建有序 document
	return cfg
}

// ApplyBuiltinScripts 执行内置的增强脚本（对应原 builtin/ JS 文件）
// 内置脚本主要处理：统一延迟、移除弃用字段、修正格式等
func ApplyBuiltinScripts(cfg map[string]interface{}, coreName string) map[string]interface{} {
	// 内置脚本1：统一延迟（unified-delay 设置）
	cfg = ensureUnifiedDelay(cfg)

	// 内置脚本2：移除 subscribtion-userinfo 顶级字段（mihomo 不识别）
	delete(cfg, "subscribtion-userinfo")
	delete(cfg, "subscription-userinfo")

	return cfg
}

// ensureUnifiedDelay 确保 unified-delay 设置正确
func ensureUnifiedDelay(cfg map[string]interface{}) map[string]interface{} {
	if _, ok := cfg["unified-delay"]; !ok {
		cfg["unified-delay"] = true
	}
	return cfg
}

// ApplyTUN 注入 TUN 模式配置
// 对应原 enhance/tun.rs 的 use_tun()
func ApplyTUN(cfg map[string]interface{}, enabled bool) map[string]interface{} {
	tun := map[string]interface{}{}
	if existing, ok := cfg["tun"]; ok {
		if m, ok := toStringMap(existing); ok {
			tun = m
		}
	}
	tun["enable"] = enabled
	cfg["tun"] = tun
	return cfg
}

// ─── 工具函数 ─────────────────────────────────────────────────────────────────

func toStringMap(v interface{}) (map[string]interface{}, bool) {
	switch m := v.(type) {
	case map[string]interface{}:
		return m, true
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(m))
		for k, v := range m {
			out[fmt.Sprintf("%v", k)] = v
		}
		return out, true
	}
	return nil, false
}

func toSlice(v interface{}) ([]interface{}, bool) {
	if s, ok := v.([]interface{}); ok {
		return s, true
	}
	return nil, false
}
