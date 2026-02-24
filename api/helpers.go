package api

import (
	"fmt"
	"io"
	"os"

	"clashgo/internal/config"
	"clashgo/internal/utils"

	"gopkg.in/yaml.v3"
)

// runtimeYAMLText 读取运行时 YAML 文件内容（给前端展示）
func runtimeYAMLText(mgr *config.Manager) (string, error) {
	path := utils.Dirs().RuntimeConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("runtime config not found: %w", err)
	}
	return string(data), nil
}

// marshalYAML 序列化为 YAML 字符串（公用）
func marshalYAML(v interface{}) (string, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// copyFile 跨路径复制文件（原子写，无第三方依赖）
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return out.Sync()
}
