package llm

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/sigpanic/goink/internal/llm/adapter"
)

// Client 是 LLM 流式调用的传输层。
// 按协议选择适配器，各适配器内部持有和解析自己的 providers。
type Client struct {
	configMu sync.Mutex // 串行化配置构建、写盘和发布，不阻塞请求读取已有适配器。
	mu       sync.RWMutex
	adapters map[adapter.Protocol]adapter.ProtocolAdapter
	http     *http.Client
	logger   *slog.Logger
}

// NewClient 从用户配置创建协议级适配器，模板合并和运行时 provider 组装由具体子包完成。
func NewClient(config *UserLLMConfig, log *slog.Logger) (*Client, error) {
	c := &Client{http: newHTTPClient(), logger: log}
	if err := c.Reload(config); err != nil {
		return nil, err
	}
	return c, nil
}

// newHTTPClient 创建流式请求专用 http.Client：
//   - ResponseHeaderTimeout: 10m 首字节超时（不含 body 读取）
//   - DialContext: 10s TCP 连接超时
//   - Client.Timeout: 0（不限制整体超时，body 读取由 ctx 控制）
//
// 注意：绝不能用 http.Client.Timeout，会切断 DeepSeek 思考模型长输出
func newHTTPClient() *http.Client {
	client := &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout: 10 * time.Second,
			}).DialContext,
			ResponseHeaderTimeout: 10 * time.Minute,
		},
	}
	client.Transport = withHTTPDebug(client.Transport)
	return client
}

// Reload 热更新配置，构建失败时保留现有适配器；进行中的请求继续使用旧实例。
func (c *Client) Reload(config *UserLLMConfig) error {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	adapters, err := buildAdapters(config, c.http, c.logger)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.adapters = adapters
	return nil
}

// SaveConfig 先构建并验证新实例，再保存配置；保存成功后才替换运行时实例。
func (c *Client) SaveConfig(path string, config *UserLLMConfig) error {
	c.configMu.Lock()
	defer c.configMu.Unlock()

	adapters, err := buildAdapters(config, c.http, c.logger)
	if err != nil {
		return err
	}
	if err := SaveUserConfig(path, config); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.adapters = adapters
	return nil
}

func (c *Client) protocolAdapter(adapterName adapter.Protocol) (adapter.ProtocolAdapter, error) {
	c.mu.RLock()
	backend, ok := c.adapters[adapterName]
	c.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown adapter: %s", adapterName)
	}
	return backend, nil
}
