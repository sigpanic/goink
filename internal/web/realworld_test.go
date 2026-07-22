package web

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	nurl "net/url"
	"strings"
	"testing"

	readability "codeberg.org/readeck/go-readability/v2"
	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
)

// realWorldSite 代表一个待测试的真实网站。
type realWorldSite struct {
	URL         string
	Description string
	ExpectPass  bool   // 我们预期 Fetch 应该成功还是失败
	Why         string // 预期原因
}

func TestRealWorldSites(t *testing.T) {
	sites := []realWorldSite{
		{
			URL: "https://api-docs.deepseek.com/", Description: "DeepSeek API 文档",
			ExpectPass: true, Why: "正常技术文档，无防护",
		},
		{
			URL: "https://en.wikipedia.org/wiki/Web_scraping", Description: "Wikipedia 英文",
			ExpectPass: true, Why: "正常百科页面，无防护",
		},
		{
			URL: "https://blog.csdn.net/", Description: "CSDN 首页",
			ExpectPass: true, Why: "中文技术社区首页",
		},
		{
			URL: "https://www.infoq.cn/", Description: "InfoQ 中文",
			ExpectPass: true, Why: "中文技术新闻",
		},
		{
			URL: "https://sspai.com/", Description: "少数派首页",
			ExpectPass: true, Why: "中文科技媒体，短内容碎片，压缩率可能偏高但不应误判",
		},
		{
			URL: "https://www.jiqizhixin.com/", Description: "机器之心",
			ExpectPass: true, Why: "中文 AI 媒体",
		},
		{
			URL: "https://www.oschina.net/", Description: "开源中国",
			ExpectPass: true, Why: "中文技术社区",
		},
		{
			URL: "https://pkg.go.dev/fmt", Description: "Go 包文档",
			ExpectPass: true, Why: "正常英文技术文档",
		},
		{
			URL: "https://www.gnu.org/philosophy/free-sw.html", Description: "GNU 哲学页面",
			ExpectPass: true, Why: "正常英文文章",
		},
		{
			URL: "https://react.dev/learn", Description: "React 官方文档",
			ExpectPass: true, Why: "正常英文技术文档",
		},
		{
			URL: "https://news.ycombinator.com/", Description: "Hacker News",
			ExpectPass: true, Why: "正常技术社区，当前未启用严格防护",
		},
		{
			URL: "https://www.cloudflare.com/", Description: "Cloudflare 官网",
			ExpectPass: true, Why: "正常企业官网，Server: cloudflare 不应触发拦截（仅 cloudflare-nginx 才是验证页）",
		},
	}

	passed := 0
	failed := 0

	for _, site := range sites {
		t.Run(site.Description, func(t *testing.T) {
			resp, err := fetchRaw(site.URL)
			if err != nil {
				if site.ExpectPass {
					t.Logf("⚠ 预期通过但失败: %s → %v", site.Description, err)
				} else {
					t.Logf("✓ 预期拦截: %s → %v", site.Description, err)
				}
				failed++
				return
			}
			defer resp.Body.Close()

			result, analysis := analyzeFetch(resp)
			passed++

			t.Logf("URL:      %s", site.URL)
			t.Logf("预期:      %s (%s)", passStr(site.ExpectPass), site.Why)
			t.Logf("HTTP:      %d", resp.StatusCode)
			t.Logf("Content-Type: %s", resp.Header.Get("Content-Type"))
			t.Logf("Server:    %s", resp.Header.Get("server"))
			t.Logf("Title:     %s", result.Title)
			t.Logf("Text 长度: %d runes, %d bytes", len([]rune(result.Text)), len(result.Text))

			// 检查每个检测层
			t.Logf("检测结果:")
			t.Logf("  响应头反爬: %v", analysis.antiCrawlHeaders)
			t.Logf("  标题反爬:   %v", analysis.antiCrawlTitle)
			t.Logf("  内容量比:   %v (提取 %d 字 / 原始 %d 字节)", analysis.lowContentRatio, len([]rune(result.Text)), analysis.bodyLen)
			t.Logf("  U+FFFD:     %v", analysis.hasFFFD)
			t.Logf("  压缩率:     %.4f (阈值 %.2f) → garbled=%v", analysis.compressionRatio, compressionThreshold, analysis.isGarbled)

			// 显示内容预览
			preview := result.Text
			if len([]rune(preview)) > 300 {
				preview = string([]rune(preview)[:300]) + "..."
			}
			t.Logf("\n--- 内容预览 (前300字) ---\n%s\n---", preview)

			// 人工审查提示：查看内容是否是真实文章
			if analysis.isGarbled || analysis.antiCrawlHeaders || analysis.antiCrawlTitle || analysis.lowContentRatio {
				t.Logf("🔴 程序判定: 应拦截")
			} else {
				t.Logf("🟢 程序判定: 可接受")
			}
		})
	}

	t.Logf("\n========== 汇总 ==========")
	t.Logf("总计: %d 个网站, %d 通过, %d 被拦截", passed+failed, passed, failed)
}

// fetchRaw 发送 GET 请求并返回原始 response（不做内容提取），用于测试分析。
func fetchRaw(rawURL string) (*http.Response, error) {
	u, err := parseAndValidate(rawURL)
	if err != nil {
		return nil, err
	}

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
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

// fetchAnalysis 保存各项检测的中间结果。
type fetchAnalysis struct {
	antiCrawlHeaders bool
	antiCrawlTitle   bool
	lowContentRatio  bool
	hasFFFD          bool
	compressionRatio float64
	isGarbled        bool
	bodyLen          int
}

// analyzeFetch 对已获取的 response 做完整分析和内容提取。
func analyzeFetch(resp *http.Response) (*FetchResult, fetchAnalysis) {
	analysis := fetchAnalysis{}

	if isAntiCrawlHeaders(resp) {
		analysis.antiCrawlHeaders = true
	}

	reqURL, _ := nurl.Parse(resp.Request.URL.String())

	body, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxBytes+1))
	if err != nil || len(body) > fetchMaxBytes {
		return &FetchResult{URL: resp.Request.URL.String()}, analysis
	}
	analysis.bodyLen = len(body)

	article, err := readability.FromReader(bytes.NewReader(body), reqURL)
	if err != nil {
		return &FetchResult{URL: reqURL.String()}, analysis
	}

	title := strings.TrimSpace(article.Title())
	var contentHTML string
	if article.Node != nil {
		var buf bytes.Buffer
		article.RenderHTML(&buf)
		contentHTML = buf.String()
	}

	if strings.TrimSpace(contentHTML) == "" {
		doc, docErr := goquery.NewDocumentFromReader(bytes.NewReader(body))
		if docErr == nil {
			sel := doc.Find("body")
			if sel.Length() == 0 {
				sel = doc.Find("html")
			}
			contentHTML, _ = sel.Html()
		}
	}

	converter := md.NewConverter("", true, nil)
	text, _ := converter.ConvertString(contentHTML)
	text = strings.TrimSpace(text)

	textLen := len([]rune(text))
	analysis.hasFFFD = strings.ContainsRune(text, '�')
	if len(text) >= compressionMinBytes {
		analysis.compressionRatio = compressionRatio([]byte(text))
		analysis.isGarbled = analysis.hasFFFD || analysis.compressionRatio > compressionThreshold
	} else {
		analysis.isGarbled = analysis.hasFFFD
	}
	analysis.antiCrawlTitle = isAntiCrawlTitleOnly(title)
	analysis.lowContentRatio = analysis.bodyLen > 5000 && textLen < 200

	return &FetchResult{URL: reqURL.String(), Title: title, Text: text}, analysis
}

// isAntiCrawlTitleOnly 只检查 title 关键词（不含内容量比）。
func isAntiCrawlTitleOnly(title string) bool {
	lowerTitle := strings.ToLower(title)
	for _, kw := range antiCrawlTitles {
		if strings.Contains(lowerTitle, kw) {
			return true
		}
	}
	return false
}

func passStr(expect bool) string {
	if expect {
		return "应通过"
	}
	return "应拦截"
}
