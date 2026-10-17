package openaichat

import (
	"net"
	"net/http"
	"time"
)

// newHTTPClient 创建流式请求专用 http.Client：
//   - ResponseHeaderTimeout: 10m 首字节超时（不含 body 读取）
//   - DialContext: 10s TCP 连接超时
//   - Client.Timeout: 0（不限制整体超时，body 读取由 ctx 控制）
//
// 注意：绝不能用 http.Client.Timeout，会切断 DeepSeek 思考模型长输出
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout: 10 * time.Second,
			}).DialContext,
			ResponseHeaderTimeout: 10 * time.Minute,
		},
	}
}
