package core

import (
	"fmt"

	"clashgo/internal/config"

	"gopkg.in/yaml.v3"
)

// writeConfigYAML 将配置 map 序列化为 YAML 并原子写入文件
func writeConfigYAML(path string, cfg map[string]interface{}) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal runtime config: %w", err)
	}
	header := []byte("# ClashGo Runtime Config - Auto Generated\n\n")
	content := append(header, data...)
	// 原子写入：防止内核启动时读到半写的文件
	return config.WriteFileAtomic(path, content)
}
