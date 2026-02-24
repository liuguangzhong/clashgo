'use strict';
import { Call as $Call } from "@wailsio/runtime";

export const GetSysProxy = () => $Call.ByName("api.SystemAPI.GetSysProxy");
export const GetAutoProxy = () => $Call.ByName("api.SystemAPI.GetAutoProxy");
export const GetAutoLaunchStatus = () => $Call.ByName("api.SystemAPI.GetAutoLaunchStatus");
export const GetNetworkInterfaces = () => $Call.ByName("api.SystemAPI.GetNetworkInterfaces");
export const GetNetworkInterfacesInfo = () => $Call.ByName("api.SystemAPI.GetNetworkInterfacesInfo");
export const GetSystemHostname = () => $Call.ByName("api.SystemAPI.GetSystemHostname");
export const CheckPortAvailable = (port) => $Call.ByName("api.SystemAPI.CheckPortAvailable", port);
export const GetSystemInfo = () => $Call.ByName("api.SystemAPI.GetSystemInfo");
export const OpenAppDir = () => $Call.ByName("api.SystemAPI.OpenAppDir");
export const OpenCoreDir = () => $Call.ByName("api.SystemAPI.OpenCoreDir");
export const OpenLogsDir = () => $Call.ByName("api.SystemAPI.OpenLogsDir");
export const OpenWebURL = (url) => $Call.ByName("api.SystemAPI.OpenWebURL", url);
export const GetPortableFlag = () => $Call.ByName("api.SystemAPI.GetPortableFlag");
export const GetAppDir = () => $Call.ByName("api.SystemAPI.GetAppDir");
export const GetCoreVersion = () => $Call.ByName("api.SystemAPI.GetCoreVersion");
export const CopyClashEnv = () => $Call.ByName("api.SystemAPI.CopyClashEnv");
