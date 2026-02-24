'use strict';
import { Call as $Call } from "@wailsio/runtime";

export const CreateLocalBackup = () => $Call.ByName("api.BackupAPI.CreateLocalBackup");
export const GetBackupList = () => $Call.ByName("api.BackupAPI.GetBackupList");
export const DeleteLocalBackup = (filename) => $Call.ByName("api.BackupAPI.DeleteLocalBackup", filename);
export const RestoreLocalBackup = (filename) => $Call.ByName("api.BackupAPI.RestoreLocalBackup", filename);
export const ImportLocalBackup = (src) => $Call.ByName("api.BackupAPI.ImportLocalBackup", src);
export const ExportLocalBackup = (filename, dst) => $Call.ByName("api.BackupAPI.ExportLocalBackup", filename, dst);
export const SaveWebDAVConfig = (url, user, pass) => $Call.ByName("api.BackupAPI.SaveWebDAVConfig", url, user, pass);
export const UploadToWebDAV = () => $Call.ByName("api.BackupAPI.UploadToWebDAV");
export const ListWebDAVFiles = () => $Call.ByName("api.BackupAPI.ListWebDAVFiles");
export const DeleteWebDAVBackup = (filename) => $Call.ByName("api.BackupAPI.DeleteWebDAVBackup", filename);
export const DownloadFromWebDAV = (filename) => $Call.ByName("api.BackupAPI.DownloadFromWebDAV", filename);
export const CopyIconFile = (src, name) => $Call.ByName("api.BackupAPI.CopyIconFile", src, name);
