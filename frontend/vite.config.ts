import path from "node:path";
import { fileURLToPath } from "node:url";

import react from "@vitejs/plugin-react-swc";
import { defineConfig } from "vite";
import svgr from "vite-plugin-svgr";

const CONFIG_DIR = path.dirname(fileURLToPath(import.meta.url));
const SRC_ROOT = path.resolve(CONFIG_DIR, "src");
const SHIM = path.resolve(SRC_ROOT, "tauri-shim.ts");
const MIHOMO_SHIM = path.resolve(SRC_ROOT, "tauri-plugin-mihomo-api.ts");

export default defineConfig({
    root: "src",
    server: {
        port: 34115, // Wails dev server default
        strictPort: true,
    },
    plugins: [svgr(), react()],
    build: {
        outDir: "../dist",
        emptyOutDir: true,
        minify: "terser",
        chunkSizeWarningLimit: 4500,
        reportCompressedSize: false,
        sourcemap: false,
        cssCodeSplit: true,
        cssMinify: true,
        terserOptions: {
            compress: {
                drop_console: false,
                drop_debugger: true,
                pure_funcs: ["console.debug", "console.trace"],
                dead_code: true,
                unused: true,
            },
            mangle: { safari10: true },
        },
        rollupOptions: {
            treeshake: {
                preset: "recommended",
                moduleSideEffects: (id) => !id.endsWith(".css"),
                tryCatchDeoptimization: false,
            },
            output: {
                compact: true,
                inlineDynamicImports: true, // 单文件输出，无代码分割
            },
        },
    },
    resolve: {
        alias: {
            // 应用源码别名
            "@": SRC_ROOT,
            "@root": CONFIG_DIR,

            // ── Wails 运行时 & 绑定 ─────────────────────────────────
            // 使用独立 shim（不会被 wails generate bindings 覆盖）
            "@wailsio/runtime": path.resolve(SRC_ROOT, "wails-runtime-shim.js"),

            // ── Tauri → Wails / 浏览器 shim ────────────────────
            // Mihomo API 插件 shim（fetch/WebSocket 直连）
            "tauri-plugin-mihomo-api": MIHOMO_SHIM,

            // Tauri 核心包 shim
            "@tauri-apps/api/core": SHIM,
            "@tauri-apps/api/app": SHIM,
            "@tauri-apps/api/event": SHIM,
            "@tauri-apps/api/window": SHIM,
            "@tauri-apps/api/webview": SHIM,
            "@tauri-apps/api/webviewWindow": SHIM,
            "@tauri-apps/api/path": SHIM,
            "@tauri-apps/api": SHIM,

            // Tauri 插件 shim
            "@tauri-apps/plugin-http": SHIM,
            "@tauri-apps/plugin-clipboard-manager": SHIM,
            "@tauri-apps/plugin-dialog": SHIM,
            "@tauri-apps/plugin-fs": SHIM,
            "@tauri-apps/plugin-shell": SHIM,
            "@tauri-apps/plugin-process": SHIM,
            "@tauri-apps/plugin-updater": SHIM,
        },
    },
    define: {
        OS_PLATFORM: `"${process.platform}"`,
        // Tauri 全局变量禁用
        "__TAURI_INTERNALS__": "undefined",
        "__TAURI__": "undefined",
    },
});
