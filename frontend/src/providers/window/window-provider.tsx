/**
 * WindowProvider — Wails 版本
 *
 * 原项目通过 @tauri-apps/api/window 控制窗口。
 * ClashGo 中，窗口操作通过 Wails runtime（window.runtime.*）实现。
 */
import React, { useCallback, useEffect, useMemo, useState } from "react";

import { WindowContext } from "./window-context";

// Wails runtime 在 WebView 加载时由引擎注入到 window.runtime
function getWailsRuntime() {
  return (window as typeof window & { runtime?: Record<string, (...args: unknown[]) => unknown> }).runtime;
}

export const WindowProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [decorated, setDecorated] = useState<boolean | null>(true);
  const [maximized, setMaximized] = useState<boolean | null>(false);

  const close = useCallback(async () => {
    await new Promise((resolve) => setTimeout(resolve, 20));
    getWailsRuntime()?.WindowHide?.();
  }, []);

  const minimize = useCallback(async () => {
    await new Promise((resolve) => setTimeout(resolve, 10));
    getWailsRuntime()?.WindowMinimise?.();
  }, []);

  const toggleMaximize = useCallback(async () => {
    if (maximized) {
      getWailsRuntime()?.WindowUnmaximise?.();
      setMaximized(false);
    } else {
      getWailsRuntime()?.WindowMaximise?.();
      setMaximized(true);
    }
  }, [maximized]);

  const toggleFullscreen = useCallback(async () => {
    getWailsRuntime()?.WindowFullscreen?.();
  }, []);

  const refreshDecorated = useCallback(async () => {
    return decorated ?? true;
  }, [decorated]);

  const toggleDecorations = useCallback(async () => {
    setDecorated((prev) => !prev);
  }, []);

  // 监听窗口 resize 事件（用浏览器原生事件替代 Tauri onResized）
  useEffect(() => {
    const handler = () => {
      // window.outerHeight 无法精确判断 maximize，用 screen 比对
      const isMax =
        window.outerWidth >= screen.availWidth &&
        window.outerHeight >= screen.availHeight;
      setMaximized(isMax);
    };
    window.addEventListener("resize", handler);
    return () => window.removeEventListener("resize", handler);
  }, []);

  // 提供一个与 Tauri currentWindow 接口兼容的 stub 对象
  const currentWindow = useMemo(() => ({
    close,
    minimize,
    maximize: () => { getWailsRuntime()?.WindowMaximise?.(); setMaximized(true); return Promise.resolve(); },
    unmaximize: () => { getWailsRuntime()?.WindowUnmaximise?.(); setMaximized(false); return Promise.resolve(); },
    toggleMaximize,
    hide: () => { getWailsRuntime()?.WindowHide?.(); return Promise.resolve(); },
    show: () => { getWailsRuntime()?.WindowShow?.(); return Promise.resolve(); },
    isMaximized: () => Promise.resolve(maximized ?? false),
    isVisible: () => Promise.resolve(true),
    isDecorated: () => Promise.resolve(decorated ?? true),
    setDecorations: (v: boolean) => { setDecorated(v); return Promise.resolve(); },
    isFullscreen: () => Promise.resolve(false),
    setFullscreen: (_v: boolean) => Promise.resolve(),
    onResized: (_cb: () => void) => Promise.resolve(() => { }),
    setMinimizable: (_v: boolean) => Promise.resolve(),
    theme: () => Promise.resolve<"light" | "dark">(
      window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"
    ),
    onThemeChanged: (_cb: (_e: { payload: "light" | "dark" }) => void) => Promise.resolve(() => { }),
    setTheme: (_t: "light" | "dark" | null) => Promise.resolve(),
  }), [close, minimize, toggleMaximize, maximized, decorated]);

  const contextValue = useMemo(
    () => ({
      decorated,
      maximized,
      toggleDecorations,
      refreshDecorated,
      minimize,
      close,
      toggleMaximize,
      toggleFullscreen,
      currentWindow,
    }),
    [
      decorated,
      maximized,
      toggleDecorations,
      refreshDecorated,
      minimize,
      close,
      toggleMaximize,
      toggleFullscreen,
      currentWindow,
    ],
  );

  return <WindowContext value={contextValue}>{children}</WindowContext>;
};
