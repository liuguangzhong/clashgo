'use strict';
import { Call as $Call } from "@wailsio/runtime";

export const InstallService = () => $Call.ByName("api.ServiceAPI.InstallService");
export const UninstallService = () => $Call.ByName("api.ServiceAPI.UninstallService");
export const ReinstallService = () => $Call.ByName("api.ServiceAPI.ReinstallService");
export const RepairService = () => $Call.ByName("api.ServiceAPI.RepairService");
export const IsServiceAvailable = () => $Call.ByName("api.ServiceAPI.IsServiceAvailable");
export const GetServiceStatus = () => $Call.ByName("api.ServiceAPI.GetServiceStatus");
export const InvokeUWPTool = () => $Call.ByName("api.ServiceAPI.InvokeUWPTool");
