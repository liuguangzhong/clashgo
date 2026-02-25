export namespace api {
	
	export class CreateProfileRequest {
	    type: string;
	    name: string;
	    desc?: string;
	    url?: string;
	    content?: string;
	    option?: config.ProfileOption;
	
	    static createFrom(source: any = {}) {
	        return new CreateProfileRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.name = source["name"];
	        this.desc = source["desc"];
	        this.url = source["url"];
	        this.content = source["content"];
	        this.option = this.convertValues(source["option"], config.ProfileOption);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IOCounter {
	    name: string;
	    bytes_sent: number;
	    bytes_recv: number;
	    packets_sent: number;
	    packets_recv: number;
	
	    static createFrom(source: any = {}) {
	        return new IOCounter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.bytes_sent = source["bytes_sent"];
	        this.bytes_recv = source["bytes_recv"];
	        this.packets_sent = source["packets_sent"];
	        this.packets_recv = source["packets_recv"];
	    }
	}
	export class ImportProfileRequest {
	    url: string;
	    name?: string;
	    option?: config.ProfileOption;
	
	    static createFrom(source: any = {}) {
	        return new ImportProfileRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.name = source["name"];
	        this.option = this.convertValues(source["option"], config.ProfileOption);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NetworkInterface {
	    name: string;
	    index: number;
	    mtu: number;
	    flags: string;
	    addresses: string[];
	
	    static createFrom(source: any = {}) {
	        return new NetworkInterface(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.index = source["index"];
	        this.mtu = source["mtu"];
	        this.flags = source["flags"];
	        this.addresses = source["addresses"];
	    }
	}
	export class SystemInfo {
	    cpu: number;
	    cpu_count: number;
	    mem_total: number;
	    mem_used: number;
	    mem_avail: number;
	    mem_percent: number;
	    os: string;
	    arch: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cpu = source["cpu"];
	        this.cpu_count = source["cpu_count"];
	        this.mem_total = source["mem_total"];
	        this.mem_used = source["mem_used"];
	        this.mem_avail = source["mem_avail"];
	        this.mem_percent = source["mem_percent"];
	        this.os = source["os"];
	        this.arch = source["arch"];
	    }
	}

}

export namespace backup {
	
	export class BackupInfo {
	    filename: string;
	    size: number;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new BackupInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filename = source["filename"];
	        this.size = source["size"];
	        this.created_at = this.convertValues(source["created_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RemoteBackupInfo {
	    filename: string;
	    size: number;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new RemoteBackupInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filename = source["filename"];
	        this.size = source["size"];
	        this.created_at = this.convertValues(source["created_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace config {
	
	export class ClashInfo {
	    mixed_port: number;
	    socks_port: number;
	    port: number;
	    server: string;
	    secret?: string;
	
	    static createFrom(source: any = {}) {
	        return new ClashInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mixed_port = source["mixed_port"];
	        this.socks_port = source["socks_port"];
	        this.port = source["port"];
	        this.server = source["server"];
	        this.secret = source["secret"];
	    }
	}
	export class ProfileOption {
	    user_agent?: string;
	    with_proxy?: boolean;
	    self_proxy?: boolean;
	    update_interval?: number;
	
	    static createFrom(source: any = {}) {
	        return new ProfileOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user_agent = source["user_agent"];
	        this.with_proxy = source["with_proxy"];
	        this.self_proxy = source["self_proxy"];
	        this.update_interval = source["update_interval"];
	    }
	}
	export class ProfileExtra {
	    upload: number;
	    download: number;
	    total: number;
	    expire: number;
	
	    static createFrom(source: any = {}) {
	        return new ProfileExtra(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.upload = source["upload"];
	        this.download = source["download"];
	        this.total = source["total"];
	        this.expire = source["expire"];
	    }
	}
	export class Selected {
	    name: string;
	    now: string;
	
	    static createFrom(source: any = {}) {
	        return new Selected(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.now = source["now"];
	    }
	}
	export class IProfile {
	    uid?: string;
	    type?: string;
	    name?: string;
	    desc?: string;
	    file?: string;
	    url?: string;
	    selected?: Selected[];
	    extra?: ProfileExtra;
	    // Go type: time
	    updated_at?: any;
	    interval?: number;
	    option?: ProfileOption;
	    current_merge?: string;
	    current_script?: string;
	    current_rules?: string;
	    current_proxies?: string;
	    current_groups?: string;
	
	    static createFrom(source: any = {}) {
	        return new IProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uid = source["uid"];
	        this.type = source["type"];
	        this.name = source["name"];
	        this.desc = source["desc"];
	        this.file = source["file"];
	        this.url = source["url"];
	        this.selected = this.convertValues(source["selected"], Selected);
	        this.extra = this.convertValues(source["extra"], ProfileExtra);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.interval = source["interval"];
	        this.option = this.convertValues(source["option"], ProfileOption);
	        this.current_merge = source["current_merge"];
	        this.current_script = source["current_script"];
	        this.current_rules = source["current_rules"];
	        this.current_proxies = source["current_proxies"];
	        this.current_groups = source["current_groups"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IProfiles {
	    current?: string;
	    items?: IProfile[];
	
	    static createFrom(source: any = {}) {
	        return new IProfiles(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.items = this.convertValues(source["items"], IProfile);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IVergeTheme {
	    primary_color?: string;
	    secondary_color?: string;
	    primary_text?: string;
	    secondary_text?: string;
	    info_color?: string;
	    error_color?: string;
	    warning_color?: string;
	    success_color?: string;
	    font_family?: string;
	    css_injection?: string;
	
	    static createFrom(source: any = {}) {
	        return new IVergeTheme(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.primary_color = source["primary_color"];
	        this.secondary_color = source["secondary_color"];
	        this.primary_text = source["primary_text"];
	        this.secondary_text = source["secondary_text"];
	        this.info_color = source["info_color"];
	        this.error_color = source["error_color"];
	        this.warning_color = source["warning_color"];
	        this.success_color = source["success_color"];
	        this.font_family = source["font_family"];
	        this.css_injection = source["css_injection"];
	    }
	}
	export class IVerge {
	    app_log_level?: string;
	    app_log_max_size?: number;
	    app_log_max_count?: number;
	    language?: string;
	    theme_mode?: string;
	    collapse_navbar?: boolean;
	    start_page?: string;
	    notice_position?: string;
	    menu_icon?: string;
	    enable_system_proxy?: boolean;
	    proxy_auto_config?: boolean;
	    pac_file_content?: string;
	    proxy_host?: string;
	    system_proxy_bypass?: string;
	    use_default_bypass?: boolean;
	    enable_proxy_guard?: boolean;
	    proxy_guard_duration?: number;
	    enable_bypass_check?: boolean;
	    enable_tun_mode?: boolean;
	    verge_mixed_port?: number;
	    verge_socks_port?: number;
	    verge_socks_enabled?: boolean;
	    verge_port?: number;
	    verge_http_enabled?: boolean;
	    verge_redir_port?: number;
	    verge_redir_enabled?: boolean;
	    verge_tproxy_port?: number;
	    verge_tproxy_enabled?: boolean;
	    enable_auto_launch?: boolean;
	    enable_silent_start?: boolean;
	    startup_script?: string;
	    clash_core?: string;
	    hotkeys?: string[];
	    enable_global_hotkey?: boolean;
	    auto_close_connection?: boolean;
	    auto_check_update?: boolean;
	    default_latency_test?: string;
	    default_latency_timeout?: number;
	    enable_builtin_enhanced?: boolean;
	    auto_log_clean?: number;
	    enable_auto_backup_schedule?: boolean;
	    auto_backup_interval_hours?: number;
	    auto_backup_on_change?: boolean;
	    webdav_url?: string;
	    webdav_username?: string;
	    webdav_password?: string;
	    sysproxy_tray_icon?: boolean;
	    tun_tray_icon?: boolean;
	    tray_proxy_groups_display_mode?: string;
	    tray_inline_outbound_modes?: boolean;
	    enable_auto_light_weight_mode?: boolean;
	    auto_light_weight_minutes?: number;
	    enable_dns_settings?: boolean;
	    enable_external_controller?: boolean;
	    theme_setting?: IVergeTheme;
	    proxy_layout_column?: number;
	    home_cards?: any;
	
	    static createFrom(source: any = {}) {
	        return new IVerge(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.app_log_level = source["app_log_level"];
	        this.app_log_max_size = source["app_log_max_size"];
	        this.app_log_max_count = source["app_log_max_count"];
	        this.language = source["language"];
	        this.theme_mode = source["theme_mode"];
	        this.collapse_navbar = source["collapse_navbar"];
	        this.start_page = source["start_page"];
	        this.notice_position = source["notice_position"];
	        this.menu_icon = source["menu_icon"];
	        this.enable_system_proxy = source["enable_system_proxy"];
	        this.proxy_auto_config = source["proxy_auto_config"];
	        this.pac_file_content = source["pac_file_content"];
	        this.proxy_host = source["proxy_host"];
	        this.system_proxy_bypass = source["system_proxy_bypass"];
	        this.use_default_bypass = source["use_default_bypass"];
	        this.enable_proxy_guard = source["enable_proxy_guard"];
	        this.proxy_guard_duration = source["proxy_guard_duration"];
	        this.enable_bypass_check = source["enable_bypass_check"];
	        this.enable_tun_mode = source["enable_tun_mode"];
	        this.verge_mixed_port = source["verge_mixed_port"];
	        this.verge_socks_port = source["verge_socks_port"];
	        this.verge_socks_enabled = source["verge_socks_enabled"];
	        this.verge_port = source["verge_port"];
	        this.verge_http_enabled = source["verge_http_enabled"];
	        this.verge_redir_port = source["verge_redir_port"];
	        this.verge_redir_enabled = source["verge_redir_enabled"];
	        this.verge_tproxy_port = source["verge_tproxy_port"];
	        this.verge_tproxy_enabled = source["verge_tproxy_enabled"];
	        this.enable_auto_launch = source["enable_auto_launch"];
	        this.enable_silent_start = source["enable_silent_start"];
	        this.startup_script = source["startup_script"];
	        this.clash_core = source["clash_core"];
	        this.hotkeys = source["hotkeys"];
	        this.enable_global_hotkey = source["enable_global_hotkey"];
	        this.auto_close_connection = source["auto_close_connection"];
	        this.auto_check_update = source["auto_check_update"];
	        this.default_latency_test = source["default_latency_test"];
	        this.default_latency_timeout = source["default_latency_timeout"];
	        this.enable_builtin_enhanced = source["enable_builtin_enhanced"];
	        this.auto_log_clean = source["auto_log_clean"];
	        this.enable_auto_backup_schedule = source["enable_auto_backup_schedule"];
	        this.auto_backup_interval_hours = source["auto_backup_interval_hours"];
	        this.auto_backup_on_change = source["auto_backup_on_change"];
	        this.webdav_url = source["webdav_url"];
	        this.webdav_username = source["webdav_username"];
	        this.webdav_password = source["webdav_password"];
	        this.sysproxy_tray_icon = source["sysproxy_tray_icon"];
	        this.tun_tray_icon = source["tun_tray_icon"];
	        this.tray_proxy_groups_display_mode = source["tray_proxy_groups_display_mode"];
	        this.tray_inline_outbound_modes = source["tray_inline_outbound_modes"];
	        this.enable_auto_light_weight_mode = source["enable_auto_light_weight_mode"];
	        this.auto_light_weight_minutes = source["auto_light_weight_minutes"];
	        this.enable_dns_settings = source["enable_dns_settings"];
	        this.enable_external_controller = source["enable_external_controller"];
	        this.theme_setting = this.convertValues(source["theme_setting"], IVergeTheme);
	        this.proxy_layout_column = source["proxy_layout_column"];
	        this.home_cards = source["home_cards"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Manager {
	
	
	    static createFrom(source: any = {}) {
	        return new Manager(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	
	
	export class RuntimeSnapshot {
	    config: Record<string, any>;
	    exists_keys: Record<string, boolean>;
	    chain_logs: Record<string, Array<Array<string>>>;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = source["config"];
	        this.exists_keys = source["exists_keys"];
	        this.chain_logs = source["chain_logs"];
	    }
	}

}

export namespace mihomo {
	
	export class ConnectionMeta {
	    network: string;
	    type: string;
	    host: string;
	    sourceIP: string;
	    sourcePort: string;
	    remoteAddr: string;
	    dnsMode: string;
	    processPath: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.network = source["network"];
	        this.type = source["type"];
	        this.host = source["host"];
	        this.sourceIP = source["sourceIP"];
	        this.sourcePort = source["sourcePort"];
	        this.remoteAddr = source["remoteAddr"];
	        this.dnsMode = source["dnsMode"];
	        this.processPath = source["processPath"];
	    }
	}
	export class Connection {
	    id: string;
	    metadata: ConnectionMeta;
	    upload: number;
	    download: number;
	    // Go type: time
	    start: any;
	    chains: string[];
	    rule: string;
	    rulePayload: string;
	
	    static createFrom(source: any = {}) {
	        return new Connection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.metadata = this.convertValues(source["metadata"], ConnectionMeta);
	        this.upload = source["upload"];
	        this.download = source["download"];
	        this.start = this.convertValues(source["start"], null);
	        this.chains = source["chains"];
	        this.rule = source["rule"];
	        this.rulePayload = source["rulePayload"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ConnectionsResponse {
	    downloadTotal: number;
	    uploadTotal: number;
	    connections: Connection[];
	
	    static createFrom(source: any = {}) {
	        return new ConnectionsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.downloadTotal = source["downloadTotal"];
	        this.uploadTotal = source["uploadTotal"];
	        this.connections = this.convertValues(source["connections"], Connection);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Delay {
	    time: string;
	    delay: number;
	
	    static createFrom(source: any = {}) {
	        return new Delay(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.delay = source["delay"];
	    }
	}
	export class Proxy {
	    name: string;
	    type: string;
	    alive: boolean;
	    history: Delay[];
	    now?: string;
	    all?: string[];
	    udp: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Proxy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.alive = source["alive"];
	        this.history = this.convertValues(source["history"], Delay);
	        this.now = source["now"];
	        this.all = source["all"];
	        this.udp = source["udp"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Provider {
	    name: string;
	    type: string;
	    vehicleType: string;
	    proxies: Proxy[];
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Provider(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.vehicleType = source["vehicleType"];
	        this.proxies = this.convertValues(source["proxies"], Proxy);
	        this.updatedAt = source["updatedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProvidersResponse {
	    providers: Record<string, Provider>;
	
	    static createFrom(source: any = {}) {
	        return new ProvidersResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providers = this.convertValues(source["providers"], Provider, true);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProxiesResponse {
	    proxies: Record<string, Proxy>;
	
	    static createFrom(source: any = {}) {
	        return new ProxiesResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.proxies = this.convertValues(source["proxies"], Proxy, true);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Rule {
	    type: string;
	    payload: string;
	    proxy: string;
	
	    static createFrom(source: any = {}) {
	        return new Rule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.payload = source["payload"];
	        this.proxy = source["proxy"];
	    }
	}
	export class RulesResponse {
	    rules: Rule[];
	
	    static createFrom(source: any = {}) {
	        return new RulesResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rules = this.convertValues(source["rules"], Rule);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TrafficStats {
	    up: number;
	    down: number;
	
	    static createFrom(source: any = {}) {
	        return new TrafficStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.up = source["up"];
	        this.down = source["down"];
	    }
	}

}

export namespace proxy {
	
	export class ProxyInfo {
	    enabled: boolean;
	    host: string;
	    port: number;
	    bypass: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.bypass = source["bypass"];
	    }
	}

}

export namespace service {
	
	export class ServiceStatus {
	    available: boolean;
	    running: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ServiceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.running = source["running"];
	        this.error = source["error"];
	    }
	}

}

export namespace unlock {
	
	export class UnlockItem {
	    name: string;
	    status: string;
	    region?: string;
	    check_time?: string;
	
	    static createFrom(source: any = {}) {
	        return new UnlockItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.region = source["region"];
	        this.check_time = source["check_time"];
	    }
	}

}

