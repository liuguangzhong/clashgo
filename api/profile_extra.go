package api

import (
	"fmt"
	"os"

	"clashgo/internal/config"
	"clashgo/internal/enhance"
	"clashgo/internal/utils"

	"gopkg.in/yaml.v3"
)

// ─── Profile 保存+验证+回滚 ───────────────────────────────────────────────────

// SaveProfileFileWithValidation 保存配置文件，保存后进行语法验证；
// 如果验证失败则自动回滚到原始内容。
// 对应原: save_profile_file（含回滚逻辑）
func (a *ProfileAPI) SaveProfileFileWithValidation(uid string, content string) error {
	profiles := a.mgr.GetProfiles()

	var filePath string
	var profileType string
	for _, p := range profiles.Items {
		if p.UID != nil && *p.UID == uid && p.File != nil {
			filePath = utils.Dirs().ProfileFile(*p.File)
			if p.Type != nil {
				profileType = *p.Type
			}
			break
		}
	}
	if filePath == "" {
		return fmt.Errorf("profile not found: %s", uid)
	}

	// 备份原始内容
	original, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read original: %w", err)
	}

	// 写入新内容（原子写入）
	if err := config.WriteFileAtomic(filePath, []byte(content)); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	// 根据文件类型验证语法
	var validErr error
	switch {
	case isJSFile(filePath):
		validErr = enhance.ValidateScript(content)
	case profileType == "merge" || isYAMLFile(filePath):
		validErr = validateYAML([]byte(content))
	}

	if validErr != nil {
		// 验证失败，回滚原内容（原子写入）
		_ = config.WriteFileAtomic(filePath, original)
		return fmt.Errorf("validation failed (content restored): %w", validErr)
	}

	return nil
}

// ViewProfileInExplorer 在系统文件管理器中打开配置文件所在位置
// 对应原: view_profile
func (a *ProfileAPI) ViewProfileInExplorer(uid string) error {
	profiles := a.mgr.GetProfiles()
	for _, p := range profiles.Items {
		if p.UID != nil && *p.UID == uid && p.File != nil {
			return openFile(utils.Dirs().ProfileFile(*p.File))
		}
	}
	return fmt.Errorf("profile not found: %s", uid)
}

// GetNextUpdateTime 返回指定订阅下次自动更新的 Unix 时间戳
// 返回 0 表示无自动更新计划
// 对应原: get_next_update_time
func (a *ProfileAPI) GetNextUpdateTime(uid string) (int64, error) {
	profiles := a.mgr.GetProfiles()
	for _, p := range profiles.Items {
		if p.UID == nil || *p.UID != uid {
			continue
		}
		if p.UpdatedAt == nil || p.Interval == nil || *p.Interval == 0 {
			return 0, nil // 无自动更新计划
		}
		next := p.UpdatedAt.Unix() + int64(*p.Interval)
		return next, nil
	}
	return 0, fmt.Errorf("profile not found: %s", uid)
}

// NotifyValidationResult 记录脚本验证结果并推送前端事件
// 对应原: script_validate_notice
func (a *ProfileAPI) NotifyValidationResult(status, msg string) {
	utils.Log().Info("[validate] " + status + ": " + msg)
	emitEvent("script:validate", map[string]string{
		"status": status,
		"msg":    msg,
	})
}

// ─── 文件类型判断 ──────────────────────────────────────────────────────────────

func isYAMLFile(path string) bool {
	n := len(path)
	return (n > 5 && path[n-5:] == ".yaml") || (n > 4 && path[n-4:] == ".yml")
}

func isJSFile(path string) bool {
	n := len(path)
	return n > 3 && path[n-3:] == ".js"
}

// validateYAML 验证 YAML 字节语法
func validateYAML(data []byte) error {
	var check map[string]interface{}
	if err := yaml.Unmarshal(data, &check); err != nil {
		return fmt.Errorf("YAML syntax error: %w", err)
	}
	return nil
}
