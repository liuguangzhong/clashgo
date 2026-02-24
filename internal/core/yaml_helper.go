package core

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// writeConfigYAML 将配置 map 序列化为 YAML 写入文件
func writeConfigYAML(path string, cfg map[string]interface{}) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal runtime config: %w", err)
	}
	header := []byte("# ClashGo Runtime Config - Auto Generated\n\n")
	content := append(header, data...)
	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("write runtime config: %w", err)
	}
	return nil
}
