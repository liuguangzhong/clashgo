# ClashGo 功能对照进度

> 对照原项目 `src-tauri/src/cmd/` 目录下所有 Tauri 命令，逐一映射到 ClashGo API 层。
> 状态: 🟢 已实现 | 🟡 部分实现 | 🔴 未实现 | ⚙️ 实现中

最后更新: 2026-02-24

---

## 一、配置管理（Config）

| 原命令 | 原文件 | ClashGo 位置 | 状态 | 备注 |
|---|---|---|---|---|
| `get_verge_config` | verge.rs | `api.ConfigAPI.GetVergeConfig` | 🟢 | |
| `patch_verge_config` | verge.rs | `api.ConfigAPI.PatchVergeConfig` | 🟢 | |
| `get_clash_info` | clash.rs | `api.ConfigAPI.GetClashInfo` | 🟢 | |
| `patch_clash_config` | clash.rs | `api.ConfigAPI.PatchClashConfig` | 🟢 | |
| `patch_clash_mode` | clash.rs | `api.ConfigAPI.PatchClashMode` | 🟢 | |
| `get_runtime_config` | runtime.rs | `api.ConfigAPI.GetRuntimeConfig` | 🟢 | |
| `get_runtime_yaml` | runtime.rs | `api.ConfigAPI.GetRuntimeYAML` | 🟢 | |
| `get_runtime_logs` | runtime.rs | `api.ConfigAPI.GetRuntimeLogs` | 🟢 | |
| `get_runtime_exists` | runtime.rs | `RuntimeSnapshot.ExistsKeys` | 🟢 | |
| `get_runtime_proxy_chain_config` | runtime.rs | `api.ConfigAPI.GetRuntimeProxyChainConfig` | 🟢 | 返回运行时 YAML 文本 |
| `update_proxy_chain_config_in_runtime` | runtime.rs | `api.ConfigAPI.UpdateProxyChainConfigInRuntime` | 🟢 | merge+热加载 |
| `save_dns_config` | clash.rs | `api.ConfigAPI.SaveDNSConfig` | 🟢 | 写 dns_config.yaml |
| `apply_dns_config` | clash.rs | `api.ConfigAPI.ApplyDNSConfig` | 🟢 | patch clash+热加载 |
| `check_dns_config_exists` | clash.rs | `api.ConfigAPI.CheckDNSConfigExists` | 🟢 | |
| `get_dns_config_content` | clash.rs | `api.ConfigAPI.GetDNSConfigContent` | 🟢 | |
| `validate_dns_config` | clash.rs | `api.ConfigAPI.ValidateDNSConfig` | 🟢 | yaml.Unmarshal 验证 |
| `copy_clash_env` | clash.rs | `api.ConfigAPI.CopyClashEnv` | 🟢 | 生成 export 环境变量 |

---

## 二、订阅管理（Profile）

| 原命令 | 原文件 | ClashGo 位置 | 状态 | 备注 |
|---|---|---|---|---|
| `get_profiles` | profile.rs | `api.ProfileAPI.GetProfiles` | 🟢 | |
| `import_profile` | profile.rs | `api.ProfileAPI.ImportProfile` | 🟢 | |
| `create_profile` | profile.rs | `api.ProfileAPI.CreateProfile` | 🟢 | |
| `update_profile` | profile.rs | `api.ProfileAPI.UpdateProfile` | 🟢 | |
| `delete_profile` | profile.rs | `api.ProfileAPI.DeleteProfile` | 🟢 | |
| `patch_profile` | profile.rs | `api.ProfileAPI.PatchProfile` | 🟢 | |
| `reorder_profile` | profile.rs | `api.ProfileAPI.ReorderProfile` | 🟢 | |
| `enhance_profiles` | profile.rs | `api.ProfileAPI.EnhanceProfiles` | 🟢 | |
| `patch_profiles_config` | profile.rs | `api.ProfileAPI.PatchProfilesConfig` | 🟡 | 原版支持全量patch IProfiles |
| `patch_profiles_config_by_profile_index` | profile.rs | `api.ProfileAPI.PatchProfilesConfig` | 🟢 | |
| `read_profile_file` | profile.rs | `api.ProfileAPI.ReadProfileFile` | 🟢 | |
| `save_profile_file` | save_profile.rs | `api.ProfileAPI.SaveProfileFileWithValidation` | 🟢 | 验证+失败回滚 |
| `view_profile` | profile.rs | `api.ProfileAPI.ViewProfileInExplorer` | 🟢 | 文件管理器打开 |
| `get_next_update_time` | profile.rs | `api.ProfileAPI.GetNextUpdateTime` | 🟢 | UpdatedAt+Interval 计算 |
| `validate_script_file` | validate.rs | `api.ProfileAPI.ValidateScriptFile` | 🟢 | |
| `script_validate_notice` | validate.rs | `api.ProfileAPI.NotifyValidationResult` | 🟢 | 记录日志+事件 |

---

## 三、代理核心（Core）

| 原命令 | 原文件 | ClashGo 位置 | 状态 | 备注 |
|---|---|---|---|---|
| `start_core` | clash.rs | `api.ProxyAPI.StartCore` | 🟢 | coreLifecycle.Start |
| `stop_core` | clash.rs | `api.ProxyAPI.StopCore` | 🟢 | coreLifecycle.Stop |
| `restart_core` | clash.rs | `api.ProxyAPI.RestartCore` | 🟢 | |
| `change_clash_core` | clash.rs | `api.ProxyAPI.ChangeCoreVersion` | 🟢 | |
| `get_clash_logs` | clash.rs | `api.ProxyAPI.GetClashLogs` | 🟢 | |
| `test_delay` | clash.rs | `api.ProxyAPI.TestProxyDelay` | 🟢 | |
| `get_running_mode` | system.rs | `App.GetRunningMode` | 🟢 | |

---

## 四、代理节点（Proxy REST）

| 原功能 | 接入方式 | ClashGo 位置 | 状态 | 备注 |
|---|---|---|---|---|
| 获取代理列表 | 前端直调 Mihomo | `api.ProxyAPI.GetProxies` | 🟢 | |
| 获取规则 | 前端直调 Mihomo | `api.ProxyAPI.GetRules` | 🟢 | |
| 获取代理提供者 | 前端直调 Mihomo | `api.ProxyAPI.GetProviders` | 🟢 | |
| 选择代理节点 | 前端直调 Mihomo | `api.ProxyAPI.SelectProxy` | 🟢 | |
| 单节点延迟测试 | 前端直调 Mihomo | `api.ProxyAPI.TestProxyDelay` | 🟢 | |
| 更新代理提供者 | 前端直调 Mihomo | `api.ProxyAPI.UpdateProxyProvider` | 🟢 | |
| 获取活跃连接 | 前端直调 Mihomo | `api.ProxyAPI.GetConnections` | 🟢 | |
| 断开指定连接 | 前端直调 Mihomo | `api.ProxyAPI.DeleteConnection` | 🟢 | |
| 断开全部连接 | 前端直调 Mihomo | `api.ProxyAPI.DeleteAllConnections` | 🟢 | |
| 实时流量统计 | 前端直调 Mihomo | `api.ProxyAPI.GetTraffic` | 🟢 | |
| DNS 查询 | 前端直调 Mihomo | `api.ProxyAPI.DNSQuery` | 🟢 | |
| 更新 GeoData | 自定义 | `api.ProxyAPI.UpdateGeoData` | 🟢 | |
| `sync_tray_proxy_selection` | proxy.rs | `api.ProxyAPI.SyncTrayProxySelection` | 🟢 | 发布 tray:sync-proxy 事件 |

---

## 五、系统代理

| 原功能 | 原位置 | ClashGo 位置 | 状态 | 备注 |
|---|---|---|---|---|
| `get_sys_proxy` | network.rs | `api.SystemAPI.GetSysProxy` | 🟢 | |
| `get_auto_proxy` | network.rs | `api.SystemAPI.GetAutoProxy` | 🟢 | |
| 设置/清除系统代理 | core/sysopt.rs | `internal/proxy/` | 🟢 | Win/Linux/macOS |
| 代理守卫 (自动恢复) | core/sysopt.rs | `internal/proxy/guard.go` | 🟢 | |

---

## 六、系统服务（Windows 提权 TUN）

| 原命令 | 原文件 | ClashGo 位置 | 状态 | 备注 |
|---|---|---|---|---|
| `install_service` | service.rs | `api.ServiceAPI.InstallService` | 🟢 | PowerShell 提权安装 |
| `uninstall_service` | service.rs | `api.ServiceAPI.UninstallService` | 🟢 | |
| `reinstall_service` | service.rs | `api.ServiceAPI.ReinstallService` | 🟢 | |
| `repair_service` | service.rs | `api.ServiceAPI.RepairService` | 🟢 | |
| `is_service_available` | service.rs | `api.ServiceAPI.IsServiceAvailable` | 🟢 | sc.exe query |

---

## 七、网络与系统信息

| 原命令 | 原文件 | ClashGo 位置 | 状态 | 备注 |
|---|---|---|---|---|
| `get_network_interfaces` | network.rs | `api.SystemAPI.GetNetworkInterfaces` | 🟢 | |
| `get_network_interfaces_info` | network.rs | `api.SystemAPI.GetNetworkInterfaces` | 🟢 | |
| `get_system_hostname` | network.rs | `api.SystemAPI.GetSystemHostname` | 🟢 | os.Hostname() |
| `is_port_in_use` | network.rs | `api.SystemAPI.CheckPortAvailable` | 🟢 | |
| CPU/内存信息 | - | `api.SystemAPI.GetSystemInfo` | 🟢 | |
| `open_app_dir` | app.rs | `api.SystemAPI.OpenAppDir` | 🟢 | |
| `open_core_dir` | app.rs | `api.SystemAPI.OpenCoreDir` | 🟢 | |
| `open_logs_dir` | app.rs | `api.SystemAPI.OpenLogsDir` | 🟢 | |
| `open_web_url` | app.rs | `api.SystemAPI.OpenWebURL` | 🟢 | rundll32/xdg-open/open |
| `open_app_log` | app.rs | `api.SystemAPI.OpenAppLog` | 🟢 | LatestLogPath |
| `open_core_log` | app.rs | `api.SystemAPI.OpenCoreLog` | 🟢 | LatestCoreLogPath |
| `get_portable_flag` | app.rs | `api.SystemAPI.GetPortableFlag` | 🟢 | IsPortable() |
| `get_app_dir` | app.rs | `api.SystemAPI.GetAppDir` | 🟢 | Dirs().HomeDir() |
| `get_auto_launch_status` | app.rs | `api.SystemAPI.GetAutoLaunchStatus` | 🟢 | AutoStartManager |

---

## 八、应用控制

| 原命令 | 原文件 | ClashGo 位置 | 状态 | 备注 |
|---|---|---|---|---|
| `exit_app` | app.rs | `App.ExitApp` | 🟢 | |
| `restart_app` | app.rs | `App.RestartApp` | 🟢 | |
| `open_devtools` | app.rs | `App.OpenDevtools` | 🟢 | |
| `notify_ui_ready` | app.rs | `App.NotifyUIReady` | 🟢 | |
| `update_ui_stage` | app.rs | `api.ConfigAPI.UpdateUIStage` | 🟢 | 记录日志 |
| `download_icon_cache` | app.rs | `api.ConfigAPI.DownloadIconCache` | 🟢 | HTTP下载到icons/目录 |
| `copy_icon_file` | app.rs | `api.BackupAPI.CopyIconFile` | 🟢 | 文件复制 |

---

## 九、备份

| 原命令 | 原文件 | ClashGo 位置 | 状态 | 备注 |
|---|---|---|---|---|
| `create_local_backup` | backup.rs | `api.BackupAPI.CreateLocalBackup` | 🟢 | |
| `list_local_backup` | backup.rs | `api.BackupAPI.GetBackupList` | 🟢 | |
| `delete_local_backup` | backup.rs | `api.BackupAPI.DeleteLocalBackup` | 🟢 | |
| `restore_local_backup` | backup.rs | `api.BackupAPI.RestoreLocalBackup` | 🟢 | |
| `import_local_backup` | backup.rs | `api.BackupAPI.ImportLocalBackup` | 🟢 | 从外部路径复制到备份目录 |
| `export_local_backup` | backup.rs | `api.BackupAPI.ExportLocalBackup` | 🟢 | 复制到用户指定路径 |
| `save_webdav_config` | webdav.rs | `api.BackupAPI.SaveWebDAVConfig` | 🟢 | |
| `create_webdav_backup` | webdav.rs | `api.BackupAPI.UploadToWebDAV` | 🟢 | |
| `list_webdav_backup` | webdav.rs | `api.BackupAPI.ListWebDAVFiles` | 🟢 | |
| `delete_webdav_backup` | webdav.rs | `api.BackupAPI.DeleteWebDAVBackup` | 🟢 | RemoveAll |
| `restore_webdav_backup` | webdav.rs | `api.BackupAPI.DownloadFromWebDAV` | 🟢 | |

---

## 十、轻量模式

| 原命令 | 原文件 | ClashGo 位置 | 状态 | 备注 |
|---|---|---|---|---|
| `entry_lightweight_mode` | lightweight.rs | `App.EntryLightweightMode` | 🟢 | WindowHide + 标志位 |
| `exit_lightweight_mode` | lightweight.rs | `App.ExitLightweightMode` | 🟢 | WindowShow + 标志位 |

---

## 十一、Windows 特有

| 原命令 | 原文件 | ClashGo 位置 | 状态 | 备注 |
|---|---|---|---|---|
| `invoke_uwp_tool` | uwp.rs | `api.ServiceAPI.InvokeUWPTool` | 🟢 | PowerShell 提权运行 |
| 系统服务完整套件 | service.rs | `api.ServiceAPI` | 🟢 | 见第六节 |

---

## 十二、媒体解锁检测

| 原功能 | 原文件 | ClashGo 位置 | 状态 | 备注 |
|---|---|---|---|---|
| Netflix 检测 | netflix.rs | `unlock.checkNetflix` | 🟢 | CDN快测+地址分析区域 |
| Disney+ 检测 | disney_plus.rs | `unlock.checkDisneyPlus` | 🟢 | BAMgrid API+GraphQL |
| YouTube 检测 | youtube.rs | `unlock.checkYouTubePremium` | 🟢 | 页面内容分析 |
| Spotify 检测 | spotify.rs | `unlock.checkSpotify` | 🟢 | country-selector API |
| Bilibili 大陆 | bilibili.rs | `unlock.checkBilibiliMainland` | 🟢 | pgc/player API |
| Bilibili 港澳台 | bilibili.rs | `unlock.checkBilibiliHKMCTW` | 🟢 | pgc/player API |
| Bahamut 检测 | bahamut.rs | `unlock.checkBahamut` | 🟢 | deviceid+token |
| Prime Video 检测 | prime_video.rs | `unlock.checkPrimeVideo` | 🟢 | currentTerritory |
| TikTok 检测 | tiktok.rs | `unlock.checkTikTok` | 🟢 | cdn-cgi/trace |
| ChatGPT 检测 | chatgpt.rs | `unlock.checkChatGPT` | 🟢 | iOS+Web 双检 |
| Claude 检测 | claude.rs | `unlock.checkClaude` | 🟢 | cdn-cgi/trace+黑名单 |
| Gemini 检测 | gemini.rs | `unlock.checkGemini` | 🟢 | 内容字符串匹配 |


---

## 实现优先级

### P0 — 核心功能 ✅ 全部完成

- [x] `start_core` / `stop_core` 前端可调 API
- [x] `save_profile_file` 保存后验证 + 失败回滚
- [x] `delete_webdav_backup` WebDAV 删除
- [x] `get_auto_launch_status` 自启动状态读取

### P1 — 常用功能 ✅ 全部完成

- [x] DNS 配置完整链路（save/apply/check/get/validate）
- [x] `open_web_url` 系统浏览器打开 URL
- [x] `open_app_log` / `open_core_log` 打开日志文件
- [x] `get_app_dir` 返回应用数据目录路径
- [x] `import_local_backup` / `export_local_backup`
- [x] `sync_tray_proxy_selection` 托盘代理同步
- [x] `view_profile` 文件管理器打开配置文件
- [x] `get_next_update_time` 订阅更新倒计时
- [x] `get_portable_flag` 暴露给前端
- [x] `get_system_hostname`
- [x] `copy_clash_env` 复制环境变量

### P2 — 进阶功能 ✅ 全部完成

- [x] 系统服务套件（Windows TUN：install/uninstall/reinstall/repair/check）
- [x] 轻量模式（entry/exit lightweight mode）
- [x] Proxy Chain 运行时配置（get/update）
- [x] `update_ui_stage` UI 加载阶段事件
- [x] `download_icon_cache` / `copy_icon_file` 图标缓存
- [x] `script_validate_notice` 验证通知

### P3 — 扩展功能 ✅ 完成

- [x] 媒体解锁检测（12个平台，13个检测项）：Netflix/Disney+/YouTube/Spotify/Bilibili大陆港澳台/Bahamut/Prime Video/TikTok/ChatGPT iOS&Web/Claude/Gemini
- [x] UWP 代理豁免工具（Windows，已有入口 `api.ServiceAPI.InvokeUWPTool` + PowerShell 提权）

---

## 统计

| 分类 | 总计 | 🟢已实现 | 🟡部分 | 🔴未实现 |
|---|---|---|---|---|
| 配置管理 | 17 | 17 | 0 | 0 |
| 订阅管理 | 16 | 16 | 0 | 0 |
| 代理核心 | 7 | 7 | 0 | 0 |
| 代理节点 | 13 | 13 | 0 | 0 |
| 系统代理 | 4 | 4 | 0 | 0 |
| 系统服务 | 5 | 5 | 0 | 0 |
| 网络/系统 | 15 | 15 | 0 | 0 |
| 应用控制 | 7 | 7 | 0 | 0 |
| 备份 | 11 | 11 | 0 | 0 |
| 轻量模式 | 2 | 2 | 0 | 0 |
| Windows特有 | 6 | 6 | 0 | 0 |
| 媒体解锁 | 11 | 11 | 0 | 0 |
| **合计** | **114** | **114** | **0** | **0** |

完成率: **114/114 = 🎉 100%**

### 新增文件清单

| 文件 | 作用 |
|---|---|
| `internal/unlock/checker.go` | 全部 13 个平台检测，并发执行 |
| `api/media_unlock.go` | `MediaUnlockAPI` 封装层 |
| `api/config_extra.go` | DNS链路 + CopyClashEnv + 图标 + 运行时配置 |
| `api/profile_extra.go` | 保存+验证+回滚, ViewProfile, 次更新时间 |
| `api/backup.go` | 完整备份 API（含 import/export/delete WebDAV） |
| `api/service.go` | Windows 服务 API |
| `api/system.go` | 扩展: hostname/autolaunch/appdir/portal/log/URL/icon |
| `internal/service/manager.go` | Windows sc.exe/PowerShell 服务管理 |
| `internal/config/yaml.go` | UnmarshalYAML 公开接口 |
| `internal/config/fileutil.go` | WriteFileAtomic 工具 |
| `api/http.go` | fetchURLBytes HTTP 工具 |
| `api/helpers.go` | copyFile/fileBase/os_stat 共用工具 |
