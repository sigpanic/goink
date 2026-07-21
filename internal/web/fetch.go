package web

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
	readability "codeberg.org/readeck/go-readability/v2"
)

const (
	fetchTimeout   = 30 * time.Second
	fetchMaxChars  = 15000
	fetchMaxBytes  = 10 << 20 // 10 MB
	fetchDelayMin  = 500 * time.Millisecond
	fetchDelayMax  = 1500 * time.Millisecond
	fetchUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

	garbledMinTotal = 50 // 总字符数低于此值不检测（太短无法判断）

	compressionMinBytes  = 1000 // 文本低于此字节不做压缩检测（gzip 开销在短文本中占比太大，压缩率失真）
	compressionThreshold = 0.75 // 压缩比超过此值视为不可压缩（加密/随机数据）
)

var cookieJar, _ = cookiejar.New(nil)

// FetchResult 是网页抓取结果。
type FetchResult struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// Fetch 抓取指定 URL 的网页内容，清洗后返回 markdown。
func Fetch(rawURL string) (*FetchResult, error) {
	u, err := parseAndValidate(rawURL)
	if err != nil {
		return nil, err
	}

	time.Sleep(fetchDelayMin + rand.N(fetchDelayMax-fetchDelayMin))

	client := &http.Client{
		Timeout: fetchTimeout,
		Jar:     cookieJar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向次数过多")
			}
			if err := validateHost(req.URL.Host); err != nil {
				return fmt.Errorf("重定向目标不安全: %w", err)
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", fetchUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Sec-Ch-Ua", `"Google Chrome";v="149", "Chromium";v="149", "Not.A/Brand";v="99"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if isAntiCrawlHeaders(resp) {
		return nil, fmt.Errorf("检测到反爬/CDN 防护响应头，无法抓取")
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if len(body) > fetchMaxBytes {
		return nil, fmt.Errorf("网页过大，超过 %d MB", fetchMaxBytes>>20)
	}

	// 先用 readability 提取正文区域
	article, err := readability.FromReader(bytes.NewReader(body), u)
	if err != nil {
		return nil, fmt.Errorf("提取正文失败: %w", err)
	}

	title := strings.TrimSpace(article.Title())

	// 将提取的正文 node 渲染为 HTML
	var contentBuf bytes.Buffer
	if article.Node != nil {
		if renderErr := article.RenderHTML(&contentBuf); renderErr != nil {
			return nil, fmt.Errorf("渲染正文失败: %w", renderErr)
		}
	}
	contentHTML := contentBuf.String()

	// readability 没提取到正文时回退用全文
	if strings.TrimSpace(contentHTML) == "" {
		doc, docErr := goquery.NewDocumentFromReader(bytes.NewReader(body))
		if docErr != nil {
			return nil, fmt.Errorf("解析 HTML 失败: %w", docErr)
		}
		contentSel := doc.Find("body")
		if contentSel.Length() == 0 {
			contentSel = doc.Find("html")
		}
		contentHTML, docErr = contentSel.Html()
		if docErr != nil {
			return nil, fmt.Errorf("提取正文失败: %w", docErr)
		}
	}

	converter := md.NewConverter("", true, nil)
	text, err := converter.ConvertString(contentHTML)
	if err != nil {
		return nil, fmt.Errorf("转换 markdown 失败: %w", err)
	}

	text = strings.TrimSpace(text)
	textLen := len([]rune(text))
	if isEncodingGarbled(text) {
		return nil, fmt.Errorf("网页编码异常，无法解析")
	}
	if isAntiCrawl(title, textLen, len(body)) {
		return nil, fmt.Errorf("网页可能为反爬验证页面，无法抓取有效内容")
	}
	if textLen > fetchMaxChars {
		runes := []rune(text)
		text = string(runes[:fetchMaxChars]) + "\n\n...[内容已截断]"
	}

	return &FetchResult{
		URL:   rawURL,
		Title: title,
		Text:  text,
	}, nil
}

// isEncodingGarbled 检测编码错误或加密导致的乱码。
//
// 信号优先级：
//  1. U+FFFD 替换字符 — Go 解码无效 UTF-8 字节时插入，确定性信号
//  2. gzip 压缩率   — 自然语言有大量冗余（词频、语法），压缩率通常在 0.4-0.7；
//     加密/随机数据几乎不可压缩，压缩率接近 1.0
func isEncodingGarbled(text string) bool {
	runes := []rune(text)
	if len(runes) < garbledMinTotal {
		return false
	}

	if strings.ContainsRune(text, '�') {
		return true
	}

	// 文本太短时 gzip 头部开销占比太大，不做压缩检测
	textBytes := []byte(text)
	if len(textBytes) >= compressionMinBytes {
		if compressionRatio(textBytes) > compressionThreshold {
			return true
		}
	}

	return false
}

// compressionRatio 返回 gzip 压缩比（压缩后大小 / 原始大小）。
// 自然语言通常 0.4-0.7，加密/随机数据通常 > 0.85。
func compressionRatio(data []byte) float64 {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(data)
	w.Close()
	return float64(buf.Len()) / float64(len(data))
}

// 反爬/验证页面的典型标题关键词。
var antiCrawlTitles = []string{
	"just a moment",
	"attention required",
	"please verify",
	"are you a robot",
	"captcha",
	"安全检查",
	"请完成验证",
	"请验证您是",
	"正在检查您的浏览器",
	"请稍候",
	"访问被拒绝",
	"access denied",
	"403 forbidden",
	"请启用javascript",
	"please enable javascript",
	"您的浏览器需要",
	"系统检测到",
}

// isAntiCrawl 检测反爬或验证页面。
// 这类页面返回正常 HTML 和 200 状态码，但没有实际文章内容。
func isAntiCrawl(title string, extractedLen, bodyLen int) bool {
	lowerTitle := strings.ToLower(title)
	for _, kw := range antiCrawlTitles {
		if strings.Contains(lowerTitle, kw) {
			return true
		}
	}

	// HTML 很大但 readability 几乎没提取到内容 → 大概率是反爬/加密页面
	if bodyLen > 5000 && extractedLen < 200 {
		return true
	}

	return false
}

// 反爬/CDN 防护的 HTTP 响应头指纹。
var antiCrawlHeaders = []string{
	"cf-chl-bypass",
	"cf-mitigated",
	"x-sucuri-id",
	"x-sucuri-cache",
	"x-iinfo",
	"x-datadome",
	"x-fw-version",
	"x-edgeconnect-mid",
	"x-akamai-transformed",
}

var antiCrawlServerTokens = []string{
	"cloudflare-nginx",
	"sucuri",
	"incapsula",
	"imperva",
}

var antiCrawlCookies = []string{
	"cf_ob_info",  // Cloudflare 验证页面
	"cf_use_ob",   // Cloudflare 验证页面
	"incap_ses",   // Incapsula
	"visid_incap", // Incapsula
	"ak_bmsc",     // Akamai Bot Manager
}
// 注意：__cf_bm 是 Cloudflare Bot Management 的普适令牌，正常页面也会设置，不能作为拦截信号

// isAntiCrawlHeaders 检测 HTTP 响应头中是否包含反爬/CDN 防护指纹。
func isAntiCrawlHeaders(resp *http.Response) bool {
	for _, key := range antiCrawlHeaders {
		if resp.Header.Get(key) != "" {
			return true
		}
	}

	lowerServer := strings.ToLower(resp.Header.Get("server"))
	for _, tk := range antiCrawlServerTokens {
		if strings.Contains(lowerServer, tk) {
			return true
		}
	}

	for _, ck := range antiCrawlCookies {
		for _, c := range resp.Cookies() {
			if strings.EqualFold(c.Name, ck) {
				return true
			}
		}
	}

	return false
}

func parseAndValidate(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("URL 格式无效: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("仅支持 http/https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("URL 缺少主机名")
	}
	if u.User != nil {
		return nil, fmt.Errorf("URL 不允许包含用户信息")
	}
	if err := validateHost(u.Host); err != nil {
		return nil, err
	}
	return u, nil
}

func validateHost(host string) error {
	// 去掉端口
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// 阻止云 metadata 端点
	blocked := map[string]bool{
		"169.254.169.254":          true,
		"metadata.google.internal": true,
		"metadata.tencentyun.com":  true,
		"100.100.100.200":          true,
	}
	if blocked[host] {
		return fmt.Errorf("禁止访问该地址")
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("DNS 解析失败: %w", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("DNS 解析无结果")
	}

	for _, ip := range ips {
		if isPrivate(ip) {
			return fmt.Errorf("禁止访问内网地址: %s", ip)
		}
	}

	return nil
}

func isPrivate(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsPrivate()
}
