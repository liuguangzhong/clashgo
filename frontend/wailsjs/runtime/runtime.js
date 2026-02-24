/**
 * @wailsio/runtime shim for build-time compilation
 *
 * 在 Wails 运行时，这个文件会被 Wails 引擎的内置实现替换。
 * 此文件仅用于 `vite build` 时的类型引用；
 * 实际运行时 Wails 会在 WebView 里注入真正的运行时。
 */

// Wails v2 通过 window.go 暴露绑定，通过 window.runtime 暴露运行时
// 前端代码通过 @wailsio/runtime 导入的函数在运行时由 Wails 注入

export const Call = {
    /**
     * 通过方法全名调用 Go 绑定
     * 在 Wails 运行时由 window.go.* 实现
     */
    ByName: function (methodName, ...args) {
        // 运行时由 Wails 引擎替换，此为 build-time stub
        if (typeof window !== 'undefined' && window['go']) {
            // 解析 "api.ConfigAPI.GetVergeConfig" → window.go.api.ConfigAPI.GetVergeConfig()
            const parts = methodName.split('.');
            let fn = window['go'];
            for (const part of parts) {
                if (fn == null) break;
                fn = fn[part];
            }
            if (typeof fn === 'function') {
                return fn(...args);
            }
        }
        return Promise.reject(new Error(`[wails] Method ${methodName} not available`));
    }
};

export const Events = {
    On: function (eventName, callback) {
        if (typeof window !== 'undefined' && window['runtime']) {
            return window['runtime'].EventsOn(eventName, callback);
        }
    },
    Off: function (eventName) {
        if (typeof window !== 'undefined' && window['runtime']) {
            return window['runtime'].EventsOff(eventName);
        }
    },
    Emit: function (eventName, data) {
        if (typeof window !== 'undefined' && window['runtime']) {
            return window['runtime'].EventsEmit(eventName, data);
        }
    },
};

export const Window = {
    Show: function () { window['runtime']?.WindowShow(); },
    Hide: function () { window['runtime']?.WindowHide(); },
    Minimize: function () { window['runtime']?.WindowMinimise(); },
    Maximize: function () { window['runtime']?.WindowMaximise(); },
};
