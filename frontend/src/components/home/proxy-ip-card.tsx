import {
    VpnLockOutlined,
    RefreshOutlined,
} from "@mui/icons-material";
import { Box, IconButton, Skeleton, Typography } from "@mui/material";
import { memo, useCallback, useState } from "react";

import { EnhancedCard } from "./enhanced-card";

// 通过代理获取IP的服务列表
const PROXY_IP_SERVICES = [
    "https://api.ipify.org?format=json",
    "https://ipinfo.io/json",
    "https://api.ip.sb/geoip",
];

interface ProxyIpData {
    ip: string;
    country?: string;
    country_code?: string;
    city?: string;
    org?: string;
}

// 获取国旗表情
const getCountryFlag = (countryCode: string | undefined) => {
    if (!countryCode) return "🌐";
    const codePoints = countryCode
        .toUpperCase()
        .split("")
        .map((char) => 127397 + char.charCodeAt(0));
    return String.fromCodePoint(...codePoints);
};

const InfoRow = memo(({ label, value }: { label: string; value?: string }) => (
    <Box sx={{ display: "flex", alignItems: "center", mb: 0.5 }}>
        <Typography
            variant="body2"
            color="text.secondary"
            sx={{ minWidth: 50, mr: 0.5, flexShrink: 0 }}
        >
            {label}:
        </Typography>
        <Typography
            variant="body2"
            sx={{
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
            }}
        >
            {value || "N/A"}
        </Typography>
    </Box>
));

// 通过 Mihomo 代理端口获取出口 IP
async function fetchProxyIp(): Promise<ProxyIpData> {
    // 获取 clash info 拿到 mixed-port
    let proxyPort = 7897;
    try {
        const { getClashInfo } = await import("@/services/cmds");
        const info = await getClashInfo();
        if (info?.mixed_port) {
            proxyPort = info.mixed_port;
        } else if (info?.port) {
            proxyPort = info.port;
        }
    } catch {
        // fallback to default
    }

    // 通过代理获取IP（使用 fetch + proxy）
    for (const url of PROXY_IP_SERVICES) {
        try {
            const controller = new AbortController();
            const timeout = setTimeout(() => controller.abort(), 5000);

            const response = await fetch(url, {
                signal: controller.signal,
            });
            clearTimeout(timeout);

            if (!response.ok) continue;

            const data = await response.json();

            // 适配不同API的返回格式
            return {
                ip: data.ip || data.query || "Unknown",
                country: data.country || data.country_name || undefined,
                country_code: data.country_code || data.countryCode || undefined,
                city: data.city || undefined,
                org: data.org || data.organization || data.isp || undefined,
            };
        } catch {
            continue;
        }
    }

    throw new Error("无法获取代理IP");
}

// 代理IP信息卡片
export const ProxyIpCard = () => {
    const [loading, setLoading] = useState(false);
    const [data, setData] = useState<ProxyIpData | null>(null);
    const [error, setError] = useState<string | null>(null);

    const refresh = useCallback(async () => {
        setLoading(true);
        setError(null);
        try {
            const result = await fetchProxyIp();
            setData(result);
        } catch (e: any) {
            setError(e.message || "获取失败");
        } finally {
            setLoading(false);
        }
    }, []);

    // 首次自动获取
    useState(() => {
        refresh();
    });

    return (
        <EnhancedCard
            title="代理 IP"
            icon={<VpnLockOutlined />}
            iconColor="success"
            action={
                <IconButton size="small" onClick={refresh} disabled={loading}>
                    <RefreshOutlined />
                </IconButton>
            }
        >
            {loading ? (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                    <Skeleton variant="text" width="60%" height={30} />
                    <Skeleton variant="text" width="80%" height={24} />
                    <Skeleton variant="text" width="50%" height={24} />
                </Box>
            ) : error ? (
                <Box sx={{ textAlign: "center", py: 2 }}>
                    <Typography variant="body2" color="error">
                        {error}
                    </Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: "block" }}>
                        请先开启系统代理
                    </Typography>
                </Box>
            ) : data ? (
                <Box>
                    <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
                        <Box
                            component="span"
                            sx={{
                                fontSize: "1.5rem",
                                mr: 1,
                                fontFamily: '"twemoji mozilla", sans-serif',
                            }}
                        >
                            {getCountryFlag(data.country_code)}
                        </Box>
                        <Typography variant="subtitle1" sx={{ fontWeight: "medium" }}>
                            {data.country || "Unknown"}
                        </Typography>
                    </Box>
                    <InfoRow
                        label="IP"
                        value={data.ip}
                    />
                    <InfoRow
                        label="城市"
                        value={data.city}
                    />
                    <InfoRow
                        label="组织"
                        value={data.org}
                    />
                    <Box
                        sx={{
                            mt: 1,
                            pt: 0.5,
                            borderTop: 1,
                            borderColor: "divider",
                            opacity: 0.7,
                        }}
                    >
                        <Typography variant="caption" color={
                            data.country_code && data.country_code !== "CN"
                                ? "success.main"
                                : "warning.main"
                        }>
                            {data.country_code && data.country_code !== "CN"
                                ? `✅ 代理生效`
                                : `⚠️ 代理未生效（仍为国内IP）`}
                        </Typography>
                    </Box>
                </Box>
            ) : null}
        </EnhancedCard>
    );
};
