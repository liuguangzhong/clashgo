package config

// EmbeddedMihomoVersion Mihomo 嵌入库版本（构建时注入）
// 编译时通过 -ldflags "-X clashgo/internal/config.EmbeddedMihomoVersion=v1.19.20" 设置
var EmbeddedMihomoVersion = "v1.19.20"
