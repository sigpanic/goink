package adapter

import "github.com/sigpanic/goink/internal/apperr"

// llm 模块错误码
const (
	// CodeRateLimited LLM 调用触发 429。
	CodeRateLimited apperr.Code = "llm.rate_limited"
	// CodeNotFound LLM 模型不存在（404）。
	CodeNotFound apperr.Code = "llm.not_found"
	// CodeForbidden LLM 调用权限拒绝（403）。
	CodeForbidden apperr.Code = "llm.forbidden"
	// CodeServerError LLM 服务端 5xx，归类为可重试服务端问题。
	CodeServerError apperr.Code = "llm.server_error"
	// CodeClientError LLM 客户端 4xx（非 429/404/403），归类为请求侧问题。
	CodeClientError apperr.Code = "llm.client_error"
)
