/**
 * Wails Shim for @tauri-apps/* and related packages
 * 
 * 这个文件像乐高拼图一样，为所有 @tauri-apps/* import 提供兼容接口。
 * 不同的包用同一个 tsconfig paths 别名映射到这里，所以所有导出都在同一个文件。
 * 
 * 覆盖范围：
 *   @tauri-apps/api/app, /core, /event, /window, /webviewWindow, /path
 *   @tauri-apps/plugin-http, -clipboard-manager, -dialog, -fs, -shell, -process, -updater
 */

// ─────────────────────────────────────────────────────────────────────────────
// @tauri-apps/api/app
// ─────────────────────────────────────────────────────────────────────────────

export async function getName(): Promise<string> {
    return "ClashGo";
}

export async function getVersion(): Promise<string> {
    return (window as typeof window & { __WAILS_VERSION__?: string }).__WAILS_VERSION__ ?? "1.0.0";
}

// ─────────────────────────────────────────────────────────────────────────────
// @tauri-apps/api/core
// ─────────────────────────────────────────────────────────────────────────────

/** invoke — 未迁移 cmds 的 fallback stub */
export async function invoke<T = unknown>(cmd: string, _args?: Record<string, unknown>): Promise<T> {
    console.warn(`[tauri-shim] invoke('${cmd}') not migrated to Wails binding`);
    return undefined as unknown as T;
}

/** convertFileSrc — 本地文件 URL 转换（Wails asset server 协议） */
export function convertFileSrc(filePath: string, _protocol = "asset"): string {
    return `/wails/file?path=${encodeURIComponent(filePath)}`;
}

// ─────────────────────────────────────────────────────────────────────────────
// @tauri-apps/api/event
// ─────────────────────────────────────────────────────────────────────────────

export type UnlistenFn = () => void;
export type EventCallback<T = unknown> = (event: { payload: T; event: string; id: number }) => void;

/** TauriEvent enum (Wails 中事件名不同，这里保持常量值兼容) */
export enum TauriEvent {
    WINDOW_RESIZED = "tauri://resize",
    WINDOW_MOVED = "tauri://move",
    WINDOW_CLOSE_REQUESTED = "tauri://close-requested",
    WINDOW_CREATED = "tauri://window-created",
    WINDOW_DESTROYED = "tauri://destroyed",
    WINDOW_FOCUS = "tauri://focus",
    WINDOW_BLUR = "tauri://blur",
    WINDOW_THEME_CHANGED = "tauri://theme-changed",
    WINDOW_SCALE_FACTOR_CHANGED = "tauri://scale-change",
    MENU = "tauri://menu",
    CHECK_UPDATE = "tauri://update",
    UPDATE_AVAILABLE = "tauri://update-available",
    INSTALL_UPDATE = "tauri://update-install",
    STATUS_UPDATE = "tauri://update-status",
    DOWNLOAD_PROGRESS = "tauri://update-download-progress",
    DRAG_DROP = "tauri://drag-drop",
    DRAG_ENTER = "tauri://drag-enter",
    DRAG_OVER = "tauri://drag-over",
    DRAG_LEAVE = "tauri://drag-leave",
}

const eventListeners = new Map<string, Set<EventCallback>>();

function getWailsRT() {
    return (window as typeof window & { runtime?: Record<string, (...args: unknown[]) => unknown> }).runtime;
}

export function listen<T>(eventName: string, handler: EventCallback<T>): Promise<UnlistenFn> {
    if (!eventListeners.has(eventName)) eventListeners.set(eventName, new Set());
    const set = eventListeners.get(eventName)!;
    const fn = handler as EventCallback;
    set.add(fn);

    const wrt = getWailsRT();
    if (wrt?.EventsOn) {
        wrt.EventsOn(eventName, (data: unknown) => {
            fn({ payload: data as T, event: eventName, id: 0 });
        });
    }

    return Promise.resolve(() => {
        set.delete(fn);
        if (getWailsRT()?.EventsOff) getWailsRT()!.EventsOff!(eventName);
    });
}

export function once<T>(eventName: string, handler: EventCallback<T>): Promise<UnlistenFn> {
    let unlisten: UnlistenFn | null = null;
    const wrapped: EventCallback<T> = (evt) => { handler(evt); unlisten?.(); };
    const p = listen(eventName, wrapped);
    p.then((fn) => { unlisten = fn; });
    return p;
}

export function emit(eventName: string, payload?: unknown): Promise<void> {
    eventListeners.get(eventName)?.forEach((fn) => fn({ payload, event: eventName, id: 0 }));
    getWailsRT()?.EventsEmit?.(eventName, payload);
    return Promise.resolve();
}

/** event namespace 兼容 (event.listen, event.once, event.emit) */
export const event = { listen, once, emit };

// ─────────────────────────────────────────────────────────────────────────────
// @tauri-apps/api/window + @tauri-apps/api/webviewWindow
// ─────────────────────────────────────────────────────────────────────────────

export type Theme = "light" | "dark";

class TauriWindowShim {
    close() { getWailsRT()?.WindowHide?.(); return Promise.resolve(); }
    minimize() { getWailsRT()?.WindowMinimise?.(); return Promise.resolve(); }
    maximize() { getWailsRT()?.WindowMaximise?.(); return Promise.resolve(); }
    unmaximize() { getWailsRT()?.WindowUnmaximise?.(); return Promise.resolve(); }
    toggleMaximize() {
        if (window.outerWidth >= screen.availWidth) {
            getWailsRT()?.WindowUnmaximise?.();
        } else {
            getWailsRT()?.WindowMaximise?.();
        }
        return Promise.resolve();
    }
    hide() { getWailsRT()?.WindowHide?.(); return Promise.resolve(); }
    show() { getWailsRT()?.WindowShow?.(); return Promise.resolve(); }
    isMaximized() {
        const isMax = window.outerWidth >= screen.availWidth && window.outerHeight >= screen.availHeight;
        return Promise.resolve(isMax);
    }
    isVisible() { return Promise.resolve(true); }
    isDecorated() { return Promise.resolve(true); }
    setDecorations(_v: boolean) { return Promise.resolve(); }
    isFullscreen() { return Promise.resolve(false); }
    setFullscreen(_v: boolean) { return Promise.resolve(); }
    onResized(cb: () => void): Promise<UnlistenFn> {
        window.addEventListener("resize", cb);
        return Promise.resolve(() => window.removeEventListener("resize", cb));
    }
    setMinimizable(_v: boolean) { return Promise.resolve(); }
    theme() {
        return Promise.resolve<Theme>(window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
    }
    onThemeChanged(_cb: (_event: { payload: Theme }) => void): Promise<UnlistenFn> {
        return Promise.resolve(() => { });
    }
    setTheme(_theme: Theme | null) { return Promise.resolve(); }
}

const _currentWindow = new TauriWindowShim();

export function getCurrentWindow() { return _currentWindow; }
export const getCurrentWebviewWindow = getCurrentWindow;
export { TauriWindowShim as WebviewWindow };

// ─────────────────────────────────────────────────────────────────────────────
// @tauri-apps/api/path
// ─────────────────────────────────────────────────────────────────────────────

export async function appDataDir(): Promise<string> { return ""; }
export async function appLocalDataDir(): Promise<string> { return ""; }
export async function join(...parts: string[]): Promise<string> { return parts.join("/"); }
export async function resolve(...parts: string[]): Promise<string> { return parts.join("/"); }
export async function dirname(p: string): Promise<string> { return p.split("/").slice(0, -1).join("/"); }
export async function basename(p: string): Promise<string> { return p.split("/").pop() ?? ""; }

// ─────────────────────────────────────────────────────────────────────────────
// @tauri-apps/plugin-http
// ─────────────────────────────────────────────────────────────────────────────

export type FetchOptions = RequestInit & { connectTimeout?: number };
export const fetch = globalThis.fetch.bind(globalThis);

// ─────────────────────────────────────────────────────────────────────────────
// @tauri-apps/plugin-clipboard-manager
// ─────────────────────────────────────────────────────────────────────────────

export async function writeText(text: string): Promise<void> {
    try {
        await navigator.clipboard.writeText(text);
    } catch {
        const el = document.createElement("textarea");
        el.value = text;
        el.style.cssText = "position:fixed;opacity:0";
        document.body.appendChild(el);
        el.select();
        document.execCommand("copy");
        document.body.removeChild(el);
    }
}

export async function readText(): Promise<string> {
    return navigator.clipboard.readText();
}

// ─────────────────────────────────────────────────────────────────────────────
// @tauri-apps/plugin-dialog
// ─────────────────────────────────────────────────────────────────────────────

export interface OpenDialogOptions {
    multiple?: boolean;
    directory?: boolean;
    filters?: Array<{ name: string; extensions: string[] }>;
    defaultPath?: string;
    title?: string;
}

export async function open(_options?: OpenDialogOptions | string): Promise<string | string[] | null> {
    console.warn("[tauri-shim] file dialog: use backend App.OpenFileDialog()");
    return null;
}

export async function save(_options?: OpenDialogOptions | string): Promise<string | null> {
    console.warn("[tauri-shim] save dialog: use backend App.SaveFileDialog()");
    return null;
}

// ─────────────────────────────────────────────────────────────────────────────
// @tauri-apps/plugin-fs
// ─────────────────────────────────────────────────────────────────────────────

export async function readTextFile(_path: string): Promise<string> {
    console.warn("[tauri-shim] readTextFile not available");
    return "";
}

export async function writeTextFile(_path: string, _content: string): Promise<void> {
    console.warn("[tauri-shim] writeTextFile not available");
}

export async function exists(_path: string): Promise<boolean> {
    return false;
}

// ─────────────────────────────────────────────────────────────────────────────
// @tauri-apps/plugin-shell
// ─────────────────────────────────────────────────────────────────────────────

export const Command = {
    create: () => ({
        execute: async () => ({ stdout: "", stderr: "", code: 0 }),
    }),
};

/** shell.open alias — opens URL externally */
export async function shellOpen(url: string): Promise<void> {
    window.open(url, "_blank");
}

// ─────────────────────────────────────────────────────────────────────────────
// @tauri-apps/plugin-process
// ─────────────────────────────────────────────────────────────────────────────

export type DownloadEvent = { event: "Started" | "Progress" | "Finished"; data: { contentLength?: number; chunkLength?: number } };

export async function exit(_code?: number): Promise<void> {
    getWailsRT()?.Quit?.();
}

export async function relaunch(): Promise<void> {
    console.warn("[tauri-shim] relaunch: use App.RestartApp()");
}

// ─────────────────────────────────────────────────────────────────────────────
// @tauri-apps/plugin-updater
// ─────────────────────────────────────────────────────────────────────────────

export type CheckOptions = {
    headers?: Record<string, string>;
    timeout?: number;
    proxy?: string;
    target?: string;
    allowDowngrades?: boolean;
};

export interface Update {
    version: string;
    available?: boolean;
    currentVersion?: string;
    date?: string;
    body?: string;
    rawJson?: Record<string, unknown>;
    close(): Promise<void>;
    downloadAndInstall(onProgress?: (event: DownloadEvent) => void): Promise<void>;
}

/** check — ClashGo 更新由 Go updater 推送事件，前端无需主动 check */
export async function check(_options?: CheckOptions): Promise<Update | null> {
    return null;
}
