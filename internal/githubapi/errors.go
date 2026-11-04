package githubapi

import "github.com/sigpanic/goink/internal/apperr"

// githubapi 模块错误码
const (
	// CodeNetwork 网络层失败：超时 / 连接拒绝 / DNS / context 取消。
	CodeNetwork apperr.Code = "githubapi.network"
	// CodeRateLimited rate limit 触发：GitHub 403+Remaining:0、429。
	CodeRateLimited apperr.Code = "githubapi.rate_limited"
	// CodeNotFound 资源不存在（404，如 skill 被移除）。
	CodeNotFound apperr.Code = "githubapi.not_found"
	// CodeForbidden 权限拒绝（403 非 rate limit）。
	CodeForbidden apperr.Code = "githubapi.forbidden"
	// CodeOther 其他未分类的 GitHub API 错误。
	CodeOther apperr.Code = "githubapi.other"
)
