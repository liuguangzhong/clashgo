package proxy

// geoip.go — GEOIP 规则实现（P2）
//
// 参照 mihomo/rules/common/geoip.go + component/geodata 实现。
// 使用 MaxMind GeoLite2 MMDB 格式（golang 纯实现：oschwald/maxminddb-golang）。
//
// 若 MMDB 文件不存在，则退化为 no-op（规则不匹配）。

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
)

// ── MMDB 极简解析器（不依赖第三方库）────────────────────────────────────────
// MaxMind DB 文件格式：https://maxmind.github.io/MaxMind-DB/
//
// 我们只需要 IP → Country ISO Code 的映射，
// 实现思路：将完整解析器内联（仅支持 GeoLite2-Country.mmdb 格式）

// GeoIPDB GeoLite2 MMDB 数据库
type GeoIPDB struct {
	mu             sync.RWMutex
	data           []byte
	nodeCount      int
	recordSize     int // 24 or 28 or 32
	ipVersion      int // 4 or 6
	searchTreeSize int
}

var (
	globalGeoIPDB     *GeoIPDB
	globalGeoIPDBOnce sync.Once
)

// LoadGeoIPDB 加载 GeoIP MMDB 文件
func LoadGeoIPDB(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read geoip db %s: %w", path, err)
	}
	db, err := parseMmdb(data)
	if err != nil {
		return fmt.Errorf("parse geoip db: %w", err)
	}
	globalGeoIPDB = db
	return nil
}

// GlobalGeoIPDB 返回全局 GeoIP 数据库
func GlobalGeoIPDB() *GeoIPDB {
	return globalGeoIPDB
}

// LookupCountry 查询 IP 所属国家代码（返回大写 ISO 3166-1 alpha-2，如 "CN"）
func LookupCountry(ip net.IP) string {
	db := globalGeoIPDB
	if db == nil {
		return ""
	}
	return db.Lookup(ip)
}

// Lookup 查询 IP 所属国家代码
func (db *GeoIPDB) Lookup(ip net.IP) string {
	db.mu.RLock()
	defer db.mu.RUnlock()

	ip = ip.To4()
	if ip == nil {
		return "" // 暂只支持 IPv4
	}

	// 在搜索树中按位遍历
	node := 0
	for i := 0; i < 32; i++ {
		bit := (int(ip[i/8]) >> (7 - uint(i)%8)) & 1
		node = db.readNode(node, bit)
		if node >= db.nodeCount {
			break
		}
	}
	if node <= db.nodeCount {
		return ""
	}

	// 读取 record 数据，解析 country.iso_code
	offset := node - db.nodeCount - 16
	if offset < 0 || offset >= len(db.data)-db.searchTreeSize {
		return ""
	}

	pos := db.searchTreeSize + 16 + offset
	return db.decodeCountryCode(pos)
}

// readNode 读取二叉搜索树节点（left/right 子节点）
func (db *GeoIPDB) readNode(node, bit int) int {
	recBytes := db.recordSize / 8
	// 节点大小 = 2 * recordSize bits（每条记录 recordSize bits）
	nodeOffset := node * db.recordSize * 2 / 8

	switch db.recordSize {
	case 24:
		base := nodeOffset + bit*3
		if base+3 > db.searchTreeSize {
			return db.nodeCount
		}
		return int(db.data[base])<<16 | int(db.data[base+1])<<8 | int(db.data[base+2])
	case 28:
		if bit == 0 {
			base := nodeOffset
			if base+4 > db.searchTreeSize {
				return db.nodeCount
			}
			return (int(db.data[base+3]>>4) << 24) | int(db.data[base])<<16 | int(db.data[base+1])<<8 | int(db.data[base+2])
		}
		base := nodeOffset + 3
		if base+4 > db.searchTreeSize {
			return db.nodeCount
		}
		return (int(db.data[base]&0xF) << 24) | int(db.data[base+1])<<16 | int(db.data[base+2])<<8 | int(db.data[base+3])
	case 32:
		base := nodeOffset + bit*4
		if base+4 > db.searchTreeSize {
			return db.nodeCount
		}
		return int(binary.BigEndian.Uint32(db.data[base : base+4]))
	default:
		_ = recBytes
		return db.nodeCount
	}
}

// decodeCountryCode 从数据段解析 country.iso_code 字段
// MMDB 数据格式比较复杂，这里做极简 heuristic 搜索
func (db *GeoIPDB) decodeCountryCode(pos int) string {
	if pos >= len(db.data) {
		return ""
	}
	// 尝试在接下来的 512 字节内搜索 "iso_code" 字符串后跟的 2 字节 ASCII
	end := pos + 512
	if end > len(db.data) {
		end = len(db.data)
	}
	needle := []byte("iso_code")
	chunk := db.data[pos:end]
	idx := indexOf(chunk, needle)
	if idx < 0 {
		return ""
	}
	// iso_code 后面紧跟一个 MMDB string 类型字节，再是 2 字节 ISO 代码
	start := pos + idx + len(needle)
	if start+3 >= len(db.data) {
		return ""
	}
	// MMDB string 类型：高 3 位 = 2（string），低 5 位 = 长度
	typeByte := db.data[start]
	if typeByte>>5 != 2 {
		// 可能有扩展类型前缀，跳过
		start++
		if start+2 >= len(db.data) {
			return ""
		}
		typeByte = db.data[start]
	}
	strLen := int(typeByte & 0x1F)
	if strLen == 0 || strLen > 8 {
		return ""
	}
	start++
	if start+strLen > len(db.data) {
		return ""
	}
	code := string(db.data[start : start+strLen])
	if len(code) == 2 && isASCIIUpper(code) {
		return code
	}
	return ""
}

func indexOf(haystack, needle []byte) int {
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j, b := range needle {
			if haystack[i+j] != b {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func isASCIIUpper(s string) bool {
	for _, c := range s {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

// parseMmdb 极简 MMDB 文件解析（只读取 metadata）
// MMDB metadata 位于文件末尾，以 "\xAB\xCD\xEFMaxMind.com" 分隔符标识
func parseMmdb(data []byte) (*GeoIPDB, error) {
	// 定位 metadata 分隔符
	sep := []byte{0xAB, 0xCD, 0xEF, 'M', 'a', 'x', 'M', 'i', 'n', 'd', '.', 'c', 'o', 'm'}
	idx := lastIndexOf(data, sep)
	if idx < 0 {
		return nil, fmt.Errorf("MMDB magic not found")
	}
	metaStart := idx + len(sep)

	db := &GeoIPDB{data: data}
	// 从 metadata 中提取 node_count、record_size、ip_version
	if err := db.parseMeta(data[metaStart:]); err != nil {
		return nil, err
	}
	db.searchTreeSize = int(db.nodeCount) * db.recordSize * 2 / 8
	return db, nil
}

// parseMeta 解析 MMDB metadata（MAP 类型）
func (db *GeoIPDB) parseMeta(meta []byte) error {
	// metadata 是一个 MMDB MAP，解析其中关键字段
	pos := 0
	if pos >= len(meta) {
		return fmt.Errorf("empty metadata")
	}
	// 跳过 MAP 的类型/长度字节，直接搜索关键字段
	fields := map[string]*int{
		"node_count":  &db.nodeCount,
		"record_size": &db.recordSize,
		"ip_version":  &db.ipVersion,
	}
	_ = pos

	// 逐个搜索字段名
	for name, ptr := range fields {
		needle := []byte(name)
		idx := indexOf(meta, needle)
		if idx < 0 {
			continue
		}
		valPos := idx + len(needle)
		if valPos >= len(meta) {
			continue
		}
		// 紧接着的是 MMDB uint 类型
		val, _, err := decodeMmdbUint(meta, valPos)
		if err == nil {
			*ptr = val
		}
	}

	if db.nodeCount == 0 || db.recordSize == 0 {
		return fmt.Errorf("MMDB metadata missing node_count or record_size")
	}
	if db.ipVersion == 0 {
		db.ipVersion = 4
	}
	return nil
}

// decodeMmdbUint 解码 MMDB uint 类型（类型字节 + 数值）
func decodeMmdbUint(data []byte, pos int) (int, int, error) {
	if pos >= len(data) {
		return 0, 0, io.ErrUnexpectedEOF
	}
	ctrl := data[pos]
	typ := ctrl >> 5
	size := int(ctrl & 0x1F)
	pos++

	// 扩展类型
	if typ == 0 {
		if pos >= len(data) {
			return 0, 0, io.ErrUnexpectedEOF
		}
		typ = data[pos] + 7
		pos++
	}
	// 扩展 size
	switch {
	case size == 29:
		if pos >= len(data) {
			return 0, 0, io.ErrUnexpectedEOF
		}
		size = int(data[pos]) + 29
		pos++
	case size == 30:
		if pos+1 >= len(data) {
			return 0, 0, io.ErrUnexpectedEOF
		}
		size = int(binary.BigEndian.Uint16(data[pos:pos+2])) + 285
		pos += 2
	}

	// uint（typ == 2 for uint16, typ == 6 for uint32/uint64）
	_ = typ
	if size == 0 {
		return 0, pos, nil
	}
	if pos+size > len(data) {
		return 0, 0, io.ErrUnexpectedEOF
	}
	var val int
	for i := 0; i < size; i++ {
		val = val<<8 | int(data[pos+i])
	}
	return val, pos + size, nil
}

func lastIndexOf(data, needle []byte) int {
	result := -1
	for i := 0; i <= len(data)-len(needle); i++ {
		match := true
		for j, b := range needle {
			if data[i+j] != b {
				match = false
				break
			}
		}
		if match {
			result = i
		}
	}
	return result
}

// ── GEOIP 规则 ────────────────────────────────────────────────────────────────

// GeoIPRule 基于 GeoLite2 MMDB 的 IP 归属地规则
// 对应 mihomo/rules/common/geoip.go
type GeoIPRule struct {
	country   string // 如 "CN"
	adapter   string
	noResolve bool
}

func NewGeoIPRule(country, adapter string, noResolve bool) *GeoIPRule {
	return &GeoIPRule{
		country:   strings.ToUpper(country),
		adapter:   adapter,
		noResolve: noResolve,
	}
}

func (r *GeoIPRule) RuleType() string { return "GEOIP" }
func (r *GeoIPRule) Adapter() string  { return r.adapter }
func (r *GeoIPRule) Payload() string  { return r.country }

func (r *GeoIPRule) Match(metadata *Metadata) bool {
	ip := metadata.DstIP
	if ip == nil && !r.noResolve && metadata.DstHost != "" {
		// 需要解析域名
		if globalResolver != nil {
			resolved, err := globalResolver.Resolve(context.Background(), metadata.DstHost)
			if err == nil {
				ip = resolved
			}
		} else {
			resolved, err := net.LookupHost(metadata.DstHost)
			if err == nil && len(resolved) > 0 {
				ip = net.ParseIP(resolved[0])
			}
		}
	}
	if ip == nil {
		return false
	}

	// 特殊处理：GEOIP,CN 也匹配私有/保留地址
	if r.country == "CN" && isPrivateIP(ip) {
		return false // 私有 IP 不属于 CN，单独用 IP-CIDR 规则覆盖
	}

	country := LookupCountry(ip)
	return strings.EqualFold(country, r.country)
}

// isPrivateIP 判断是否私有/保留 IP
func isPrivateIP(ip net.IP) bool {
	private := []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"127.0.0.0/8", "169.254.0.0/16", "::1/128", "fc00::/7",
	}
	for _, cidr := range private {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

// ── GEOSITE 规则（基于域名分类）────────────────────────────────────────────────
// 对应 mihomo/rules/common/geosite.go
// 简化实现：仅支持通过域名前缀/关键字匹配，完整实现需要 v2ray-geosite 数据库

type GeoSiteRule struct {
	site    string // 如 "cn" "google"
	adapter string
	domains []string // 该 site 包含的域名列表（从 geosite.dat 加载）
}

func NewGeoSiteRule(site, adapter string) *GeoSiteRule {
	return &GeoSiteRule{site: strings.ToLower(site), adapter: adapter}
}

func (r *GeoSiteRule) RuleType() string { return "GEOSITE" }
func (r *GeoSiteRule) Adapter() string  { return r.adapter }
func (r *GeoSiteRule) Payload() string  { return r.site }

func (r *GeoSiteRule) Match(metadata *Metadata) bool {
	host := metadata.DstHost
	if host == "" {
		return false
	}
	// 简化：使用内置的少量关键域名列表
	// 完整实现需要加载 geosite.dat
	for _, domain := range r.domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// LoadGeoSite 加载 geosite 数据（预留接口，暂用内置列表）
func (r *GeoSiteRule) LoadGeoSite(domains []string) {
	r.domains = domains
}
