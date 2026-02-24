// Package unlock 媒体解锁检测
// 对应原 src-tauri/src/cmd/media_unlock_checker/
// 所有检测通过代理出口节点发起真实 HTTP 请求
package unlock

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
const timeout = 30 * time.Second

// UnlockItem 单个平台检测结果
// 对应原 types.rs::UnlockItem
type UnlockItem struct {
	Name      string  `json:"name"`
	Status    string  `json:"status"` // "Yes" | "No" | "Failed" | "Originals Only" | ...
	Region    *string `json:"region"`
	CheckTime *string `json:"check_time"`
}

// 默认 Pending 列表名（与原项目一致）
var defaultNames = []string{
	"哔哩哔哩大陆",
	"哔哩哔哩港澳台",
	"ChatGPT iOS",
	"ChatGPT Web",
	"Claude",
	"Gemini",
	"YouTube Premium",
	"Bahamut Anime",
	"Netflix",
	"Disney+",
	"Prime Video",
	"Spotify",
	"TikTok",
}

// DefaultUnlockItems 返回 Pending 占位列表
func DefaultUnlockItems() []UnlockItem {
	items := make([]UnlockItem, len(defaultNames))
	for i, name := range defaultNames {
		items[i] = UnlockItem{Name: name, Status: "Pending"}
	}
	return items
}

// newClient 创建共享 HTTP 客户端
func newClient() *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// 允许最多 10 次重定向，并保留 User-Agent
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			req.Header.Set("User-Agent", userAgent)
			return nil
		},
		Transport: &http.Transport{
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			DisableKeepAlives:     false,
		},
	}
}

// nowStr 返回本地时间字符串
func nowStr() *string {
	s := time.Now().Format("2006-01-02 15:04:05")
	return &s
}

// strPtr 将字符串转为指针
func strPtr(s string) *string { return &s }

// countryCodeToEmoji 二字母 ISO 代码 → 国旗 emoji（同原 utils.rs）
func countryCodeToEmoji(code string) string {
	uc := strings.ToUpper(strings.TrimSpace(code))
	if len(uc) == 3 {
		// 简单 alpha3→alpha2 映射（常用）
		alpha3Map := map[string]string{
			"CHN": "CN", "USA": "US", "GBR": "GB", "JPN": "JP",
			"KOR": "KR", "TWN": "TW", "HKG": "HK", "SGP": "SG",
			"AUS": "AU", "DEU": "DE", "FRA": "FR", "CAN": "CA",
		}
		if a2, ok := alpha3Map[uc]; ok {
			uc = a2
		} else {
			return ""
		}
	}
	if len(uc) != 2 {
		return ""
	}
	r1 := rune(0x1F1E6) + rune(uc[0]-'A')
	r2 := rune(0x1F1E6) + rune(uc[1]-'A')
	return string(r1) + string(r2)
}

// get 执行 GET 请求，返回响应（需调用者关闭 Body）
func get(client *http.Client, url string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return client.Do(req)
}

// readBody 读取响应体字符串
func readBody(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	defer resp.Body.Close()
	buf := new(strings.Builder)
	buf.Grow(4096)
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			break
		}
	}
	return buf.String()
}

// ----- 哔哩哔哩大陆 -----

func checkBilibiliMainland(client *http.Client) UnlockItem {
	url := "https://api.bilibili.com/pgc/player/web/playurl?avid=82846771&qn=0&type=&otype=json&ep_id=307247&fourk=1&fnver=0&fnval=16&module=bangumi"
	resp, err := get(client, url, nil)
	if err != nil {
		return UnlockItem{Name: "哔哩哔哩大陆", Status: "Failed", CheckTime: nowStr()}
	}
	body := readBody(resp)
	var data map[string]interface{}
	if json.Unmarshal([]byte(body), &data) != nil {
		return UnlockItem{Name: "哔哩哔哩大陆", Status: "Failed", CheckTime: nowStr()}
	}
	status := "Failed"
	if code, ok := data["code"].(float64); ok {
		if code == 0 {
			status = "Yes"
		} else if code == -10403 {
			status = "No"
		}
	}
	return UnlockItem{Name: "哔哩哔哩大陆", Status: status, CheckTime: nowStr()}
}

// ----- 哔哩哔哩港澳台 -----

func checkBilibiliHKMCTW(client *http.Client) UnlockItem {
	url := "https://api.bilibili.com/pgc/player/web/playurl?avid=18281381&cid=29892777&qn=0&type=&otype=json&ep_id=183799&fourk=1&fnver=0&fnval=16&module=bangumi"
	resp, err := get(client, url, nil)
	if err != nil {
		return UnlockItem{Name: "哔哩哔哩港澳台", Status: "Failed", CheckTime: nowStr()}
	}
	body := readBody(resp)
	var data map[string]interface{}
	if json.Unmarshal([]byte(body), &data) != nil {
		return UnlockItem{Name: "哔哩哔哩港澳台", Status: "Failed", CheckTime: nowStr()}
	}
	status := "Failed"
	if code, ok := data["code"].(float64); ok {
		if code == 0 {
			status = "Yes"
		} else if code == -10403 {
			status = "No"
		}
	}
	return UnlockItem{Name: "哔哩哔哩港澳台", Status: status, CheckTime: nowStr()}
}

// ----- ChatGPT -----

func checkChatGPT(client *http.Client) []UnlockItem {
	// 获取区域
	var region *string
	if resp, err := get(client, "https://chat.openai.com/cdn-cgi/trace", nil); err == nil {
		body := readBody(resp)
		for _, line := range strings.Split(body, "\n") {
			if after, ok := strings.CutPrefix(line, "loc="); ok {
				loc := strings.TrimSpace(after)
				emoji := countryCodeToEmoji(loc)
				region = strPtr(emoji + loc)
				break
			}
		}
	}

	// iOS 检测
	iosStatus := "Failed"
	if resp, err := get(client, "https://ios.chat.openai.com/", nil); err == nil {
		body := strings.ToLower(readBody(resp))
		switch {
		case strings.Contains(body, "you may be connected to a disallowed isp"):
			iosStatus = "Disallowed ISP"
		case strings.Contains(body, "request is not allowed. please try again later."):
			iosStatus = "Yes"
		case strings.Contains(body, "sorry, you have been blocked"):
			iosStatus = "Blocked"
		}
	}

	// Web 检测
	webStatus := "Failed"
	if resp, err := get(client, "https://api.openai.com/compliance/cookie_requirements", nil); err == nil {
		body := strings.ToLower(readBody(resp))
		if strings.Contains(body, "unsupported_country") {
			webStatus = "Unsupported Country/Region"
		} else {
			webStatus = "Yes"
		}
	}

	return []UnlockItem{
		{Name: "ChatGPT iOS", Status: iosStatus, Region: region, CheckTime: nowStr()},
		{Name: "ChatGPT Web", Status: webStatus, Region: region, CheckTime: nowStr()},
	}
}

// ----- Claude -----

var claudeBlockedCodes = map[string]bool{
	"AF": true, "BY": true, "CN": true, "CU": true,
	"HK": true, "IR": true, "KP": true, "MO": true, "RU": true, "SY": true,
}

func checkClaude(client *http.Client) UnlockItem {
	resp, err := get(client, "https://claude.ai/cdn-cgi/trace", nil)
	if err != nil {
		return UnlockItem{Name: "Claude", Status: "Failed", CheckTime: nowStr()}
	}
	body := readBody(resp)
	for _, line := range strings.Split(body, "\n") {
		if after, ok := strings.CutPrefix(line, "loc="); ok {
			code := strings.ToUpper(strings.TrimSpace(after))
			emoji := countryCodeToEmoji(code)
			status := "Yes"
			if claudeBlockedCodes[code] {
				status = "No"
			}
			return UnlockItem{Name: "Claude", Status: status, Region: strPtr(emoji + code), CheckTime: nowStr()}
		}
	}
	return UnlockItem{Name: "Claude", Status: "Failed", CheckTime: nowStr()}
}

// ----- Gemini -----

func checkGemini(client *http.Client) UnlockItem {
	resp, err := get(client, "https://gemini.google.com", nil)
	if err != nil {
		return UnlockItem{Name: "Gemini", Status: "Failed", CheckTime: nowStr()}
	}
	body := readBody(resp)

	status := "No"
	if strings.Contains(body, "45631641,null,true") {
		status = "Yes"
	}

	// 从响应内容中提取 alpha3 区域码
	var region *string
	marker := `,2,1,200,"`
	if idx := strings.Index(body, marker); idx >= 0 {
		rest := body[idx+len(marker):]
		if end := strings.Index(rest, `"`); end >= 0 && end == 3 {
			code := rest[:end]
			emoji := countryCodeToEmoji(code)
			region = strPtr(emoji + code)
		}
	}

	return UnlockItem{Name: "Gemini", Status: status, Region: region, CheckTime: nowStr()}
}

// ----- YouTube Premium -----

func checkYouTubePremium(client *http.Client) UnlockItem {
	resp, err := get(client, "https://www.youtube.com/premium", nil)
	if err != nil {
		return UnlockItem{Name: "YouTube Premium", Status: "Failed", CheckTime: nowStr()}
	}
	body := readBody(resp)
	bodyLower := strings.ToLower(body)

	if strings.Contains(bodyLower, "youtube premium is not available in your country") {
		return UnlockItem{Name: "YouTube Premium", Status: "No", CheckTime: nowStr()}
	}

	if strings.Contains(bodyLower, "ad-free") {
		// 尝试从页面中找 country-code 元素
		marker := `id="country-code"`
		if idx := strings.Index(body, marker); idx >= 0 {
			rest := body[idx+len(marker):]
			if gtIdx := strings.Index(rest, ">"); gtIdx >= 0 {
				rest = rest[gtIdx+1:]
				if ltIdx := strings.Index(rest, "<"); ltIdx >= 0 {
					code := strings.TrimSpace(rest[:ltIdx])
					if code != "" {
						emoji := countryCodeToEmoji(code)
						return UnlockItem{Name: "YouTube Premium", Status: "Yes", Region: strPtr(emoji + code), CheckTime: nowStr()}
					}
				}
			}
		}
		return UnlockItem{Name: "YouTube Premium", Status: "Yes", CheckTime: nowStr()}
	}

	return UnlockItem{Name: "YouTube Premium", Status: "Failed", CheckTime: nowStr()}
}

// ----- Bahamut Anime -----

func checkBahamut(client *http.Client) UnlockItem {
	// 获取 deviceid
	resp, err := get(client, "https://ani.gamer.com.tw/ajax/getdeviceid.php", nil)
	if err != nil {
		return UnlockItem{Name: "Bahamut Anime", Status: "Failed", CheckTime: nowStr()}
	}
	body := readBody(resp)
	deviceID := extractQuotedValue(body, `"deviceid"`)
	if deviceID == "" {
		return UnlockItem{Name: "Bahamut Anime", Status: "Failed", CheckTime: nowStr()}
	}

	// 检查 token
	tokenURL := fmt.Sprintf("https://ani.gamer.com.tw/ajax/token.php?adID=89422&sn=37783&device=%s", deviceID)
	tokenResp, err := get(client, tokenURL, nil)
	if err != nil {
		return UnlockItem{Name: "Bahamut Anime", Status: "No", CheckTime: nowStr()}
	}
	tokenBody := readBody(tokenResp)
	if !strings.Contains(tokenBody, "animeSn") {
		return UnlockItem{Name: "Bahamut Anime", Status: "No", CheckTime: nowStr()}
	}

	// 获取区域
	var region *string
	if mainResp, err2 := get(client, "https://ani.gamer.com.tw/", nil); err2 == nil {
		mainBody := readBody(mainResp)
		code := extractAttrValue(mainBody, `data-geo="`)
		if code != "" {
			emoji := countryCodeToEmoji(code)
			region = strPtr(emoji + code)
		}
	}

	return UnlockItem{Name: "Bahamut Anime", Status: "Yes", Region: region, CheckTime: nowStr()}
}

// ----- Netflix -----

func checkNetflix(client *http.Client) UnlockItem {
	// 先试 CDN 快速检测
	if item := checkNetflixCDN(client); item.Status == "Yes" {
		return item
	}

	resp1, err1 := get(client, "https://www.netflix.com/title/81280792", nil)
	resp2, err2 := get(client, "https://www.netflix.com/title/70143836", nil)
	if err1 != nil || err2 != nil {
		return UnlockItem{Name: "Netflix", Status: "Failed", CheckTime: nowStr()}
	}

	st1 := resp1.StatusCode
	st2 := resp2.StatusCode
	resp1.Body.Close()
	resp2.Body.Close()

	if st1 == 404 && st2 == 404 {
		return UnlockItem{Name: "Netflix", Status: "Originals Only", CheckTime: nowStr()}
	}
	if st1 == 403 || st2 == 403 {
		return UnlockItem{Name: "Netflix", Status: "No", CheckTime: nowStr()}
	}
	if st1 == 200 || st1 == 301 || st2 == 200 || st2 == 301 {
		// 获取区域
		resp3, err3 := get(client, "https://www.netflix.com/title/80018499", nil)
		if err3 != nil {
			return UnlockItem{Name: "Netflix", Status: "Yes", CheckTime: nowStr()}
		}
		loc := resp3.Header.Get("location")
		resp3.Body.Close()
		if loc != "" {
			parts := strings.Split(loc, "/")
			if len(parts) >= 4 {
				regionCode := strings.Split(parts[3], "-")[0]
				emoji := countryCodeToEmoji(regionCode)
				return UnlockItem{Name: "Netflix", Status: "Yes", Region: strPtr(emoji + regionCode), CheckTime: nowStr()}
			}
		}
		emoji := countryCodeToEmoji("us")
		return UnlockItem{Name: "Netflix", Status: "Yes", Region: strPtr(emoji + "us"), CheckTime: nowStr()}
	}

	return UnlockItem{Name: "Netflix", Status: fmt.Sprintf("Failed (status: %d_%d)", st1, st2), CheckTime: nowStr()}
}

func checkNetflixCDN(client *http.Client) UnlockItem {
	url := "https://api.fast.com/netflix/speedtest/v2?https=true&token=YXNkZmFzZGxmbnNkYWZoYXNkZmhrYWxm&urlCount=5"
	resp, err := get(client, url, nil)
	if err != nil {
		return UnlockItem{Name: "Netflix", Status: "Failed (CDN API)", CheckTime: nowStr()}
	}
	if resp.StatusCode == 403 {
		resp.Body.Close()
		return UnlockItem{Name: "Netflix", Status: "No (IP Banned By Netflix)", CheckTime: nowStr()}
	}
	body := readBody(resp)
	var data map[string]interface{}
	if json.Unmarshal([]byte(body), &data) != nil {
		return UnlockItem{Name: "Netflix", Status: "Unknown", CheckTime: nowStr()}
	}
	if targets, ok := data["targets"].([]interface{}); ok && len(targets) > 0 {
		if loc, ok := targets[0].(map[string]interface{})["location"].(map[string]interface{}); ok {
			if country, ok := loc["country"].(string); ok && country != "" {
				emoji := countryCodeToEmoji(country)
				return UnlockItem{Name: "Netflix", Status: "Yes", Region: strPtr(emoji + country), CheckTime: nowStr()}
			}
		}
	}
	return UnlockItem{Name: "Netflix", Status: "Unknown", CheckTime: nowStr()}
}

// ----- Disney+ -----

func checkDisneyPlus(client *http.Client) UnlockItem {
	deviceURL := "https://disney.api.edge.bamgrid.com/devices"
	authHeader := "Bearer ZGlzbmV5JmJyb3dzZXImMS4wLjA.Cu56AgSfBTDag5NiRA81oLHkDZfu5L3CKadnefEAY84"

	deviceBody := `{"deviceFamily":"browser","applicationRuntime":"chrome","deviceProfile":"windows","attributes":{}}`
	req, _ := http.NewRequestWithContext(context.Background(), "POST", deviceURL, strings.NewReader(deviceBody))
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("authorization", authHeader)
	req.Header.Set("content-type", "application/json; charset=UTF-8")

	devResp, err := client.Do(req)
	if err != nil {
		return UnlockItem{Name: "Disney+", Status: "Failed (Network Connection)", CheckTime: nowStr()}
	}
	if devResp.StatusCode == 403 {
		devResp.Body.Close()
		return UnlockItem{Name: "Disney+", Status: "No (IP Banned By Disney+)", CheckTime: nowStr()}
	}
	devBody := readBody(devResp)

	assertion := extractQuotedValue(devBody, `"assertion"`)
	if assertion == "" {
		return UnlockItem{Name: "Disney+", Status: "Failed (Error: Cannot extract assertion)", CheckTime: nowStr()}
	}

	// 获取 token
	tokenURL := "https://disney.api.edge.bamgrid.com/token"
	tokenBody := fmt.Sprintf(
		"grant_type=urn%%3Aietf%%3Aparams%%3Aoauth%%3Agrant-type%%3Atoken-exchange&latitude=0&longitude=0&platform=browser&subject_token=%s&subject_token_type=urn%%3Abamtech%%3Aparams%%3Aoauth%%3Atoken-type%%3Adevice",
		assertion,
	)
	tokReq, _ := http.NewRequestWithContext(context.Background(), "POST", tokenURL, strings.NewReader(tokenBody))
	tokReq.Header.Set("User-Agent", userAgent)
	tokReq.Header.Set("authorization", authHeader)
	tokReq.Header.Set("content-type", "application/x-www-form-urlencoded")

	tokResp, err := client.Do(tokReq)
	if err != nil {
		return UnlockItem{Name: "Disney+", Status: "Failed (Network Connection)", CheckTime: nowStr()}
	}
	tokText := readBody(tokResp)

	if strings.Contains(tokText, "forbidden-location") || strings.Contains(tokText, "403 ERROR") {
		return UnlockItem{Name: "Disney+", Status: "No (IP Banned By Disney+)", CheckTime: nowStr()}
	}

	refreshToken := extractQuotedValue(tokText, `"refresh_token"`)
	if refreshToken == "" {
		return UnlockItem{Name: "Disney+", Status: "Failed (Cannot extract refresh token)", CheckTime: nowStr()}
	}

	// 检查主页可用性
	mainResp, err2 := get(client, "https://disneyplus.com", nil)
	isUnavailable := err2 != nil
	if err2 == nil {
		finalURL := mainResp.Request.URL.String()
		mainResp.Body.Close()
		isUnavailable = strings.Contains(finalURL, "preview") || strings.Contains(finalURL, "unavailable")
	}

	// GraphQL 获取区域码
	graphqlURL := "https://disney.api.edge.bamgrid.com/graph/v1/device/graphql"
	graphqlPayload := fmt.Sprintf(
		`{"query":"mutation refreshToken($input: RefreshTokenInput!) { refreshToken(refreshToken: $input) { activeSession { sessionId } } }","variables":{"input":{"refreshToken":"%s"}}}`,
		refreshToken,
	)
	gqlReq, _ := http.NewRequestWithContext(context.Background(), "POST", graphqlURL, strings.NewReader(graphqlPayload))
	gqlReq.Header.Set("User-Agent", userAgent)
	gqlReq.Header.Set("authorization", authHeader)
	gqlReq.Header.Set("content-type", "application/json")

	gqlResp, err := client.Do(gqlReq)
	if err != nil {
		return UnlockItem{Name: "Disney+", Status: "Failed (Network Connection)", CheckTime: nowStr()}
	}
	gqlBody := readBody(gqlResp)

	regionCode := extractQuotedValue(gqlBody, `"countryCode"`)
	if regionCode == "" {
		return UnlockItem{Name: "Disney+", Status: "No", CheckTime: nowStr()}
	}
	if regionCode == "JP" || !isUnavailable {
		inSupported := strings.Contains(gqlBody, `"inSupportedLocation":true`)
		if !inSupported && strings.Contains(gqlBody, `"inSupportedLocation":false`) {
			emoji := countryCodeToEmoji(regionCode)
			return UnlockItem{Name: "Disney+", Status: "Soon", Region: strPtr(emoji + regionCode + "（即将上线）"), CheckTime: nowStr()}
		}
		emoji := countryCodeToEmoji(regionCode)
		return UnlockItem{Name: "Disney+", Status: "Yes", Region: strPtr(emoji + regionCode), CheckTime: nowStr()}
	}
	return UnlockItem{Name: "Disney+", Status: "No", CheckTime: nowStr()}
}

// ----- Prime Video -----

func checkPrimeVideo(client *http.Client) UnlockItem {
	resp, err := get(client, "https://www.primevideo.com", nil)
	if err != nil {
		return UnlockItem{Name: "Prime Video", Status: "Failed (Network Connection)", CheckTime: nowStr()}
	}
	body := readBody(resp)

	if strings.Contains(body, "isServiceRestricted") {
		return UnlockItem{Name: "Prime Video", Status: "No (Service Not Available)", CheckTime: nowStr()}
	}

	region := extractQuotedValue(body, `"currentTerritory"`)
	if region != "" {
		emoji := countryCodeToEmoji(region)
		return UnlockItem{Name: "Prime Video", Status: "Yes", Region: strPtr(emoji + region), CheckTime: nowStr()}
	}

	return UnlockItem{Name: "Prime Video", Status: "Failed (Error: PAGE ERROR)", CheckTime: nowStr()}
}

// ----- Spotify -----

func checkSpotify(client *http.Client) UnlockItem {
	url := "https://www.spotify.com/api/content/v1/country-selector?platform=web&format=json"
	resp, err := get(client, url, nil)
	if err != nil {
		return UnlockItem{Name: "Spotify", Status: "Failed", CheckTime: nowStr()}
	}
	st := resp.StatusCode
	body := readBody(resp)

	if st == 403 || st == 451 {
		return UnlockItem{Name: "Spotify", Status: "No", CheckTime: nowStr()}
	}
	if st < 200 || st >= 300 {
		return UnlockItem{Name: "Spotify", Status: "Failed", CheckTime: nowStr()}
	}
	if strings.Contains(strings.ToLower(body), "not available in your country") {
		return UnlockItem{Name: "Spotify", Status: "No", CheckTime: nowStr()}
	}

	region := extractQuotedValue(body, `"countryCode"`)
	var regionPtr *string
	if region != "" {
		emoji := countryCodeToEmoji(region)
		regionPtr = strPtr(emoji + strings.ToUpper(region))
	}
	return UnlockItem{Name: "Spotify", Status: "Yes", Region: regionPtr, CheckTime: nowStr()}
}

// ----- TikTok -----

func checkTikTok(client *http.Client) UnlockItem {
	status := "Failed"
	var region *string

	if resp, err := get(client, "https://www.tiktok.com/cdn-cgi/trace", nil); err == nil {
		st := resp.StatusCode
		body := readBody(resp)
		status = determineTikTokStatus(st, body)
		region = extractTikTokRegion(body)
	}

	if region == nil || status == "Failed" {
		if resp, err := get(client, "https://www.tiktok.com/", nil); err == nil {
			st := resp.StatusCode
			body := readBody(resp)
			fallbackStatus := determineTikTokStatus(st, body)
			fallbackRegion := extractTikTokRegion(body)
			if status != "No" {
				status = fallbackStatus
			}
			if region == nil {
				region = fallbackRegion
			}
		}
	}

	return UnlockItem{Name: "TikTok", Status: status, Region: region, CheckTime: nowStr()}
}

func determineTikTokStatus(st int, body string) string {
	if st == 403 || st == 451 {
		return "No"
	}
	if st < 200 || st >= 300 {
		return "Failed"
	}
	lower := strings.ToLower(body)
	if strings.Contains(lower, "access denied") ||
		strings.Contains(lower, "not available in your region") ||
		strings.Contains(lower, "tiktok is not available") {
		return "No"
	}
	return "Yes"
}

func extractTikTokRegion(body string) *string {
	code := extractQuotedValue(body, `"region"`)
	if code == "" {
		return nil
	}
	part := strings.Split(code, "-")[0]
	upper := strings.ToUpper(part)
	if upper == "" {
		return nil
	}
	emoji := countryCodeToEmoji(upper)
	return strPtr(emoji + upper)
}

// ----- 通用辅助 -----

// extractQuotedValue 从 JSON-like 文本中提取 key 对应的第一个引号内字符串值
// 如 `"key":"value"` → "value"
func extractQuotedValue(body, key string) string {
	idx := strings.Index(body, key)
	if idx < 0 {
		return ""
	}
	rest := body[idx+len(key):]
	// 跳过 `:` 和可能的空格
	rest = strings.TrimLeft(rest, ` \t:`)
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	rest = rest[1:]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// extractAttrValue 提取 HTML 属性值，如 `data-geo="TW"` → "TW"
func extractAttrValue(body, attrPrefix string) string {
	idx := strings.Index(body, attrPrefix)
	if idx < 0 {
		return ""
	}
	rest := body[idx+len(attrPrefix):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// ----- 主入口 -----

// CheckAllMedia 并发检测所有媒体平台，通过出口代理发起请求
// 对应原: check_media_unlock
func CheckAllMedia() []UnlockItem {
	client := newClient()
	type checkFn func() []UnlockItem

	tasks := []checkFn{
		func() []UnlockItem { return []UnlockItem{checkBilibiliMainland(client)} },
		func() []UnlockItem { return []UnlockItem{checkBilibiliHKMCTW(client)} },
		func() []UnlockItem { return checkChatGPT(client) },
		func() []UnlockItem { return []UnlockItem{checkClaude(client)} },
		func() []UnlockItem { return []UnlockItem{checkGemini(client)} },
		func() []UnlockItem { return []UnlockItem{checkYouTubePremium(client)} },
		func() []UnlockItem { return []UnlockItem{checkBahamut(client)} },
		func() []UnlockItem { return []UnlockItem{checkNetflix(client)} },
		func() []UnlockItem { return []UnlockItem{checkDisneyPlus(client)} },
		func() []UnlockItem { return []UnlockItem{checkPrimeVideo(client)} },
		func() []UnlockItem { return []UnlockItem{checkSpotify(client)} },
		func() []UnlockItem { return []UnlockItem{checkTikTok(client)} },
	}

	var mu sync.Mutex
	var results []UnlockItem
	var wg sync.WaitGroup

	for _, task := range tasks {
		wg.Add(1)
		t := task
		go func() {
			defer wg.Done()
			items := t()
			mu.Lock()
			results = append(results, items...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results
}
