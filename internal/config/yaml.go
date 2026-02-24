package config

import "gopkg.in/yaml.v3"

// UnmarshalYAML 公开给 api 层用于 YAML 语法验证
func UnmarshalYAML(data []byte, v interface{}) error {
	return yaml.Unmarshal(data, v)
}
