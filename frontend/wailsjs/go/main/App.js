'use strict';
import { Call as $Call } from "@wailsio/runtime";

export const NotifyUIReady = () => $Call.ByName("main.App.NotifyUIReady");
export const ExitApp = () => $Call.ByName("main.App.ExitApp");
export const RestartApp = () => $Call.ByName("main.App.RestartApp");
export const OpenDevtools = () => $Call.ByName("main.App.OpenDevtools");
export const GetRunningMode = () => $Call.ByName("main.App.GetRunningMode");
export const IsLightweightMode = () => $Call.ByName("main.App.IsLightweightMode");
export const EntryLightweightMode = () => $Call.ByName("main.App.EntryLightweightMode");
export const ExitLightweightMode = () => $Call.ByName("main.App.ExitLightweightMode");
export const SyncTrayProxySelection = () => $Call.ByName("main.App.SyncTrayProxySelection");
export const StartHidden = () => $Call.ByName("main.App.StartHidden");
