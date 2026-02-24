package enhance

import (
	"encoding/json"
	"fmt"

	"clashgo/internal/utils"

	"github.com/dop251/goja"
	"go.uber.org/zap"
)

// ScriptLog 脚本执行日志条目
type ScriptLog = [2]string // [level, message]

// ExecuteScript 使用 goja（纯 Go V8 兼容引擎）执行用户 JS 脚本
// 对应原 enhance/script.rs 的 use_script()
//
// 脚本格式（与原版完全兼容）:
//
//	function main(config) {
//	    // 修改 config
//	    return config;
//	}
func ExecuteScript(script string, cfg map[string]interface{}, profileName string) (
	result map[string]interface{},
	logs []ScriptLog,
	err error,
) {
	logs = []ScriptLog{}

	vm := goja.New()

	// ─── 注入 console.log / console.error ────────────────────────────────────
	console := vm.NewObject()
	_ = console.Set("log", func(call goja.FunctionCall) goja.Value {
		msg := formatArgs(call.Arguments)
		logs = append(logs, ScriptLog{"info", msg})
		utils.Log().Debug("JS console.log", zap.String("msg", msg))
		return goja.Undefined()
	})
	_ = console.Set("error", func(call goja.FunctionCall) goja.Value {
		msg := formatArgs(call.Arguments)
		logs = append(logs, ScriptLog{"error", msg})
		utils.Log().Warn("JS console.error", zap.String("msg", msg))
		return goja.Undefined()
	})
	_ = console.Set("warn", func(call goja.FunctionCall) goja.Value {
		msg := formatArgs(call.Arguments)
		logs = append(logs, ScriptLog{"warn", msg})
		return goja.Undefined()
	})
	_ = vm.Set("console", console)

	// ─── 注入 __profile_name（脚本可以读取当前订阅名）────────────────────────
	_ = vm.Set("__profile_name", profileName)

	// ─── 执行脚本（定义 main 函数等）────────────────────────────────────────
	if _, err := vm.RunString(script); err != nil {
		return cfg, logs, fmt.Errorf("script init error: %w", err)
	}

	// ─── 获取 main 函数并调用 ────────────────────────────────────────────────
	mainFn, ok := goja.AssertFunction(vm.Get("main"))
	if !ok {
		return cfg, logs, fmt.Errorf("script must export a 'main' function")
	}

	// 将 Go map 转为 JS 对象（通过 JSON roundtrip 保证深拷贝）
	jsParam, err := mapToJSValue(vm, cfg)
	if err != nil {
		return cfg, logs, fmt.Errorf("param serialization: %w", err)
	}

	// 调用 main(config)
	jsResult, err := mainFn(goja.Undefined(), jsParam)
	if err != nil {
		logs = append(logs, ScriptLog{"error", err.Error()})
		return cfg, logs, fmt.Errorf("script execution: %w", err)
	}

	// ─── 将 JS 返回值转换回 Go map ────────────────────────────────────────────
	resultCfg, err := jsValueToMap(jsResult)
	if err != nil {
		return cfg, logs, fmt.Errorf("result deserialization: %w", err)
	}

	return resultCfg, logs, nil
}

// mapToJSValue 将 Go map 通过 JSON 序列化注入为 JS 对象
func mapToJSValue(vm *goja.Runtime, m map[string]interface{}) (goja.Value, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	val, err := vm.RunString("(" + string(data) + ")")
	if err != nil {
		return nil, err
	}
	return val, nil
}

// jsValueToMap 将 goja Value 转换为 Go map（通过 JSON roundtrip）
func jsValueToMap(val goja.Value) (map[string]interface{}, error) {
	if val == nil || goja.IsNull(val) || goja.IsUndefined(val) {
		return nil, fmt.Errorf("script returned null/undefined")
	}

	export := val.Export()
	data, err := json.Marshal(export)
	if err != nil {
		return nil, fmt.Errorf("marshal js result: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal js result: %w", err)
	}
	return result, nil
}

// formatArgs 将 goja 函数参数格式化为字符串
func formatArgs(args []goja.Value) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		if arg == nil {
			parts[i] = "null"
			continue
		}
		exported := arg.Export()
		if exported == nil {
			parts[i] = "null"
			continue
		}
		data, err := json.Marshal(exported)
		if err != nil {
			parts[i] = arg.String()
		} else {
			parts[i] = string(data)
		}
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " "
		}
		result += p
	}
	return result
}

// ValidateScript 验证脚本语法合法性（不执行 main，只检查解析）
func ValidateScript(script string) error {
	vm := goja.New()
	// 注入 console 对象（脚本可能引用，验证时不收集日志）
	_ = vm.Set("console", map[string]interface{}{
		"log": func() {}, "error": func() {}, "warn": func() {},
	})
	if _, err := vm.RunString(script); err != nil {
		return fmt.Errorf("syntax error: %w", err)
	}
	if _, ok := goja.AssertFunction(vm.Get("main")); !ok {
		return fmt.Errorf("script missing 'main' function")
	}
	return nil
}
