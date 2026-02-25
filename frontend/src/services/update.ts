/**
 * ClashGo 自动更新检查（替代 Tauri plugin-updater）
 *
 * 通过 GitHub/Gitee API 检查最新 Release，比对版本号。
 * 多源 fallback：先查 GitHub，失败查 Gitee。
 */

import { version as appVersion } from "@root/package.json";

export type VersionParts = {
  main: number[];
  pre: (number | string)[];
};

const SEMVER_FULL_REGEX =
  /^\d+(?:\.\d+){1,2}(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/;
const SEMVER_SEARCH_REGEX =
  /v?\d+(?:\.\d+){1,2}(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?/i;

export const normalizeVersion = (
  input: string | null | undefined,
): string | null => {
  if (typeof input !== "string") return null;
  const trimmed = input.trim();
  if (!trimmed) return null;
  return trimmed.replace(/^v/i, "");
};

export const ensureSemver = (
  input: string | null | undefined,
): string | null => {
  const normalized = normalizeVersion(input);
  if (!normalized) return null;
  return SEMVER_FULL_REGEX.test(normalized) ? normalized : null;
};

export const extractSemver = (
  input: string | null | undefined,
): string | null => {
  if (typeof input !== "string") return null;
  const match = input.match(SEMVER_SEARCH_REGEX);
  if (!match) return null;
  return normalizeVersion(match[0]);
};

export const splitVersion = (version: string | null): VersionParts | null => {
  if (!version) return null;
  const [mainPart, preRelease] = version.split("-");
  const main = mainPart
    .split(".")
    .map((part) => Number.parseInt(part, 10))
    .map((num) => (Number.isNaN(num) ? 0 : num));

  const pre =
    preRelease?.split(".").map((token) => {
      const numeric = Number.parseInt(token, 10);
      return Number.isNaN(numeric) ? token : numeric;
    }) ?? [];

  return { main, pre };
};

const compareVersionParts = (a: VersionParts, b: VersionParts): number => {
  const length = Math.max(a.main.length, b.main.length);
  for (let i = 0; i < length; i += 1) {
    const diff = (a.main[i] ?? 0) - (b.main[i] ?? 0);
    if (diff !== 0) return diff > 0 ? 1 : -1;
  }

  if (a.pre.length === 0 && b.pre.length === 0) return 0;
  if (a.pre.length === 0) return 1;
  if (b.pre.length === 0) return -1;

  const preLen = Math.max(a.pre.length, b.pre.length);
  for (let i = 0; i < preLen; i += 1) {
    const aToken = a.pre[i];
    const bToken = b.pre[i];
    if (aToken === undefined) return -1;
    if (bToken === undefined) return 1;

    if (typeof aToken === "number" && typeof bToken === "number") {
      if (aToken > bToken) return 1;
      if (aToken < bToken) return -1;
      continue;
    }

    if (typeof aToken === "number") return -1;
    if (typeof bToken === "number") return 1;

    if (aToken > bToken) return 1;
    if (aToken < bToken) return -1;
  }

  return 0;
};

export const compareVersions = (
  a: string | null,
  b: string | null,
): number | null => {
  const partsA = splitVersion(a);
  const partsB = splitVersion(b);
  if (!partsA || !partsB) return null;
  return compareVersionParts(partsA, partsB);
};

// ── Release 信息 ─────────────────────────────────────────────────────────

export interface UpdateInfo {
  version: string;
  body: string;
  date: string;
  available: boolean;
  downloadUrl: string;
}

// ── 多源检查（GitHub → Gitee fallback）────────────────────────────────────

const RELEASE_SOURCES = [
  {
    name: "GitHub",
    url: "https://api.github.com/repos/liuguangzhong/clashgo/releases/latest",
    parse: (data: any): UpdateInfo => ({
      version: normalizeVersion(data.tag_name) || "",
      body: data.body || "",
      date: data.published_at || "",
      available: false,
      downloadUrl: data.html_url || "",
    }),
  },
  {
    name: "Gitee",
    url: "https://gitee.com/api/v5/repos/open-nexusai/clashgo/releases/latest",
    parse: (data: any): UpdateInfo => ({
      version: normalizeVersion(data.tag_name) || "",
      body: data.body || "",
      date: data.created_at || "",
      available: false,
      downloadUrl: `https://gitee.com/open-nexusai/clashgo/releases/tag/${data.tag_name}`,
    }),
  },
];

const localVersionNormalized = normalizeVersion(appVersion);

export const checkUpdateSafe = async (): Promise<UpdateInfo | null> => {
  let lastError: unknown = null;

  for (const source of RELEASE_SOURCES) {
    try {
      console.debug(`[updater] Checking ${source.name}: ${source.url}`);

      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 10000);

      const resp = await fetch(source.url, {
        signal: controller.signal,
        headers: { Accept: "application/json" },
      });
      clearTimeout(timeoutId);

      if (!resp.ok) {
        throw new Error(`HTTP ${resp.status} from ${source.name}`);
      }

      const data = await resp.json();
      const info = source.parse(data);

      if (!info.version) {
        throw new Error(`No version found from ${source.name}`);
      }

      const remoteVersion = ensureSemver(info.version) || info.version;
      const cmp = compareVersions(remoteVersion, localVersionNormalized);

      if (cmp !== null && cmp > 0) {
        info.available = true;
        info.version = remoteVersion;
        console.info(
          `[updater] New version available: ${remoteVersion} (current: ${localVersionNormalized}) from ${source.name}`,
        );
        return info;
      }

      console.debug(
        `[updater] Up to date (remote: ${remoteVersion}, local: ${localVersionNormalized}) from ${source.name}`,
      );
      return null;
    } catch (err) {
      console.warn(`[updater] ${source.name} check failed:`, err);
      lastError = err;
    }
  }

  console.error("[updater] All sources failed:", lastError);
  return null;
};

export type CheckOptions = Record<string, unknown>;
