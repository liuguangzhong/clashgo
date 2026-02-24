'use strict';
import { Call as $Call } from "@wailsio/runtime";

export const GetProfiles = () => $Call.ByName("api.ProfileAPI.GetProfiles");
export const EnhanceProfiles = () => $Call.ByName("api.ProfileAPI.EnhanceProfiles");
export const PatchProfilesConfig = (uid) => $Call.ByName("api.ProfileAPI.PatchProfilesConfig", uid);
export const CreateProfile = (item, fileData) => $Call.ByName("api.ProfileAPI.CreateProfile", item, fileData);
export const ViewProfileInExplorer = (uid) => $Call.ByName("api.ProfileAPI.ViewProfileInExplorer", uid);
export const ReadProfileFile = (uid) => $Call.ByName("api.ProfileAPI.ReadProfileFile", uid);
export const SaveProfileFileWithValidation = (uid, content) => $Call.ByName("api.ProfileAPI.SaveProfileFileWithValidation", uid, content);
export const ImportProfile = (url, option) => $Call.ByName("api.ProfileAPI.ImportProfile", url, option);
export const ReorderProfile = (activeUID, overUID) => $Call.ByName("api.ProfileAPI.ReorderProfile", activeUID, overUID);
export const UpdateProfile = (uid) => $Call.ByName("api.ProfileAPI.UpdateProfile", uid);
export const DeleteProfile = (uid) => $Call.ByName("api.ProfileAPI.DeleteProfile", uid);
export const PatchProfile = (uid, patch) => $Call.ByName("api.ProfileAPI.PatchProfile", uid, patch);
export const GetNextUpdateTime = (uid) => $Call.ByName("api.ProfileAPI.GetNextUpdateTime", uid);
export const ValidateScriptFile = (uid) => $Call.ByName("api.ProfileAPI.ValidateScriptFile", uid);
export const NotifyValidationResult = (status, msg) => $Call.ByName("api.ProfileAPI.NotifyValidationResult", status, msg);
