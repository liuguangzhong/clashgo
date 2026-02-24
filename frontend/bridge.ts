/**
 * ClashGo Frontend Bridge
 * 
 * 这个文件是从 Tauri IPC 到 Wails Binding 的适配层。
 * 原项目使用:
 *   import { invoke } from '@tauri-apps/api/core';
 *   await invoke('get_verge_config');
 * 
 * ClashGo 替换为:
 *   import { GetVergeConfig } from './bridge';
 *   await GetVergeConfig();
 * 
 * Wails 在构建时自动生成 wailsjs/go/ 目录下的 TypeScript 绑定。
 * 本文件统一重导出，使业务代码改动最小。
 */

// ─── 自动生成的 Wails 绑定（构建后存在于 wailsjs/go/）────────────────────────
// import { GetVergeConfig, PatchVergeConfig, ... } from '../wailsjs/go/api/ConfigAPI';
// import { GetProfiles, ImportProfile, ... } from '../wailsjs/go/api/ProfileAPI';
// import { GetSystemInfo, ... } from '../wailsjs/go/api/SystemAPI';
// import { EventsOn, EventsOff, EventsEmit } from '../wailsjs/runtime/runtime';

// ─── 类型定义（与后端 Go struct 对齐）────────────────────────────────────────

export interface IVerge {
    app_log_level?: string;
    language?: string;
    theme_mode?: string;
    enable_system_proxy?: boolean;
    enable_tun_mode?: boolean;
    clash_core?: string;
    enable_auto_launch?: boolean;
    enable_silent_start?: boolean;
    proxy_host?: string;
    system_proxy_bypass?: string;
    use_default_bypass?: boolean;
    enable_proxy_guard?: boolean;
    proxy_guard_duration?: number;
    verge_mixed_port?: number;
    verge_socks_port?: number;
    verge_socks_enabled?: boolean;
    verge_port?: number;
    verge_http_enabled?: boolean;
    verge_redir_port?: number;
    verge_redir_enabled?: boolean;
    verge_tproxy_port?: number;
    verge_tproxy_enabled?: boolean;
    enable_auto_backup_schedule?: boolean;
    webdav_url?: string;
    webdav_username?: string;
    webdav_password?: string;
    hotkeys?: string[];
    enable_global_hotkey?: boolean;
    auto_close_connection?: boolean;
    auto_check_update?: boolean;
    default_latency_test?: string;
    enable_builtin_enhanced?: boolean;
    auto_log_clean?: number;
    theme_setting?: IVergeTheme;
    tray_proxy_groups_display_mode?: string;
    proxy_layout_column?: number;
    home_cards?: unknown;
    [key: string]: unknown;
}

export interface IVergeTheme {
    primary_color?: string;
    secondary_color?: string;
    primary_text?: string;
    secondary_text?: string;
    info_color?: string;
    error_color?: string;
    warning_color?: string;
    success_color?: string;
    font_family?: string;
    css_injection?: string;
}

export interface IProfile {
    uid?: string;
    type?: 'remote' | 'local' | 'merge' | 'script';
    name?: string;
    desc?: string;
    file?: string;
    url?: string;
    selected?: { name: string; now: string }[];
    extra?: ProfileExtra;
    updated_at?: string;
    interval?: number;
    option?: ProfileOption;
}

export interface ProfileExtra {
    upload: number;
    download: number;
    total: number;
    expire: number;
}

export interface ProfileOption {
    user_agent?: string;
    with_proxy?: boolean;
    self_proxy?: boolean;
    update_interval?: number;
}

export interface IProfiles {
    current?: string;
    items?: IProfile[];
}

export interface ClashInfo {
    mixed_port: number;
    socks_port: number;
    port: number;
    server: string;
    secret?: string;
}

// ─── Tauri → Wails IPC 映射表 ─────────────────────────────────────────────────
// 下面是原项目 invoke() 调用与 ClashGo Wails 绑定的完整对照：
//
// 原来 (Tauri):                          现在 (Wails):
// invoke('get_verge_config')          → ConfigAPI.GetVergeConfig()
// invoke('patch_verge_config', patch) → ConfigAPI.PatchVergeConfig(patch)
// invoke('get_clash_info')            → ConfigAPI.GetClashInfo()
// invoke('patch_clash_config', patch) → ConfigAPI.PatchClashConfig(patch)
// invoke('patch_clash_mode', mode)    → ConfigAPI.PatchClashMode(mode)
// invoke('get_runtime_config')        → ConfigAPI.GetRuntimeConfig()  
// invoke('get_runtime_yaml')          → ConfigAPI.GetRuntimeYAML()
// invoke('get_runtime_logs')          → ConfigAPI.GetRuntimeLogs()
// invoke('get_profiles')              → ProfileAPI.GetProfiles()
// invoke('import_profile', opts)      → ProfileAPI.ImportProfile(opts)
// invoke('create_profile', item)      → ProfileAPI.CreateProfile(item)
// invoke('patch_profile', uid, patch) → ProfileAPI.PatchProfile(uid, patch)
// invoke('delete_profile', uid)       → ProfileAPI.DeleteProfile(uid)
// invoke('enhance_profiles')          → ProfileAPI.EnhanceProfiles()
// invoke('patch_profiles_config',uid) → ProfileAPI.PatchProfilesConfig(uid)
// invoke('update_profile', uid, opts) → ProfileAPI.UpdateProfile(uid)
// invoke('read_profile_file', uid)    → ProfileAPI.ReadProfileFile(uid)
// invoke('save_profile_file', uid, c) → ProfileAPI.SaveProfileFile(uid, c)
// invoke('get_proxies')               → ProxyAPI.GetProxies()
// invoke('get_rules')                 → ProxyAPI.GetRules()
// invoke('get_providers_proxies')     → ProxyAPI.GetProviders()
// invoke('select_proxy', opts)        → ProxyAPI.SelectProxy(group, proxy)
// invoke('test_proxy_delay', opts)    → ProxyAPI.TestProxyDelay(name, url, t)
// invoke('get_sys_proxy')             → SystemAPI.GetSysProxy()
// invoke('get_network_interfaces')    → SystemAPI.GetNetworkInterfaces()
// invoke('get_system_info')           → SystemAPI.GetSystemInfo()
// invoke('check_port_available', p)   → SystemAPI.CheckPortAvailable(p)
// invoke('open_app_dir')              → SystemAPI.OpenAppDir()
// invoke('open_logs_dir')             → SystemAPI.OpenLogsDir()
// invoke('create_backup')             → BackupAPI.CreateLocalBackup()
// invoke('upload_webdav_backup')      → BackupAPI.UploadToWebDAV()
// invoke('list_webdav_backup')        → BackupAPI.ListWebDAVFiles()

// ─── 事件名对照（EventsOn 保持不变）─────────────────────────────────────────
// Tauri event:        → Wails EventsOn:
// 'verge://updated'   → 'verge:updated'
// 'clash://config'    → 'clash:updated' 
// 'clash://log'       → 'clash:log'   (payload: { type, payload })
// 'app://ready'       → 'app:ready'
// 'core://updated'    → 'core:updated'

export const WAILS_EVENTS = {
    VERGE_UPDATED: 'verge:updated',
    CLASH_UPDATED: 'clash:updated',
    CLASH_LOG: 'clash:log',
    APP_READY: 'app:ready',
    CORE_UPDATED: 'core:updated',
} as const;

// ─── 统一 API 调用封装（减少业务代码改动量）─────────────────────────────────

/**
 * 调用 ClashGo 后端 API（统一错误处理）
 * 替代原来的 invoke() 函数
 */
export async function callAPI<T>(fn: () => Promise<T>): Promise<T> {
    try {
        return await fn();
    } catch (err) {
        const message = err instanceof Error
            ? err.message
            : typeof err === 'string'
                ? err
                : 'Unknown error';
        throw new Error(`API Error: ${message}`);
    }
}
