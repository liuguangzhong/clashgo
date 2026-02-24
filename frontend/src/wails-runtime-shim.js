/**
 * @wailsio/runtime shim — 不会被 Wails generate bindings 覆盖
 * 
 * Wails 生成的绑定文件使用:
 *   import {Call as $Call} from "@wailsio/runtime"
 *   $Call.ByName("api.ConfigAPI.GetVergeConfig", ...args)
 * 
 * 在运行时，Wails 引擎注入 window.go 对象树:
 *   window.go.api.ConfigAPI.GetVergeConfig()
 * 
 * 此文件桥接两者：把 Call.ByName 转发到 window.go.*
 */

// Re-export everything from the Wails-generated runtime
export * from "../wailsjs/runtime/runtime.js";

// ── Call 对象（绑定文件的核心依赖）──────────────────────────────────
export const Call = {
    /**
     * @param {string} name - "package.Struct.Method" e.g. "api.ConfigAPI.GetVergeConfig"
     * @param  {...any} args
     * @returns {Promise<any>}
     */
    ByName(name, ...args) {
        const parts = name.split(".");
        let target = window["go"];
        if (!target) {
            return Promise.reject(new Error(`[wails] window.go not available`));
        }
        for (const part of parts) {
            target = target[part];
            if (!target) {
                return Promise.reject(new Error(`[wails] method not found: ${name}`));
            }
        }
        if (typeof target !== "function") {
            return Promise.reject(new Error(`[wails] ${name} is not a function`));
        }
        return target(...args);
    },

    ByID(id, ...args) {
        if (window["_wails"]?.callByID) {
            return window["_wails"].callByID(id, ...args);
        }
        return Promise.reject(new Error(`[wails] ByID not supported in v2`));
    },
};
