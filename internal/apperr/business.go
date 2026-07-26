package apperr

import "fmt"

// 通用业务错误码补充
const (
	// CodeNotFound 业务资源不存在（跨模块通用）。
	CodeNotFound Code = "not_found"
)

// BusinessError 是无领域专属字段的业务错误通用载体。
// 实现 Coder 接口，供 CodeFromError 直接读取 Code。
//
// 判断标准：后端是否需要领域专属字段（如 githubapi.Error.ResetAt / llm.APIError.Retryable）。
// 需要→领域 error；不需要→BusinessError。
//
// 注意：字段名 CodeVal 而非 Code，是为了避免与 Coder 接口的 Code() 方法同名冲突
// （Go 不允许同类型字段与方法同名）。
//
// 警示：勿用 BusinessError 包装实现了 Coder 的领域 error——errors.As 会先匹配外层
// BusinessError，底层领域 Code 被遮蔽。想保留领域分类用 fmt.Errorf("...: %w", err) 包装。
type BusinessError struct {
	CodeVal Code // apperr.CodeInvalid / apperr.CodeNotFound / ...
	Msg     string
	Cause   error
}

// Error 实现 error 接口。Cause 非 nil 时拼接 Cause 文本，便于追踪原始错误。
func (e *BusinessError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Cause)
	}
	return e.Msg
}

// Unwrap 暴露 Cause，配合 errors.Is / errors.As 穿透包装链。
func (e *BusinessError) Unwrap() error { return e.Cause }

// Code 实现 Coder 接口。
// 自保：若直接用 struct literal 构造绕过 NewInvalid/NewNotFound，CodeVal 字段未赋值取零值 CodeOK，
// 这里降级为 CodeInternal，防止错误被静默吞成成功（呼应 CodeFromError 的 CodeOK 不变量）。
func (e *BusinessError) Code() Code {
	if e.CodeVal == CodeOK {
		return CodeInternal
	}
	return e.CodeVal
}

// NewInvalid 构造入参非法类业务错误。
func NewInvalid(msg string) *BusinessError {
	return &BusinessError{CodeVal: CodeInvalid, Msg: msg}
}

// NewNotFound 构造业务资源不存在类业务错误。
func NewNotFound(msg string) *BusinessError {
	return &BusinessError{CodeVal: CodeNotFound, Msg: msg}
}
