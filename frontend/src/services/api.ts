import { asyncRetry } from "foxts/async-retry";
import { extractErrorMessage } from "foxts/extract-error-message";

import { debugLog } from "@/utils/debug";

// Get current IP and geolocation information
interface IpInfo {
  ip: string;
  country_code: string;
  country: string;
  region: string;
  city: string;
  organization: string;
  asn: number;
  asn_organization: string;
  longitude: number;
  latitude: number;
  timezone: string;
}

// IP检测服务配置
interface ServiceConfig {
  url: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mapping: (data: any) => IpInfo;
}

// 可用的IP检测服务列表及字段映射
const IP_CHECK_SERVICES: ServiceConfig[] = [
  {
    url: "https://api.ip.sb/geoip",
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mapping: (data: any) => ({
      ip: data.ip || "",
      country_code: data.country_code || "",
      country: data.country || "",
      region: data.region || "",
      city: data.city || "",
      organization: data.organization || data.isp || "",
      asn: data.asn || 0,
      asn_organization: data.asn_organization || "",
      longitude: data.longitude || 0,
      latitude: data.latitude || 0,
      timezone: data.timezone || "",
    }),
  },
  {
    url: "https://ipapi.co/json",
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mapping: (data: any) => ({
      ip: data.ip || "",
      country_code: data.country_code || "",
      country: data.country_name || "",
      region: data.region || "",
      city: data.city || "",
      organization: data.org || "",
      asn: data.asn ? parseInt(data.asn.replace("AS", "")) : 0,
      asn_organization: data.org || "",
      longitude: data.longitude || 0,
      latitude: data.latitude || 0,
      timezone: data.timezone || "",
    }),
  },
  {
    url: "https://api.ipapi.is/",
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mapping: (data: any) => ({
      ip: data.ip || "",
      country_code: data.location?.country_code || "",
      country: data.location?.country || "",
      region: data.location?.state || "",
      city: data.location?.city || "",
      organization: data.asn?.org || data.company?.name || "",
      asn: data.asn?.asn || 0,
      asn_organization: data.asn?.org || "",
      longitude: data.location?.longitude || 0,
      latitude: data.location?.latitude || 0,
      timezone: data.location?.timezone || "",
    }),
  },
  {
    url: "https://ipwho.is/",
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mapping: (data: any) => ({
      ip: data.ip || "",
      country_code: data.country_code || "",
      country: data.country || "",
      region: data.region || "",
      city: data.city || "",
      organization: data.connection?.org || data.connection?.isp || "",
      asn: data.connection?.asn || 0,
      asn_organization: data.connection?.isp || "",
      longitude: data.longitude || 0,
      latitude: data.latitude || 0,
      timezone: data.timezone?.id || "",
    }),
  },
  {
    url: "https://get.geojs.io/v1/ip/geo.json",
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mapping: (data: any) => ({
      ip: data.ip || "",
      country_code: data.country_code || "",
      country: data.country || "",
      region: data.region || "",
      city: data.city || "",
      organization: data.organization_name || "",
      asn: data.asn || 0,
      asn_organization: data.organization_name || "",
      longitude: Number(data.longitude) || 0,
      latitude: Number(data.latitude) || 0,
      timezone: data.timezone || "",
    }),
  },
];

/**
 * 通过 Go 后端 SystemAPI.FetchViaProxy 发 HTTP GET 请求。
 * 这样请求会经过 Clash 代理端口（localhost:7897），
 * 绕过 WebKit2GTK 在 Linux 下不走系统代理的限制。
 */
async function fetchViaGoProxy(url: string, proxyPort: number): Promise<string> {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let SystemAPI: any = null;
  try {
    SystemAPI = await import("../../wailsjs/go/api/SystemAPI");
  } catch {
    // not in Wails environment
  }

  if (SystemAPI?.FetchViaProxy) {
    return SystemAPI.FetchViaProxy(url, proxyPort);
  }

  // fallback: native fetch（开发模式 / 非 Wails 环境）
  const resp = await fetch(url, { signal: AbortSignal.timeout(8000) });
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  return resp.text();
}

/** 读取当前 Clash mixed-port */
async function getMixedPort(): Promise<number> {
  try {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    let ConfigAPI: any = null;
    try {
      ConfigAPI = await import("../../wailsjs/go/api/ConfigAPI");
    } catch { /* stub */ }
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const info: any = await ConfigAPI?.GetClashInfo?.();
    return info?.mixed_port || info?.port || 7897;
  } catch {
    return 7897;
  }
}

/** 获取当前 IP 和地理位置信息（通过代理）*/
export const getIpInfo = async (): Promise<
  IpInfo & { lastFetchTs: number }
> => {
  const maxRetries = 2;
  const proxyPort = await getMixedPort();

  const shuffledServices = IP_CHECK_SERVICES.toSorted(
    () => Math.random() - 0.5,
  );
  let lastError: unknown | null = null;

  for (const service of shuffledServices) {
    debugLog(`尝试IP检测服务: ${service.url}`);

    try {
      return await asyncRetry(
        async (bail) => {
          console.debug("Fetching IP information via Go proxy:", service.url);

          let body: string;
          try {
            body = await fetchViaGoProxy(service.url, proxyPort);
          } catch (err) {
            throw err;
          }

          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          let data: any;
          try {
            data = JSON.parse(body);
          } catch {
            return bail(new Error(`无法解析 JSON 响应 from ${service.url}`));
          }

          if (data && data.ip) {
            debugLog(`IP检测成功，使用服务: ${service.url}`);
            return Object.assign(service.mapping(data), {
              lastFetchTs: Date.now(),
            });
          } else {
            return bail(new Error(`无效的响应格式 from ${service.url}`));
          }
        },
        {
          retries: maxRetries,
          minTimeout: 1000,
          maxTimeout: 4000,
          randomize: true,
        },
      );
    } catch (error) {
      debugLog(`IP检测服务失败: ${service.url}`, error);
      lastError = error;
    }
  }

  if (lastError) {
    throw new Error(
      `所有IP检测服务都失败: ${extractErrorMessage(lastError) || "未知错误"}`,
    );
  } else {
    throw new Error("没有可用的IP检测服务");
  }
};
