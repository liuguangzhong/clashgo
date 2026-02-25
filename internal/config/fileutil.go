package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// WriteFileAtomic 原子写文件：先写临时文件，再重命名，防中断/并发损坏
// 所有持久化写入必须经过此函数
func WriteFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// MarshalYAML 将任意值序列化为 YAML 字节，强制所有长字符串加引号防折行
// yaml.v3 默认在 80 字符处折行，会破坏含 URL/emoji 的 YAML 结构
func MarshalYAML(v interface{}) ([]byte, error) {
	// 先编码为 yaml.Node 以便修改 Style
	node := &yaml.Node{}
	if err := node.Encode(v); err != nil {
		return nil, fmt.Errorf("yaml encode to node: %w", err)
	}
	// 强制长字符串和特殊字符串使用双引号风格（不折行）
	forceQuoteNodes(node)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	if err := enc.Encode(node); err != nil {
		return nil, fmt.Errorf("yaml encode: %w", err)
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}

// forceQuoteNodes 递归遍历 yaml.Node，对需要保护的标量强制双引号
func forceQuoteNodes(node *yaml.Node) {
	if node == nil {
		return
	}
	if node.Kind == yaml.ScalarNode {
		// 需要强制引号的条件：长字符串 / 含特殊字符
		if needsQuoting(node.Value, node.Tag) {
			node.Style = yaml.DoubleQuotedStyle
		}
	}
	for _, child := range node.Content {
		forceQuoteNodes(child)
	}
}

// needsQuoting 判断标量值是否需要强制双引号
func needsQuoting(value, tag string) bool {
	// 布尔/整数/浮点/null 类型由 yaml 本身处理，不需要引号
	switch tag {
	case "!!bool", "!!int", "!!float", "!!null":
		return false
	}
	// 超过 60 字符的字符串（URL、长路径）强制引号防折行
	if len(value) > 60 {
		return true
	}
	// 含 emoji 或非 ASCII 字符（节点名如 "🇯🇵日本-专线"）
	for _, r := range value {
		if r > 127 {
			return true
		}
	}
	return false
}

// WriteYAMLAtomic 是 MarshalYAML + WriteFileAtomic 的组合，带注释前缀
// Manager 内所有 YAML 持久化均调用此函数
func WriteYAMLAtomic(path string, v interface{}, comment string) error {
	data, err := MarshalYAML(v)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	content := append([]byte(comment+"\n\n"), data...)
	return WriteFileAtomic(path, content)
}
