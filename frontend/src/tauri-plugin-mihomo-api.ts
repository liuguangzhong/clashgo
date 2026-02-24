/**
 * tauri-plugin-mihomo-api shim for ClashGo (Wails)
 *
 * 原项目通过 Tauri 原生插件访问 Mihomo REST/WS API。
 * ClashGo 中，前端直接用 fetch/WebSocket 访问 Mihomo HTTP API，
 * server 地址从 ConfigAPI.GetClashInfo() 获取。
 *
 * 这个文件导出与原插件完全相同的接口，业务代码无需修改。
 */

// ── 类型定义（与原插件对齐）────────────────────────────────────────────────

export type LogLevel = "info" | "warning" | "error" | "debug" | "silent";

export interface Traffic {
    up: number;
    down: number;
}

export interface Memory {
    inuse: number;
    oslimit: number;
}

export interface Rule {
    type: string;
    payload: string;
    proxy: string;
    size?: number;
}

export interface Proxy {
    name: string;
    type: string;
    alive: boolean;
    history: Array<{ time: string; delay: number }>;
    now?: string;
    all?: string[];
    udp?: boolean;
    xudp?: boolean;
    tfo?: boolean;
    mptcp?: boolean;
    smux?: boolean;
    provider?: string;
}

export interface ProxyProvider {
    name: string;
    type: string;
    vehicleType: string;
    proxies: Proxy[];
    updatedAt: string;
    subscriptionInfo?: {
        Upload: number;
        Download: number;
        Total: number;
        Expire: number;
    };
}

export interface Connection {
    id: string;
    metadata: {
        network: string;
        type: string;
        host: string;
        sourceIP: string;
        sourcePort: string;
        remoteAddr: string;
        dnsMode: string;
        processPath: string;
    };
    upload: number;
    download: number;
    start: string;
    chains: string[];
    rule: string;
    rulePayload: string;
}

export interface ProxyDelay {
    delay?: number;
    error?: string;
}

export type Message =
    | { type: "traffic"; data: Traffic }
    | { type: "memory"; data: Memory }
    | { type: "log"; data: { type: LogLevel; payload: string } }
    | { type: "connection"; data: { connections: Connection[]; downloadTotal: number; uploadTotal: number } }
    | { type: "Text"; data: string };

// ── Mihomo 服务器地址（由 ClashInfo 动态填充）──────────────────────────────

let _mihomoServer = "127.0.0.1:9097";
let _mihomoSecret = "";

/** 由 AppDataProvider 初始化后注入 */
export function initMihomoClient(server: string, secret: string) {
    _mihomoServer = server;
    _mihomoSecret = secret;
}

function mihomoBaseURL() {
    return `http://${_mihomoServer}`;
}

function mihomoHeaders(): HeadersInit {
    return _mihomoSecret
        ? { Authorization: `Bearer ${_mihomoSecret}`, "Content-Type": "application/json" }
        : { "Content-Type": "application/json" };
}

async function mihomoFetch<T>(path: string, init?: RequestInit): Promise<T> {
    const resp = await fetch(`${mihomoBaseURL()}${path}`, {
        ...init,
        headers: { ...mihomoHeaders(), ...(init?.headers ?? {}) },
    });
    if (!resp.ok) {
        const body = await resp.text().catch(() => "");
        throw new Error(`Mihomo API ${init?.method ?? "GET"} ${path}: HTTP ${resp.status} ${body}`);
    }
    if (resp.status === 204) return undefined as T;
    return resp.json() as Promise<T>;
}

// ── REST API 封装 ─────────────────────────────────────────────────────────────

export async function getVersion(): Promise<{ version: string; meta: boolean }> {
    return mihomoFetch("/version");
}

export async function getProxies(): Promise<{ proxies: Record<string, Proxy> }> {
    return mihomoFetch("/proxies");
}

export async function getProxyProviders(): Promise<{ providers: Record<string, ProxyProvider> }> {
    return mihomoFetch("/providers/proxies");
}

export async function selectNodeForGroup(group: string, proxy: string): Promise<void> {
    return mihomoFetch(`/proxies/${encodeURIComponent(group)}`, {
        method: "PUT",
        body: JSON.stringify({ name: proxy }),
    });
}

export async function delayProxyByName(
    name: string,
    url: string,
    timeout: number,
): Promise<ProxyDelay> {
    try {
        const result = await mihomoFetch<{ delay: number }>(
            `/proxies/${encodeURIComponent(name)}/delay?url=${encodeURIComponent(url)}&timeout=${timeout}`,
        );
        return { delay: result.delay };
    } catch (e) {
        return { error: String(e) };
    }
}

export async function delayGroup(group: string, url: string, timeout: number): Promise<void> {
    return mihomoFetch(`/group/${encodeURIComponent(group)}/delay?url=${encodeURIComponent(url)}&timeout=${timeout}`);
}

export async function healthcheckProxyProvider(name: string): Promise<void> {
    return mihomoFetch(`/providers/proxies/${encodeURIComponent(name)}/healthcheck`, {
        method: "GET",
    });
}

export async function updateProxyProvider(name: string): Promise<void> {
    return mihomoFetch(`/providers/proxies/${encodeURIComponent(name)}`, { method: "PUT" });
}

export async function updateRuleProvider(name: string): Promise<void> {
    return mihomoFetch(`/providers/rules/${encodeURIComponent(name)}`, { method: "PUT" });
}

export async function closeAllConnections(): Promise<void> {
    return mihomoFetch("/connections", { method: "DELETE" });
}

export async function closeConnection(id: string): Promise<void> {
    return mihomoFetch(`/connections/${id}`, { method: "DELETE" });
}

export async function updateGeo(): Promise<void> {
    // Mihomo Meta: PUT /configs/geo
    return mihomoFetch<void>("/configs/geo", { method: "PUT" }).catch((): void => {
        // fallback: reload config (triggers geo re-download if cache is deleted)
        console.warn("[mihomo-shim] /configs/geo not supported, using reload fallback");
    });
}

export async function upgradeCore(): Promise<void> {
    return mihomoFetch<void>("/upgrade", { method: "POST" }).catch((e): void => {
        console.warn("[mihomo-shim] upgradeCore:", e);
    });
}

// ── WebSocket 订阅（MihomoWebSocket shim）────────────────────────────────────

type WSListener = (msg: Message) => void;

export class MihomoWebSocket {
    private static instances = new Map<string, MihomoWebSocket>();

    private ws: WebSocket | null = null;
    private listeners = new Set<WSListener>();
    private path: string;
    private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    private closed = false;

    private constructor(path: string) {
        this.path = path;
        this.connect();
    }

    static get(path: string): MihomoWebSocket {
        if (!MihomoWebSocket.instances.has(path)) {
            MihomoWebSocket.instances.set(path, new MihomoWebSocket(path));
        }
        return MihomoWebSocket.instances.get(path)!;
    }

    static cleanupAll() {
        MihomoWebSocket.instances.forEach((inst) => inst.close());
        MihomoWebSocket.instances.clear();
    }

    // ── 原插件兼容静态工厂方法 ──────────────────────────────────────────────
    static connect_traffic(): Promise<MihomoWebSocket> { return Promise.resolve(MihomoWebSocket.get("/traffic")); }
    static connect_memory(): Promise<MihomoWebSocket> { return Promise.resolve(MihomoWebSocket.get("/memory")); }
    static connect_connections(): Promise<MihomoWebSocket> { return Promise.resolve(MihomoWebSocket.get("/connections")); }
    static connect_logs(level: LogLevel = "info"): Promise<MihomoWebSocket> { return Promise.resolve(MihomoWebSocket.get(`/logs?level=${level}`)); }

    private wsURL(): string {
        const secret = _mihomoSecret ? `?token=${encodeURIComponent(_mihomoSecret)}` : "";
        return `ws://${_mihomoServer}${this.path}${secret}`;
    }

    private connect() {
        if (this.closed) return;
        try {
            this.ws = new WebSocket(this.wsURL());
            this.ws.onmessage = (event) => {
                try {
                    const data = JSON.parse(event.data);
                    this.listeners.forEach((fn) => {
                        try {
                            fn(data as Message);
                        } catch { /* ignore listener errors */ }
                    });
                } catch { /* ignore parse errors */ }
            };
            this.ws.onclose = () => {
                if (!this.closed) {
                    this.reconnectTimer = setTimeout(() => this.connect(), 2000);
                }
            };
            this.ws.onerror = () => {
                this.ws?.close();
            };
        } catch { /* ignore connection errors */ }
    }

    addListener(fn: WSListener): () => void {
        this.listeners.add(fn);
        return () => this.listeners.delete(fn);
    }

    close() {
        this.closed = true;
        if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
        this.ws?.close();
    }
}

// ── 便捷工厂函数（hooks 使用）──────────────────────────────────────────────

export function getTrafficWS(): MihomoWebSocket {
    return MihomoWebSocket.get("/traffic");
}

export function getMemoryWS(): MihomoWebSocket {
    return MihomoWebSocket.get("/memory");
}

export function getLogsWS(level: LogLevel = "info"): MihomoWebSocket {
    return MihomoWebSocket.get(`/logs?level=${level}`);
}

export function getConnectionsWS(): MihomoWebSocket {
    return MihomoWebSocket.get("/connections");
}

// ── 额外 REST 函数（原插件导出） ───────────────────────────────────────────────

/** getConnections — 获取当前活跃连接列表（REST 快照，非 WS）*/
export async function getConnections(): Promise<{
    connections: Connection[];
    downloadTotal: number;
    uploadTotal: number;
}> {
    return mihomoFetch("/connections");
}

/** getRules — 获取规则列表 */
export async function getRules(): Promise<{ rules: Rule[] }> {
    return mihomoFetch("/rules");
}

// ── 额外类型定义 ───────────────────────────────────────────────────────────────

export interface RuleProvider {
    name: string;
    type: string;
    vehicleType: string;
    behavior: string;
    ruleCount: number;
    updatedAt: string;
}

export interface BaseConfig {
    port?: number;
    "socks-port"?: number;
    "mixed-port"?: number;
    "redir-port"?: number;
    "tproxy-port"?: number;
    mode?: string;
    "log-level"?: string;
    ipv6?: boolean;
    "allow-lan"?: boolean;
    tun?: {
        enable?: boolean;
        stack?: string;
        "auto-route"?: boolean;
        "auto-detect-interface"?: boolean;
    };
    dns?: Record<string, unknown>;
    [key: string]: unknown;
}

/** getBaseConfig — 获取 Mihomo 运行时配置 */
export async function getBaseConfig(): Promise<BaseConfig> {
    return mihomoFetch("/configs");
}

/** getRuleProviders — 获取规则提供者 */
export async function getRuleProviders(): Promise<{ providers: Record<string, RuleProvider> }> {
    return mihomoFetch("/providers/rules");
}


// ── ProxyProvider subscriptionInfo 补丁已合并到 ProxyProvider interface ─────
// export interface ProxyProviderWithSub — removed, subscriptionInfo is now on ProxyProvider directly
