package apperr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sigpanic/goink/internal/apperr"
	"github.com/sigpanic/goink/internal/githubapi"
	"github.com/sigpanic/goink/internal/llm"
)

// TestOk_ConstructsSuccess 验证 Ok 包装成功结果时 ErrCode 为空、ErrMsg 缺省、Data 透传。
func TestOk_ConstructsSuccess(t *testing.T) {
	res := apperr.Ok("hello")
	require.NotNil(t, res)
	assert.Equal(t, "hello", res.Data)
	assert.Equal(t, apperr.CodeOK, res.ErrCode)
	assert.Empty(t, res.ErrMsg, "成功响应 ErrMsg 应为空，便于 omitempty 序列化")
}

// TestOk_EmptyType 验证用 Empty 类型构造无数据成功响应。
func TestOk_EmptyType(t *testing.T) {
	res := apperr.Ok(apperr.Empty{})
	require.NotNil(t, res)
	assert.Equal(t, apperr.Empty{}, res.Data)
	assert.Equal(t, apperr.CodeOK, res.ErrCode)
}

// TestErr_GitHubAPIError_KindMapping 验证 *githubapi.Error 各 Kind → 对应 Code。
// 覆盖 KindNetwork/KindRateLimited/KindNotFound/KindForbidden/KindOther 五类。
func TestErr_GitHubAPIError_KindMapping(t *testing.T) {
	cases := []struct {
		name string
		kind githubapi.Kind
		want apperr.Code
	}{
		{"network", githubapi.KindNetwork, apperr.CodeGitHubAPINetwork},
		{"rate_limited", githubapi.KindRateLimited, apperr.CodeGitHubAPIRateLimited},
		{"not_found", githubapi.KindNotFound, apperr.CodeGitHubAPINotFound},
		{"forbidden", githubapi.KindForbidden, apperr.CodeGitHubAPIForbidden},
		{"other_fallback", githubapi.KindOther, apperr.CodeGitHubAPIOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ghErr := &githubapi.Error{Kind: tc.kind, Message: "msg"}
			res := apperr.Err[string](ghErr)
			require.NotNil(t, res)
			assert.Equal(t, tc.want, res.ErrCode)
			assert.Equal(t, "", res.Data, "失败时 Data 应为零值")
		})
	}
}

// TestErr_LLMAPIError_StatusMapping 验证 *llm.APIError 各 StatusCode → 对应 Code。
// 覆盖 429/404/403/500/400 五类状态码。
func TestErr_LLMAPIError_StatusMapping(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		want       apperr.Code
	}{
		{"429_rate_limited", 429, apperr.CodeLLMRateLimited},
		{"404_not_found", 404, apperr.CodeLLMNotFound},
		{"403_forbidden", 403, apperr.CodeLLMForbidden},
		{"500_server_error", 500, apperr.CodeLLMServerError},
		{"400_client_error", 400, apperr.CodeLLMClientError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apiErr := &llm.APIError{StatusCode: tc.statusCode, Message: "msg"}
			res := apperr.Err[string](apiErr)
			require.NotNil(t, res)
			assert.Equal(t, tc.want, res.ErrCode)
		})
	}
}

// TestErr_WrappedError_PreservesKind 验证 errors.As 穿透 fmt.Errorf %w 包装直达底层 *githubapi.Error。
func TestErr_WrappedError_PreservesKind(t *testing.T) {
	ghErr := &githubapi.Error{Kind: githubapi.KindNetwork, Message: "request failed"}
	wrapped := fmt.Errorf("remote: fetch: %w", ghErr)
	res := apperr.Err[string](wrapped)
	require.NotNil(t, res)
	assert.Equal(t, apperr.CodeGitHubAPINetwork, res.ErrCode, "errors.As 应穿透 %w 包装")
	assert.Contains(t, res.ErrMsg, "remote: fetch:")
	assert.Contains(t, res.ErrMsg, "request failed")
}

// TestErr_WrappedError_PreservesLLMStatusCode 验证 errors.As 对 *llm.APIError 的穿透。
func TestErr_WrappedError_PreservesLLMStatusCode(t *testing.T) {
	apiErr := &llm.APIError{StatusCode: 404, Message: "model not found"}
	wrapped := fmt.Errorf("llm: call: %w", apiErr)
	res := apperr.Err[string](wrapped)
	require.NotNil(t, res)
	assert.Equal(t, apperr.CodeLLMNotFound, res.ErrCode)
}

// TestErr_InvalidTarget_String 验证业务层 "invalid" 约定字符串映射到 CodeInvalid。
func TestErr_InvalidTarget_String(t *testing.T) {
	err := errors.New(`remote: invalid target "foo"`)
	res := apperr.Err[string](err)
	require.NotNil(t, res)
	assert.Equal(t, apperr.CodeInvalid, res.ErrCode)
}

// TestErr_RequiresNonZero_String 验证 "requires non-zero" 约定字符串映射到 CodeInvalid。
func TestErr_RequiresNonZero_String(t *testing.T) {
	err := errors.New("remote: install to novel layer requires non-zero novelID")
	res := apperr.Err[string](err)
	require.NotNil(t, res)
	assert.Equal(t, apperr.CodeInvalid, res.ErrCode)
}

// TestErr_UnknownError_Fallback 验证未识别错误 fallback 到 CodeInternal。
func TestErr_UnknownError_Fallback(t *testing.T) {
	err := errors.New("something weird")
	res := apperr.Err[string](err)
	require.NotNil(t, res)
	assert.Equal(t, apperr.CodeInternal, res.ErrCode)
	assert.Equal(t, "something weird", res.ErrMsg)
}

// TestCodeFromError_Nil 验证 nil 错误返回 CodeOK。
func TestCodeFromError_Nil(t *testing.T) {
	assert.Equal(t, apperr.CodeOK, apperr.CodeFromError(nil))
}

// TestErr_GenericClass_Instantiation 验证嵌套泛型类型能正常实例化 Err[T]。
func TestErr_GenericClass_Instantiation(t *testing.T) {
	err := errors.New("network down")
	resMap := apperr.Err[map[string]any](err)
	require.NotNil(t, resMap)
	assert.Equal(t, apperr.CodeInternal, resMap.ErrCode)
	assert.Nil(t, resMap.Data)
	resSlice := apperr.Err[[]string](err)
	require.NotNil(t, resSlice)
	assert.Equal(t, apperr.CodeInternal, resSlice.ErrCode)
	assert.Nil(t, resSlice.Data)
	type Skill struct{ Name string }
	resPtr := apperr.Err[*Skill](err)
	require.NotNil(t, resPtr)
	assert.Equal(t, apperr.CodeInternal, resPtr.ErrCode)
	assert.Nil(t, resPtr.Data)
	resNested := apperr.Err[*[]map[string]any](err)
	require.NotNil(t, resNested)
	assert.Equal(t, apperr.CodeInternal, resNested.ErrCode)
	assert.Nil(t, resNested.Data)
}

// TestCode_StringValues 前端契约稳定性测试：错误码字符串值不可变。
func TestCode_StringValues(t *testing.T) {
	assert.Equal(t, "", string(apperr.CodeOK))
	assert.Equal(t, "internal", string(apperr.CodeInternal))
	assert.Equal(t, "invalid", string(apperr.CodeInvalid))
	assert.Equal(t, "githubapi.network", string(apperr.CodeGitHubAPINetwork))
	assert.Equal(t, "githubapi.rate_limited", string(apperr.CodeGitHubAPIRateLimited))
	assert.Equal(t, "githubapi.not_found", string(apperr.CodeGitHubAPINotFound))
	assert.Equal(t, "githubapi.forbidden", string(apperr.CodeGitHubAPIForbidden))
	assert.Equal(t, "githubapi.other", string(apperr.CodeGitHubAPIOther))
	assert.Equal(t, "llm.rate_limited", string(apperr.CodeLLMRateLimited))
	assert.Equal(t, "llm.not_found", string(apperr.CodeLLMNotFound))
	assert.Equal(t, "llm.forbidden", string(apperr.CodeLLMForbidden))
	assert.Equal(t, "llm.server_error", string(apperr.CodeLLMServerError))
	assert.Equal(t, "llm.client_error", string(apperr.CodeLLMClientError))
}
