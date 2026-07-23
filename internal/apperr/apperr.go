// Package apperr 提供应用层错误码基础设施，对标 storage.PageResult[T]，
// 用于 Wails API 统一返回 *Result[T]，前端通过 ErrCode 分类反馈。
//
// 设计目标：
//   - 作为基础设施：提供泛型 Result[T] 包装，让需要分类错误反馈的 Wails API 有统一形态
//   - 零侵入：不强制重构现有 API，只在新增 API 上启用；旧 API 保持原签名
//   - 可扩展：错误码用字符串而非 int 枚举，便于后续新增场景直接加值
//   - 后端语义保留：CodeFromError 通过 errors.As 提取底层 *githubapi.Error / *llm.APIError，
//     不丢失原始 Kind/StatusCode 信息
//
// 错误码按模块分区组织（通用 / githubapi / llm）。同样语义（如 404）在不同模块
// 有不同错误码字符串（githubapi.not_found vs llm.not_found），便于前端针对模块做
// 差异化反馈：githubapi 404 通常表示 skill 被移除，llm 404 表示模型不存在。
//
// 使用示例：
//
//	func (a *App) ListRemoteSkills(...) *apperr.Result[*storage.PageResult[remote.RemoteSkillMeta]] {
//	    all, err := a.remoteService.ListRemoteSkills(a.ctx, false)
//	    if err != nil {
//	        return apperr.Err[*storage.PageResult[remote.RemoteSkillMeta]](err)
//	    }
//	    // ...
//	    return apperr.Ok(page)
//	}
package apperr

import (
	"errors"
	"strings"

	"github.com/sigpanic/goink/internal/githubapi"
	"github.com/sigpanic/goink/internal/llm"
)

// Code 是应用层错误码，作为前端契约的稳定标识。
// 空字符串 "" 表示成功（零值），其他值对应不同错误类别。
// 一旦发布即成为前端契约，后续不可改名（只能新增）。
type Code string

// 通用错误码（跨模块共享）
const (
	// CodeOK 无错误（默认零值）。Result[T] 零值的 ErrCode 为空，前端 if (res.err_code) 自然 falsy 检查。
	CodeOK Code = ""
	// CodeInternal 其他未分类错误（默认 fallback）。
	CodeInternal Code = "internal"
	// CodeInvalid 入参非法（target 不是 user/novel、novelID=0 等）。
	CodeInvalid Code = "invalid"
)

// githubapi 模块错误码
const (
	// CodeGitHubAPINetwork 网络层失败：超时 / 连接拒绝 / DNS / context 取消。
	CodeGitHubAPINetwork Code = "githubapi.network"
	// CodeGitHubAPIRateLimited rate limit 触发：GitHub 403+Remaining:0、429。
	CodeGitHubAPIRateLimited Code = "githubapi.rate_limited"
	// CodeGitHubAPINotFound 资源不存在（404，如 skill 被移除）。
	CodeGitHubAPINotFound Code = "githubapi.not_found"
	// CodeGitHubAPIForbidden 权限拒绝（403 非 rate limit）。
	CodeGitHubAPIForbidden Code = "githubapi.forbidden"
	// CodeGitHubAPIOther 其他未分类的 GitHub API 错误。
	CodeGitHubAPIOther Code = "githubapi.other"
)

// llm 模块错误码
const (
	// CodeLLMRateLimited LLM 调用触发 429。
	CodeLLMRateLimited Code = "llm.rate_limited"
	// CodeLLMNotFound LLM 模型不存在（404）。
	CodeLLMNotFound Code = "llm.not_found"
	// CodeLLMForbidden LLM 调用权限拒绝（403）。
	CodeLLMForbidden Code = "llm.forbidden"
	// CodeLLMServerError LLM 服务端 5xx，归类为可重试服务端问题。
	CodeLLMServerError Code = "llm.server_error"
	// CodeLLMClientError LLM 客户端 4xx（非 429/404/403），归类为请求侧问题。
	CodeLLMClientError Code = "llm.client_error"
)

// Result 是 Wails API 的统一返回包装，泛型 T 表示业务数据类型。
//
// 成功时 ErrCode=""，失败时 Data 为零值、ErrCode 非 ""、ErrMsg 含可读描述。
// 始终序列化为 HTTP 200，业务错误体现在 ErrCode 字段，避免前端用字符串匹配区分错误类型。
//
// T 可以是 *storage.PageResult[X]、string、apperr.Empty 等任意类型。
type Result[T any] struct {
	Data    T      `json:"data"`              // 业务数据（失败时为零值）
	ErrCode Code   `json:"err_code"`          // 空字符串表示成功
	ErrMsg  string `json:"err_msg,omitempty"` // 人类可读错误描述（仅失败时存在）
}

// Ok 包装成功结果。
func Ok[T any](data T) *Result[T] {
	return &Result[T]{Data: data}
}

// Err 包装失败结果，自动通过 CodeFromError 推断错误码。
// ErrMsg 直接使用 err.Error() 原文，前端按 ErrCode 选择对应的 i18n 文案展示。
func Err[T any](err error) *Result[T] {
	return &Result[T]{
		ErrCode: CodeFromError(err),
		ErrMsg:  err.Error(),
	}
}

// Empty 用于无业务数据返回的场景（如 InstallRemoteSkill）。
// 类型别名而非新定义类型，便于 Wails 绑定生成器输出干净的 TS 类型。
type Empty = struct{}

// CodeFromError 只做调度，具体映射逻辑在各模块专属函数里。
// 同一个语义（如 404）在不同模块前端反馈可能不同，故错误码带模块前缀。
//
// 注意：remote.Service 用 fmt.Errorf("remote: ...: %w", err) 包装，
// errors.As 能穿透多层 wrap 直达底层，比字符串前缀匹配可靠。
func CodeFromError(err error) Code {
	if err == nil {
		return CodeOK
	}
	var ghErr *githubapi.Error
	if errors.As(err, &ghErr) {
		return codeFromGitHubAPIError(ghErr)
	}
	var llmErr *llm.APIError
	if errors.As(err, &llmErr) {
		return codeFromLLMAPIError(llmErr)
	}
	// 业务层 fmt.Errorf("remote: invalid target %q", target) 等约定
	msg := err.Error()
	if strings.Contains(msg, "invalid") || strings.Contains(msg, "requires non-zero") {
		return CodeInvalid
	}
	return CodeInternal
}

// codeFromGitHubAPIError 映射 *githubapi.Error.Kind → apperr.Code。
// githubapi 的 404 表示仓库文件不存在（如 skill 被移除），前端反馈应区别于 llm 404。
func codeFromGitHubAPIError(err *githubapi.Error) Code {
	switch err.Kind {
	case githubapi.KindNetwork:
		return CodeGitHubAPINetwork
	case githubapi.KindRateLimited:
		return CodeGitHubAPIRateLimited
	case githubapi.KindNotFound:
		return CodeGitHubAPINotFound
	case githubapi.KindForbidden:
		return CodeGitHubAPIForbidden
	default:
		return CodeGitHubAPIOther
	}
}

// codeFromLLMAPIError 映射 *llm.APIError.StatusCode → apperr.Code。
// llm 的 404 表示模型不存在，前端反馈应区别于 githubapi 404。
func codeFromLLMAPIError(err *llm.APIError) Code {
	switch {
	case err.StatusCode == 429:
		return CodeLLMRateLimited
	case err.StatusCode == 404:
		return CodeLLMNotFound
	case err.StatusCode == 403:
		return CodeLLMForbidden
	case err.StatusCode >= 500:
		return CodeLLMServerError
	default:
		return CodeLLMClientError
	}
}
