/**
 * cmds.ts — Wails 版本
 *
 * 原项目使用 `invoke('cmd_name', args)` 调用 Tauri IPC。
 * ClashGo 中，Wails 在构建时自动生成 TypeScript 绑定到
 * `../wailsjs/go/<Package>/<MethodName>` 路径下。
 *
 * 本文件将所有 cmds 函数映射到对应的 Wails 绑定，
 * 保持与原项目完全相同的函数签名，上层代码无需改动。
 */

import dayjs from "dayjs";

import { showNotice } from "@/services/notice-service";
import { debugLog } from "@/utils/debug";

// ── Wails 自动生成的绑定（wails build/dev 后出现） ─────────────────────────────
// 声明为 any 类型是为了在未构建时仍可编译；构建后这些路径会被真实文件覆盖
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let _ConfigAPI: any = {};
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let _ProfileAPI: any = {};
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let _ProxyAPI: any = {};
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let _SystemAPI: any = {};
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let _BackupAPI: any = {};
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let _ServiceAPI: any = {};
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let _App: any = {};
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let _MediaUnlockAPI: any = {};

// 动态加载 Wails 绑定（构建后存在，开发模式下 Wails dev 自动注入）
async function loadBindings() {
  try {
    // Wails 构建时生成这些模块（路径相对于 frontend/ 根）
    const [cfg, prof, prx, sys, bak, svc, app, media] = await Promise.all([
      import("../../wailsjs/go/api/ConfigAPI"),
      import("../../wailsjs/go/api/ProfileAPI"),
      import("../../wailsjs/go/api/ProxyAPI"),
      import("../../wailsjs/go/api/SystemAPI"),
      import("../../wailsjs/go/api/BackupAPI"),
      import("../../wailsjs/go/api/ServiceAPI"),
      import("../../wailsjs/go/main/App"),
      import("../../wailsjs/go/api/MediaUnlockAPI"),
    ]);
    _ConfigAPI = cfg;
    _ProfileAPI = prof;
    _ProxyAPI = prx;
    _SystemAPI = sys;
    _BackupAPI = bak;
    _ServiceAPI = svc;
    _App = app;
    _MediaUnlockAPI = media;
  } catch {
    // 未构建时（纯前端开发模式）忽略，使用 stub
    console.warn("[ClashGo] Wails bindings not found – running in stub mode");
  }
}

// 立即加载
loadBindings();

// ── 代理环境变量 ──────────────────────────────────────────────────────────────

export async function copyClashEnv() {
  return _ConfigAPI.CopyClashEnv?.();
}

// ── 订阅 Profiles ─────────────────────────────────────────────────────────────

export async function getProfiles() {
  return _ProfileAPI.GetProfiles?.();
}

export async function enhanceProfiles() {
  return _ProfileAPI.EnhanceProfiles?.();
}

export async function patchProfilesConfig(profiles: IProfilesConfig) {
  // Wails: ProfileAPI.PatchProfilesConfig(uid) 只接受 uid
  const uid = profiles.current;
  if (!uid) return;
  return _ProfileAPI.PatchProfilesConfig?.(uid);
}

export async function createProfile(
  item: Partial<IProfileItem>,
  fileData?: string | null,
) {
  // Go 端 CreateProfile 接收单个 CreateProfileRequest 结构体
  return _ProfileAPI.CreateProfile?.({
    type: item.type ?? "local",
    name: item.name ?? "",
    content: fileData ?? "",
  });
}

export async function viewProfile(index: string) {
  return _ProfileAPI.ViewProfileInExplorer?.(index);
}

export async function readProfileFile(index: string) {
  return _ProfileAPI.ReadProfileFile?.(index);
}

export async function saveProfileFile(index: string, fileData: string) {
  return _ProfileAPI.SaveProfileFileWithValidation?.(index, fileData);
}

export async function importProfile(url: string, option?: IProfileOption) {
  // Go 端 ImportProfile 接收单个 ImportProfileRequest 结构体
  return _ProfileAPI.ImportProfile?.({ url, option: option ?? { with_proxy: true } });
}

export async function reorderProfile(activeId: string, overId: string) {
  return _ProfileAPI.ReorderProfile?.(activeId, overId);
}

export async function updateProfile(index: string, option?: IProfileOption) {
  return _ProfileAPI.UpdateProfile?.(index);
}

export async function deleteProfile(index: string) {
  return _ProfileAPI.DeleteProfile?.(index);
}

export async function patchProfile(
  index: string,
  profile: Partial<IProfileItem>,
) {
  return _ProfileAPI.PatchProfile?.(index, profile);
}

// ── 配置 Config ───────────────────────────────────────────────────────────────

export async function getClashInfo() {
  return _ConfigAPI.GetClashInfo?.();
}

export async function getRuntimeConfig() {
  return _ConfigAPI.GetRuntimeConfig?.();
}

export async function getRuntimeYaml() {
  return _ConfigAPI.GetRuntimeYAML?.();
}

export async function getRuntimeExists() {
  const snap = await _ConfigAPI.GetRuntimeConfig?.();
  return snap?.exists_keys ? Object.keys(snap.exists_keys) : [];
}

export async function getRuntimeLogs() {
  return _ConfigAPI.GetRuntimeLogs?.();
}

export async function getRuntimeProxyChainConfig(proxyChainExitNode: string) {
  return _ConfigAPI.GetRuntimeProxyChainConfig?.(proxyChainExitNode);
}

export async function updateProxyChainConfigInRuntime(proxyChainConfig: unknown) {
  return _ConfigAPI.UpdateProxyChainConfigInRuntime?.(proxyChainConfig);
}

export async function patchClashConfig(payload: Partial<IConfigData>) {
  return _ConfigAPI.PatchClashConfig?.(payload);
}

export async function patchClashMode(payload: string) {
  return _ConfigAPI.PatchClashMode?.(payload);
}

export async function syncTrayProxySelection() {
  // App 层直接操作托盘，无需前端二次 emit
  return _App.SyncTrayProxySelection?.();
}

// ── 日志 Logs ────────────────────────────────────────────────────────────────

export async function getClashLogs() {
  const regex = /time="(.+?)"\s+level=(.+?)\s+msg="(.+?)"/;
  const newRegex = /(.+?)\s+(.+?)\s+(.+)/;
  const logs: string[] = await _ProxyAPI.GetClashLogs?.() ?? [];

  return logs.reduce<ILogItem[]>((acc, log) => {
    const result = log.match(regex);
    if (result) {
      const [, _time, type, payload] = result;
      const time = dayjs(_time).format("MM-DD HH:mm:ss");
      acc.push({ time, type, payload });
      return acc;
    }
    const result2 = log.match(newRegex);
    if (result2) {
      const [, time, type, payload] = result2;
      acc.push({ time, type, payload });
    }
    return acc;
  }, []);
}

export async function clearLogs() {
  // clashgo 没有单独的 clear_logs；重启 core 等效清空
  console.warn("[ClashGo] clearLogs: not implemented, logs are in-memory");
}

// ── Verge Config ─────────────────────────────────────────────────────────────

export async function getVergeConfig() {
  return _ConfigAPI.GetVergeConfig?.();
}

export async function patchVergeConfig(payload: IVergeConfig) {
  return _ConfigAPI.PatchVergeConfig?.(payload);
}

// ── 系统代理 ─────────────────────────────────────────────────────────────────

export async function getSystemProxy() {
  return _SystemAPI.GetSysProxy?.();
}

export async function getAutotemProxy() {
  try {
    debugLog("[API] 开始调用 get_auto_proxy");
    const result = await _SystemAPI.GetAutoProxy?.();
    debugLog("[API] get_auto_proxy 调用成功:", result);
    return result;
  } catch (error) {
    console.error("[API] get_auto_proxy 调用失败:", error);
    return { enable: false, url: "" };
  }
}

export async function getAutoLaunchStatus() {
  try {
    return await _SystemAPI.GetAutoLaunchStatus?.();
  } catch (error) {
    console.error("获取自启动状态失败:", error);
    return false;
  }
}

// ── 内核控制 ─────────────────────────────────────────────────────────────────

export async function changeClashCore(clashCore: string) {
  return _ProxyAPI.ChangeCoreVersion?.(clashCore);
}

export async function startCore() {
  return _ProxyAPI.StartCore?.();
}

export async function stopCore() {
  return _ProxyAPI.StopCore?.();
}

export async function restartCore() {
  return _ProxyAPI.RestartCore?.();
}

// ── 应用控制 ─────────────────────────────────────────────────────────────────

export async function restartApp() {
  return _App.RestartApp?.();
}

export async function getAppDir() {
  return _SystemAPI.GetAppDir?.();
}

export async function openAppDir() {
  return _SystemAPI.OpenAppDir?.().catch((err: unknown) => showNotice.error(String(err)));
}

export async function openCoreDir() {
  return _SystemAPI.OpenCoreDir?.().catch((err: unknown) => showNotice.error(String(err)));
}

export async function openLogsDir() {
  return _SystemAPI.OpenLogsDir?.().catch((err: unknown) => showNotice.error(String(err)));
}

export const openWebUrl = async (url: string) => {
  try {
    await _SystemAPI.OpenWebURL?.(url);
  } catch (err) {
    showNotice.error(String(err));
  }
};

// ── 延迟测试 ─────────────────────────────────────────────────────────────────

export async function cmdGetProxyDelay(
  name: string,
  timeout: number,
  url?: string,
) {
  const testUrl = url || "http://cp.cloudflare.com";
  try {
    const delay = await _ProxyAPI.TestProxyDelay?.(name, testUrl, timeout);
    if (typeof delay === "number") return { delay };
    return { delay: 1e6 };
  } catch {
    return { delay: 1e6 };
  }
}

export async function cmdTestDelay(url: string) {
  // 通用 URL 可达性测试（非代理测试）
  const start = Date.now();
  try {
    await fetch(url, { method: "HEAD", signal: AbortSignal.timeout(5000) });
    return Date.now() - start;
  } catch {
    return -1;
  }
}

// ── 系统信息 ─────────────────────────────────────────────────────────────────

export async function invoke_uwp_tool() {
  return _ServiceAPI.InvokeUWPTool?.().catch((err: unknown) =>
    showNotice.error(String(err), 1500),
  );
}

export async function getPortableFlag() {
  return _SystemAPI.GetPortableFlag?.();
}

export async function openDevTools() {
  return _App.OpenDevtools?.();
}

export async function exitApp() {
  return _App.ExitApp?.();
}

export async function exportDiagnosticInfo() {
  // TODO: 实现诊断信息导出
  console.warn("[ClashGo] exportDiagnosticInfo not yet implemented");
}

export async function getSystemInfo() {
  const info = await _SystemAPI.GetSystemInfo?.();
  if (!info) return "";
  return JSON.stringify(info);
}

// ── 图标 ─────────────────────────────────────────────────────────────────────

export async function copyIconFile(
  path: string,
  name: "common" | "sysproxy" | "tun",
) {
  const key = `icon_${name}_update_time`;
  const previousTime = localStorage.getItem(key) || "";
  const currentTime = String(Date.now());
  localStorage.setItem(key, currentTime);
  return _BackupAPI.CopyIconFile?.(path, name);
}

export async function downloadIconCache(url: string, name: string) {
  return _ConfigAPI.DownloadIconCache?.(url, name);
}

// ── 网络接口 ─────────────────────────────────────────────────────────────────

export async function getNetworkInterfaces() {
  return _SystemAPI.GetNetworkInterfaces?.();
}

export async function getSystemHostname() {
  return _SystemAPI.GetSystemHostname?.();
}

export async function getNetworkInterfacesInfo() {
  return _SystemAPI.GetNetworkInterfacesInfo?.();
}

// ── 备份 Backup ───────────────────────────────────────────────────────────────

export async function createWebdavBackup() {
  return _BackupAPI.UploadToWebDAV?.();
}

export async function createLocalBackup() {
  return _BackupAPI.CreateLocalBackup?.();
}

export async function deleteWebdavBackup(filename: string) {
  return _BackupAPI.DeleteWebDAVBackup?.(filename);
}

export async function deleteLocalBackup(filename: string) {
  return _BackupAPI.DeleteLocalBackup?.(filename);
}

export async function restoreWebDavBackup(filename: string) {
  return _BackupAPI.DownloadFromWebDAV?.(filename);
}

export async function restoreLocalBackup(filename: string) {
  return _BackupAPI.RestoreLocalBackup?.(filename);
}

export async function importLocalBackup(source: string) {
  return _BackupAPI.ImportLocalBackup?.(source);
}

export async function exportLocalBackup(filename: string, destination: string) {
  return _BackupAPI.ExportLocalBackup?.(filename, destination);
}

export async function saveWebdavConfig(
  url: string,
  username: string,
  password: string,
) {
  return _BackupAPI.SaveWebDAVConfig?.(url, username, password);
}

export async function listWebDavBackup() {
  const list: IWebDavFile[] = await _BackupAPI.ListWebDAVFiles?.() ?? [];
  list.forEach((item) => {
    item.filename = item.href.split("/").pop() as string;
  });
  return list;
}

export async function listLocalBackup() {
  return _BackupAPI.GetBackupList?.();
}

// ── 脚本验证 ─────────────────────────────────────────────────────────────────

export async function scriptValidateNotice(status: string, msg: string) {
  return _ProfileAPI.NotifyValidationResult?.(status, msg);
}

export async function validateScriptFile(filePath: string) {
  return _ProfileAPI.ValidateScriptFile?.(filePath);
}

// ── 运行状态 ─────────────────────────────────────────────────────────────────

export const getRunningMode = async () => {
  return _App.GetRunningMode?.();
};

export const getAppUptime = async () => {
  // ClashGo 未实现 uptime，返回 0
  return 0;
};

// ── 系统服务 ─────────────────────────────────────────────────────────────────

export const installService = async () => {
  return _ServiceAPI.InstallService?.();
};

export const uninstallService = async () => {
  return _ServiceAPI.UninstallService?.();
};

export const reinstallService = async () => {
  return _ServiceAPI.ReinstallService?.();
};

export const repairService = async () => {
  return _ServiceAPI.RepairService?.();
};

export const isServiceAvailable = async () => {
  try {
    return await _ServiceAPI.IsServiceAvailable?.();
  } catch (error) {
    console.error("Service check failed:", error);
    return false;
  }
};

// ── 轻量模式 ─────────────────────────────────────────────────────────────────

export const entry_lightweight_mode = async () => {
  return _App.EntryLightweightMode?.();
};

export const exit_lightweight_mode = async () => {
  return _App.ExitLightweightMode?.();
};

// ── 权限检查 ─────────────────────────────────────────────────────────────────

export const isAdmin = async () => {
  try {
    // ClashGo 没有 app_is_admin，用服务查询替代
    const status = await _ServiceAPI.GetServiceStatus?.();
    return status?.running ?? false;
  } catch {
    return false;
  }
};

// ── 更新时间 ─────────────────────────────────────────────────────────────────

export async function getNextUpdateTime(uid: string) {
  const ts = await _ProfileAPI.GetNextUpdateTime?.(uid);
  // 0 = 无计划
  return ts === 0 ? null : ts;
}

// ── 端口检查 ─────────────────────────────────────────────────────────────────

export const isPortInUse = async (port: number) => {
  try {
    // CheckPortAvailable 返回 true=可用（未占用），这里取反
    const available = await _SystemAPI.CheckPortAvailable?.(port);
    return !available;
  } catch {
    return false;
  }
};

// ── 代理列表（Mihomo REST，直接通过 ClashAPI 访问）──────────────────────────

export async function calcuProxyProviders() {
  const resp = await _ProxyAPI.GetProviders?.();
  if (!resp?.providers) return {};
  return Object.fromEntries(
    Object.entries(resp.providers as Record<string, unknown>)
      .sort()
      .filter(([, item]) => {
        const v = item as { vehicleType?: string };
        return v?.vehicleType === "HTTP" || v?.vehicleType === "File";
      }),
  );
}

export async function calcuProxies(): Promise<{
  global: IProxyGroupItem;
  direct: IProxyItem;
  groups: IProxyGroupItem[];
  records: Record<string, IProxyItem>;
  proxies: IProxyItem[];
}> {
  const [proxyResp, providerRecord] = await Promise.all([
    _ProxyAPI.GetProxies?.(),
    calcuProxyProviders(),
  ]);

  const proxyRecord: Record<string, IProxyItem> = proxyResp?.proxies ?? {};

  const providerMap = Object.fromEntries(
    Object.entries(providerRecord as Record<string, { proxies: IProxyItem[] }>)
      .flatMap(([provider, item]) =>
        (item?.proxies ?? []).map((p) => [p.name, { ...p, provider }]),
      ),
  );

  const generateItem = (name: string): IProxyItem => {
    if (proxyRecord[name]) return proxyRecord[name];
    if (providerMap[name]) return providerMap[name] as IProxyItem;
    return { name, type: "unknown", udp: false, history: [] } as unknown as IProxyItem;
  };

  const { GLOBAL: global, DIRECT: direct, REJECT: reject } = proxyRecord;

  let groups: IProxyGroupItem[] = Object.values(proxyRecord).reduce<IProxyGroupItem[]>(
    (acc, each) => {
      if (each?.name !== "GLOBAL" && each?.all) {
        acc.push({ ...each, all: each.all!.map(generateItem) });
      }
      return acc;
    },
    [],
  );

  if (global?.all) {
    const globalGroups: IProxyGroupItem[] = global.all.reduce<IProxyGroupItem[]>(
      (acc, name) => {
        if (proxyRecord[name]?.all) {
          acc.push({ ...proxyRecord[name], all: proxyRecord[name].all!.map(generateItem) });
        }
        return acc;
      },
      [],
    );
    const globalNames = new Set(globalGroups.map((g) => g.name));
    groups = groups.filter((g) => !globalNames.has(g.name)).concat(globalGroups);
  }

  const proxies = [direct, reject].concat(
    Object.values(proxyRecord).filter(
      (p) => !p?.all?.length && p?.name !== "DIRECT" && p?.name !== "REJECT",
    ),
  );

  return {
    global: { ...global, all: global?.all?.map(generateItem) ?? [] } as IProxyGroupItem,
    direct: direct as IProxyItem,
    groups,
    records: proxyRecord,
    proxies: (proxies as IProxyItem[]) ?? [],
  };
}
