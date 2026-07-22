package web

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
)

func TestIsEncodingGarbled(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		garbled bool
		reason  string
	}{
		// === 正常文本 — 不应被拦截 ===
		{
			name:    "中文文章",
			text:    chineseArticle(),
			garbled: false,
			reason:  "正常中文散文应有良好压缩率",
		},
		{
			name:    "英文文章",
			text:    englishArticle(),
			garbled: false,
			reason:  "正常英文文章应有良好压缩率",
		},
		{
			name:    "中英混排技术文档",
			text:    mixedTechDoc(),
			garbled: false,
			reason:  "中英混排技术文章不应被误判",
		},
		{
			name:    "Markdown 含链接和代码块",
			text:    markdownWithCode(),
			garbled: false,
			reason:  "带 markdown 语法的正常内容不应被拦截",
		},
		{
			name:    "纯代码段",
			text:    codeSnippet(),
			garbled: false,
			reason:  "代码是合法内容，不是乱码",
		},
		{
			name:    "短文本（低于字符数阈值）",
			text:    "Hello World 你好世界",
			garbled: false,
			reason:  "低于 garbledMinTotal 不检测",
		},
		{
			name: "反爬验证页（可读文本）",
			text: "Just a moment... Checking your browser before accessing the site. " +
				"Please enable JavaScript and cookies to continue. " +
				"DDoS protection by Cloudflare. Ray ID: 8a7f3c2e1b9d4f6a. " +
				"Your request has been blocked. If you are the site owner, contact support.",
			garbled: false,
			reason:  "反爬页面是可读英文，由 isAntiCrawl 处理",
		},

		// === 应被拦截 ===
		{
			name:    "含 U+FFFD 替换字符",
			text:    genTextWithFFFD(600),
			garbled: true,
			reason:  "U+FFFD 是确定性编码错误信号",
		},
		{
			name:    "Base64 编码的加密数据",
			text:    base64Encrypted(2000),
			garbled: true,
			reason:  "base64 密文压缩率应高于正常文本阈值",
		},
		{
			name:    "随机 Unicode 文本（模拟解码错误的乱码）",
			text:    randomUnicodeText(2000),
			garbled: true,
			reason:  "随机字符序列压缩率应高于正常文本阈值",
		},
		{
			name:    "短 base64（低于压缩检测字节阈值）",
			text:    shortBase64(500),
			garbled: false,
			reason:  "低于 compressionMinBytes(1000)，不做压缩检测",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEncodingGarbled(tt.text)
			if got != tt.garbled {
				ratio := compressionRatio([]byte(tt.text))
				t.Errorf("isEncodingGarbled() = %v, want %v (%s)\n  text: %d runes, %d bytes\n  compression ratio: %.4f (threshold: %.2f)",
					got, tt.garbled, tt.reason,
					len([]rune(tt.text)), len(tt.text),
					ratio, compressionThreshold)
			}
		})
	}
}

func TestIsAntiCrawl(t *testing.T) {
	tests := []struct {
		name         string
		title        string
		extractedLen int
		bodyLen      int
		want         bool
	}{
		// 标题关键词
		{"Cloudflare", "Just a moment...", 50, 8000, true},
		{"Cloudflare中文", "安全检查 | example.com", 30, 6000, true},
		{"CAPTCHA", "Please Verify You Are Human", 20, 10000, true},
		{"403", "403 Forbidden", 10, 500, true},
		{"JS验证", "请启用JavaScript以继续访问", 15, 3000, true},
		{"系统检测", "系统检测到异常访问 - example.com", 100, 8000, true},
		{"访问被拒", "访问被拒绝", 20, 5000, true},

		// 内容量比
		{"内容极少的大页面", "Some Title", 150, 20000, true},

		// 正常页面
		{"正常中文文章", "深入理解 Go 语言并发模型", 5000, 30000, false},
		{"正常英文文章", "Understanding Rust's Borrow Checker", 8000, 40000, false},
		{"正常短页面", "Hello World", 300, 2000, false},
		{"正常标题小页面", "About Us", 200, 3000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAntiCrawl(tt.title, tt.extractedLen, tt.bodyLen); got != tt.want {
				t.Errorf("isAntiCrawl(%q, %d, %d) = %v, want %v", tt.title, tt.extractedLen, tt.bodyLen, got, tt.want)
			}
		})
	}
}

// === 测试数据生成：每个函数产出唯一内容的文本（不重复段落） ===

// chineseArticle 生成中文文章（每个段落唯一，无重复）。
func chineseArticle() string {
	paras := []string{
		"互联网的快速发展深刻改变了人们获取信息的方式。从传统的纸质媒体到如今的自媒体平台，信息传播的速度和广度都达到了前所未有的水平。人工智能技术的突破性进展正在重新定义人机交互的基本范式。",
		"深度学习领域的创新不断涌现，Transformer 架构已经成为自然语言处理的主流方法。大语言模型如 GPT、Claude 和 DeepSeek 在文本生成和推理任务上展现了令人瞩目的能力，但同时也带来了计算成本、数据隐私和伦理方面的挑战。",
		"在软件开发领域，开源运动已经成为不可忽视的力量。全球数以百万计的开发者通过 GitHub 等平台协作，共同构建了现代数字基础设施。Linux 操作系统的成功证明了开源模式的生命力，它不仅运行在全球绝大多数服务器上，也是 Android 系统的内核基础。",
		"对于初学者而言，选择适合自己的编程语言至关重要，这取决于个人的兴趣方向和职业规划。Python 以其简洁的语法和丰富的生态系统，成为数据科学和人工智能领域的首选语言。JavaScript 凭借浏览器原生支持和全栈开发能力，在 Web 开发领域占据着不可替代的地位。",
		"Go 语言凭借出色的并发性能和部署便利性，在云计算和微服务架构中广受欢迎。Rust 语言以内存安全著称，正在系统编程领域迅速崛起，甚至被纳入 Linux 内核开发。无论选择哪条技术路线，保持持续学习的热情才是在这个行业中立于不败之地的关键。",
		"数据库技术的发展同样令人瞩目，从传统的关系型数据库到 NoSQL 和 NewSQL，开发者的选择越来越多样化。容器化技术和 Kubernetes 的普及使得应用的部署和运维变得更加标准化和自动化。前端开发框架的迭代速度令人眼花缭乱，React、Vue、Svelte 各有拥趸，但核心理念趋同。",
		"移动应用开发经历了从原生到跨平台的转变，Flutter 和 React Native 大大降低了开发门槛。网络安全的重要性日益凸显，数据泄露和勒索软件攻击的新闻几乎每天都能看到。软件测试已经从手动测试为主转向自动化测试，单元测试、集成测试和端到端测试构成了质量保障的金字塔。",
		"持续集成和持续部署已经成为现代软件开发流程的标准实践，Jenkins 和 GitHub Actions 是常用的工具。程序员需要不断更新自己的知识体系，因为技术栈的更新周期正在变得越来越短。阅读优秀的源代码是提升编程能力的有效方法，许多开源项目的代码质量不亚于教科书示例。",
		"良好的代码可读性往往比极致的性能优化更重要，因为代码被阅读的次数远多于被执行的次数。设计模式是解决常见软件设计问题的经验总结，但过度使用会导致代码变得不必要地复杂。微服务架构虽然带来了灵活性和可扩展性，但也引入了分布式系统固有的复杂性问题。",
		"消息队列在分布式系统中扮演着关键角色，Kafka 和 RabbitMQ 是两种常见的选择。缓存策略的设计直接影响系统性能，Redis 凭借其丰富的数据结构成为最流行的缓存方案。负载均衡是保证服务高可用的基础，Nginx 和 HAProxy 是运维工程师的得力助手。",
		"数据库索引的合理设计可以显著提升查询性能，但过多的索引会降低写入速度。事务管理是保证数据一致性的关键，ACID 和 BASE 代表了两种不同的设计哲学。API 设计需要兼顾易用性和向后兼容性，RESTful 和 GraphQL 各有优势。",
		"监控和告警系统是运维工作的重要组成部分，Prometheus 和 Grafana 的组合被广泛采用。日志管理对于故障排查和安全审计不可或缺，ELK 技术栈提供了完整的解决方案。自动化运维工具如 Ansible 和 Terraform 大大减少了手动操作带来的风险和负担。",
		"软件开发中的技术债务如果长期不处理，会导致代码维护成本呈指数级增长。代码审查不仅有助于发现 bug，更是团队知识分享和技术传承的重要途径。敏捷开发方法论强调迭代和反馈，Scrum 和 Kanban 是最常用的两种框架。",
		"测试驱动开发可以帮助开发者更清晰地思考需求，但也需要根据实际情况灵活应用。软件架构的演进通常从单体应用开始，随着业务增长逐步拆分为微服务。领域驱动设计强调从业务领域出发来组织代码结构，让技术更好地服务于业务。",
		"函数式编程的概念正在逐步渗透到主流语言中，不可变性和纯函数的使用越来越普遍。类型系统的发展使得更多的运行时错误能在编译期被发现，提高了软件的可靠性。响应式编程模型在处理异步数据流方面展现了独特的优势。",
		"WebAssembly 标准的成熟为浏览器端运行高性能代码开辟了新的可能。边缘计算将计算资源推向数据源附近，减少了延迟和带宽成本。物联网设备的普及对嵌入式系统和实时操作系统的开发提出了新的要求。",
		"编译器技术的进步使得程序优化达到新的高度，LLVM 项目为多种语言提供了统一的后端支持。静态分析工具的使用越来越普及，能够在代码进入生产环境之前发现潜在问题。形式化验证虽然成本较高，但在安全攸关的系统中具有重要价值。",
		"面向服务的架构治理需要平衡标准化和灵活性，过度的治理会扼杀创新，而不足的治理会导致混乱。事件驱动架构在解耦系统组件方面展现了强大的能力，但同时也引入了事件溯源的复杂性。领域特定语言在某些场景下可以极大地提高开发效率。",
	}
	return strings.Join(paras, "\n\n")
}

// englishArticle 生成英文文章（每个段落唯一，无重复）。
func englishArticle() string {
	paras := []string{
		"The field of artificial intelligence has undergone remarkable transformations in the past decade. Deep learning models have achieved human-level performance on a variety of benchmarks ranging from image recognition to natural language understanding.",
		"Transformer architectures, introduced in the groundbreaking paper Attention Is All You Need, have become the backbone of modern natural language processing systems. Large language models have demonstrated impressive capabilities in text generation and reasoning tasks.",
		"However, these systems also present significant challenges in terms of computational cost, data privacy, and ethical considerations. Researchers are actively exploring more efficient training methods including mixture of experts architectures and knowledge distillation techniques.",
		"The concept of reinforcement learning from human feedback has proven effective at aligning model outputs with human preferences and values. This approach has been instrumental in making AI systems more helpful, harmless, and honest in their interactions with users.",
		"Software engineering practices have evolved alongside these advances, with tools like GitHub Copilot and Claude Code becoming increasingly popular among professional developers. These AI coding assistants can generate boilerplate code and suggest improvements in real time.",
		"Despite these advances, human oversight remains essential to ensure code quality, security, and alignment with business requirements. The open source movement continues to drive innovation in the developer tools ecosystem with thousands of contributors worldwide.",
		"Version control systems like Git have become universal standards, enabling distributed collaboration at an unprecedented scale. Continuous integration pipelines automatically run thousands of tests on every pull request, catching regressions early in the development cycle.",
		"Container orchestration platforms like Kubernetes have simplified the deployment of distributed applications across hybrid cloud environments. Observability has emerged as a critical discipline, with tools for logging, metrics, and tracing becoming standard components.",
		"The shift towards platform engineering aims to reduce cognitive load on developers by providing self-service infrastructure and tooling. Security practices have shifted left in the development lifecycle, with vulnerability scanning integrated into CI pipelines.",
		"Database technologies continue to diversify, with specialized solutions for time series data, graph relationships, and full text search. The WebAssembly standard is opening new possibilities for running high performance code in browsers and beyond.",
		"Edge computing is bringing computation closer to data sources, reducing latency and bandwidth costs for IoT and real time applications. Programming language design is evolving to provide better safety guarantees while maintaining developer productivity.",
		"The Rust programming language has gained significant adoption in systems programming due to its memory safety guarantees without garbage collection overhead. Functional programming concepts like immutability and pure functions are being incorporated into mainstream languages.",
		"Type systems continue to evolve, with gradual typing approaches bridging the gap between dynamically and statically typed languages. Developer experience has become a key differentiator for platforms, with companies investing heavily in documentation and onboarding materials.",
		"Microservices architecture provides flexibility and independent deployability, but introduces challenges around distributed tracing, eventual consistency, and network reliability. Service meshes have emerged as a solution for managing service-to-service communication.",
		"Event driven architectures enable loose coupling between system components, but require careful design of event schemas and handling of out of order delivery. Domain driven design helps teams align technical architecture with business domain boundaries.",
		"Infrastructure as code tools like Terraform and Pulumi enable reproducible and version controlled infrastructure provisioning. Configuration management with Ansible and Chef automates server setup and reduces manual errors in production environments.",
		"Machine learning operations has become an essential practice for organizations deploying models to production. Model monitoring, versioning, and automated retraining pipelines ensure that AI systems remain accurate and reliable over time.",
	}
	return strings.Join(paras, "\n\n")
}

// mixedTechDoc 生成中英混排技术文档（无重复段落）。
func mixedTechDoc() string {
	paras := []string{
		"## DeepSeek Anthropic 端点使用指南\n\nDeepSeek 提供了与 Anthropic Messages API 兼容的端点，地址为 `https://api.deepseek.com/anthropic/v1/messages`。通过该端点，开发者可以使用 Anthropic SDK 直接调用 DeepSeek 模型，无需修改现有代码。当前支持的模型包括 `deepseek-v4-flash` 和 `deepseek-v4-pro`。",

		"## Web Search 服务端工具\n\nDeepSeek 的 Anthropic 端点支持 `web_search_20260209` 服务端工具，允许模型在推理过程中自动执行网络搜索。工具声明格式遵循 Anthropic 标准协议，服务端会返回 `server_tool_use` 块和 `web_search_tool_result` 块。The result contains structured data including search query, snippets, and source URLs.",

		"## 多轮搜索的实现挑战\n\n在实际使用中我们发现，当模型需要执行多轮搜索时，端点行为与 Anthropic 官方 API 有所不同。Anthropic 官方在服务端循环耗尽时会返回 `stop_reason: \"pause_turn\"`，允许客户端继续对话。However, on DeepSeek endpoint, when the model attempts to continue searching, the XML formatted tool call leaks into the text content block.",

		"## 替代架构\n\n一种可行的替代方案是将搜索循环控制权放在客户端：每次发送单轮搜索请求，获取结果后由客户端判断是否需要继续。The tradeoff is increased latency from multiple HTTP round trips, but the benefit is complete control over search strategy and result filtering.",

		"## API 错误处理\n\nWhen integrating with the Anthropic compatible endpoint, developers should be prepared for protocol translation errors. Common issues include thinking block replay requirements and tool_choice compatibility. DeepSeek 要求在多轮对话中重新传递 reasoning_content，否则会返回 HTTP 400 错误。",

		"## LLM Provider 配置\n\nAnthropic SDK 支持通过 `base_url` 参数自定义 API 端点，这使得切换到 DeepSeek 只需修改两行配置代码。但由于协议翻译层的存在，某些高级特性如 `code_execution` 和 `web_fetch` 服务端工具暂不支持。For basic chat and web search functionality, the integration works reliably.",

		"## 性能对比\n\nBenchmarking results indicate that DeepSeek V4 Flash offers comparable response quality to much larger models at a fraction of the cost. In web search scenarios, the first round of server side search typically completes within two to five seconds depending on query complexity and result volume.",

		"## 安全性考量\n\n使用 Anthropic 兼容端点时，API Key 通过标准 `x-api-key` 头部传递，格式与原生 Anthropic API 完全一致。所有通信均通过 HTTPS 加密传输。需要注意的是，`cache_control` 参数在 DeepSeek 端点上被忽略，Prompt Caching 功能暂不可用。",
	}
	return strings.Join(paras, "\n\n")
}

// markdownWithCode 生成含 markdown 语法和代码块的文档（无重复段落）。
func markdownWithCode() string {
	paras := []string{
		"## API Reference\n\nThis document describes the available API endpoints for the chat completion service. All endpoints require authentication via Bearer token in the Authorization header.",

		"### POST /v1/chat/completions\n\n创建聊天补全请求。\n\n```go\nfunc CreateCompletion(ctx context.Context, client *http.Client, params CompletionParams) (*CompletionResponse, error) {\n    body, err := json.Marshal(params)\n    req, _ := http.NewRequestWithContext(ctx, \"POST\", baseURL+\"/chat/completions\", bytes.NewReader(body))\n    req.Header.Set(\"Content-Type\", \"application/json\")\n    req.Header.Set(\"Authorization\", \"Bearer \"+apiKey)\n    return parseResponse(client.Do(req))\n}\n```",

		"### POST /v1/messages (Anthropic 兼容)\n\n```python\nimport anthropic\nclient = anthropic.Anthropic(\n    base_url=\"https://api.deepseek.com/anthropic/v1/messages\",\n    api_key=\"your-api-key\",\n)\nresponse = client.messages.create(\n    model=\"deepseek-v4-pro\",\n    max_tokens=4096,\n    messages=[{\"role\": \"user\", \"content\": \"Explain quantum computing.\"}],\n)\nprint(response.content[0].text)\n```",

		"### 参数说明\n\n| 参数 | 类型 | 必填 | 默认值 | 说明 |\n|------|------|------|--------|------|\n| model | string | 是 | — | 模型标识符 |\n| messages | array | 是 | — | 对话消息列表 |\n| max_tokens | integer | 是 | — | 最大输出 token 数 |\n| temperature | number | 否 | 1.0 | 采样温度，0 到 2 |\n| top_p | number | 否 | 1.0 | 核采样参数 |\n| tools | array | 否 | [] | 可用工具列表 |\n| stream | boolean | 否 | false | 启用流式响应 |",

		"### Error Handling\n\nThe API returns standard HTTP status codes. 2xx for success, 4xx for client errors, and 5xx for server errors. Detailed error information is provided in the response body as a JSON object with `code`, `message`, and `type` fields.",

		"## Testing Endpoints\n\n```bash\ncurl https://api.deepseek.com/chat/completions \\\n  -H \"Content-Type: application/json\" \\\n  -H \"Authorization: Bearer $DEEPSEEK_API_KEY\" \\\n  -d '{\"model\": \"deepseek-v4-flash\", \"messages\": [{\"role\": \"user\", \"content\": \"Hello\"}]}'\n```",
	}
	return strings.Join(paras, "\n\n")
}

// codeSnippet 生成纯代码文本（无重复文件）。
func codeSnippet() string {
	files := []string{
		`// Package main is the entry point for the API server.
package main

import (
    "fmt"
    "log"
    "net/http"
    "time"
)

func main() {
    mux := http.NewServeMux()
    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        fmt.Fprintln(w, ` + "`" + `{"status":"ok"}` + "`" + `)
    })

    srv := &http.Server{Addr: ":8080", Handler: mux,
        ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second}
    log.Printf("starting server on :8080")
    log.Fatal(srv.ListenAndServe())
}`,

		`package middleware

import (
    "log"
    "net/http"
    "time"
)

type responseWriter struct {
    http.ResponseWriter
    status int
}

func (rw *responseWriter) WriteHeader(code int) {
    rw.status = code
    rw.ResponseWriter.WriteHeader(code)
}

func Logging(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        rw := &responseWriter{ResponseWriter: w, status: 200}
        next.ServeHTTP(rw, r)
        log.Printf("%s %s %d %v", r.Method, r.URL.Path, rw.status, time.Since(start))
    })
}`,

		`package auth

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "net/http"
    "strings"
)

var ErrInvalidToken = errors.New("invalid authentication token")

func Authenticate(r *http.Request, secret []byte) (string, error) {
    token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
    if token == "" {
        return "", ErrInvalidToken
    }
    parts := strings.SplitN(token, ".", 2)
    if len(parts) != 2 {
        return "", ErrInvalidToken
    }
    expected, _ := hex.DecodeString(parts[1])
    mac := hmac.New(sha256.New, secret)
    mac.Write([]byte(parts[0]))
    if !hmac.Equal(mac.Sum(nil), expected) {
        return "", ErrInvalidToken
    }
    return parts[0], nil
}`,

		`package db

import (
    "context"
    "database/sql"
    "fmt"
    _ "github.com/mattn/go-sqlite3"
)

type Store struct{ db *sql.DB }

func NewStore(path string) (*Store, error) {
    db, err := sql.Open("sqlite3", path)
    if err != nil {
        return nil, fmt.Errorf("open: %w", err)
    }
    db.SetMaxOpenConns(1)
    if err := db.Ping(); err != nil {
        return nil, err
    }
    return &Store{db: db}, nil
}

func (s *Store) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
    return s.db.ExecContext(ctx, query, args...)
}`,

		`package config

import (
    "encoding/json"
    "os"
)

type Config struct {
    APIKey      string ` + "`" + `json:"api_key"` + "`" + `
    BaseURL     string ` + "`" + `json:"base_url"` + "`" + `
    Model       string ` + "`" + `json:"model"` + "`" + `
    MaxTokens   int    ` + "`" + `json:"max_tokens"` + "`" + `
}

func Load(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    cfg := &Config{BaseURL: "https://api.deepseek.com", MaxTokens: 4096}
    if err := json.Unmarshal(data, cfg); err != nil {
        return nil, err
    }
    return cfg, nil
}`,
	}
	return strings.Join(files, "\n")
}

// genTextWithFFFD 生成含 U+FFFD 的文本。
func genTextWithFFFD(minBytes int) string {
	var sb strings.Builder
	base := "互联网的发展深刻改变了人们获取信息的方式。人工智能技术的突破性进展正在重新定义人机交互的范式。软件开发领域中开源运动的蓬勃发展，使得全球开发者能够通过协作共同构建现代数字基础设施。"
	for sb.Len() < minBytes {
		sb.WriteString(base)
	}
	text := sb.String()
	mid := len(text) / 2
	return text[:mid] + "���" + text[mid:]
}

// shortBase64 生成指定最大字节数的 base64 文本（用于测试低于阈值的情况）。
func shortBase64(maxBytes int) string {
	buf := make([]byte, (maxBytes*3)/4)
	rand.Read(buf)
	return base64.StdEncoding.EncodeToString(buf)
}

// base64Encrypted 生成模拟加密内容的 base64 文本。
func base64Encrypted(minBytes int) string {
	var parts []string
	total := 0
	for total < minBytes {
		buf := make([]byte, 256)
		rand.Read(buf)
		parts = append(parts, base64.StdEncoding.EncodeToString(buf))
		total += 256
	}
	return strings.Join(parts, "\n")
}

// randomUnicodeText 生成随机 Unicode 文本，模拟编码错误的乱码。
func randomUnicodeText(minBytes int) string {
	var sb strings.Builder
	blocks := []struct{ start, end rune }{
		{0x0080, 0x00FF}, {0x0100, 0x024F}, {0x0370, 0x03FF},
		{0x0400, 0x04FF}, {0x0590, 0x05FF}, {0x0600, 0x06FF},
		{0x0900, 0x097F}, {0x2000, 0x206F}, {0x2100, 0x214F},
		{0x2150, 0x218F}, {0x2460, 0x24FF}, {0x2C00, 0x2C5F},
		{0xA000, 0xA4CF}, {0x4E00, 0x4FFF}, {0xFF00, 0xFFEF},
	}
	for sb.Len() < minBytes {
		block := blocks[randByte()%len(blocks)]
		r := block.start + rune(randUint16())%rune(block.end-block.start)
		sb.WriteRune(r)
	}
	return sb.String()
}

func randByte() int {
	buf := make([]byte, 1)
	rand.Read(buf)
	return int(buf[0])
}

func randUint16() uint16 {
	buf := make([]byte, 2)
	rand.Read(buf)
	return uint16(buf[0])<<8 | uint16(buf[1])
}

func TestIsAntiCrawlHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		cookies []*http.Cookie
		want    bool
	}{
		{
			name:    "无任何防护头",
			headers: map[string]string{"Server": "nginx", "Content-Type": "text/html"},
			want:    false,
		},
		{
			name:    "Cloudflare 验证头 cf-chl-bypass",
			headers: map[string]string{"cf-chl-bypass": "1", "Server": "cloudflare-nginx"},
			want:    true,
		},
		{
			name:    "Sucuri 防火墙",
			headers: map[string]string{"x-sucuri-id": "12345", "Server": "Apache"},
			want:    true,
		},
		{
			name:    "Incapsula 防护",
			headers: map[string]string{"x-iinfo": "blocked", "Server": "nginx"},
			want:    true,
		},
		{
			name:    "Cloudflare 验证 cookie",
			headers: map[string]string{"Server": "cloudflare-nginx"},
			cookies: []*http.Cookie{{Name: "cf_ob_info", Value: "1"}},
			want:    true,
		},
		{
			name:    "Akamai Bot Manager cookie",
			headers: map[string]string{"Server": "Apache"},
			cookies: []*http.Cookie{{Name: "ak_bmsc", Value: "abc123"}},
			want:    true,
		},
		{
			name:    "Cloudflare __cf_bm 不应触发（普适令牌）",
			headers: map[string]string{"Server": "cloudflare"},
			cookies: []*http.Cookie{{Name: "__cf_bm", Value: "token123"}},
			want:    false,
		},
		{
			name:    "正常 Server: cloudflare（不是 cloudflare-nginx）",
			headers: map[string]string{"Server": "cloudflare", "Content-Type": "text/html"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				Header: http.Header{},
			}
			for k, v := range tt.headers {
				resp.Header.Set(k, v)
			}
			// cookies 需要特殊处理
			if len(tt.cookies) > 0 {
				for _, c := range tt.cookies {
					resp.Header.Add("Set-Cookie", c.Name+"="+c.Value)
				}
			}

			got := isAntiCrawlHeaders(resp)
			if got != tt.want {
				t.Errorf("isAntiCrawlHeaders() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCompressionRatios 输出各类文本的压缩率供评估阈值。
func TestCompressionRatios(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"中文文章", chineseArticle()},
		{"英文文章", englishArticle()},
		{"中英混排技术文档", mixedTechDoc()},
		{"Markdown含代码", markdownWithCode()},
		{"纯代码段", codeSnippet()},
		{"base64密文(2K)", base64Encrypted(2000)},
		{"随机Unicode(2K)", randomUnicodeText(2000)},
	}

	t.Logf("compressionMinBytes=%d, compressionThreshold=%.2f", compressionMinBytes, compressionThreshold)
	t.Logf("")
	for _, c := range cases {
		ratio := compressionRatio([]byte(c.text))
		garbled := isEncodingGarbled(c.text)
		bytesLen := len(c.text)
		enough := bytesLen >= compressionMinBytes
		t.Logf("  %-22s  %5d runes  %6d bytes  ratio=%.4f  enough=%v  garbled=%v",
			c.name, len([]rune(c.text)), bytesLen, ratio, enough, garbled)
	}
}
