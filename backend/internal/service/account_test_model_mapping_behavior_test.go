//go:build unit

package service

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// accountTestMappedOpenAIAccount 构造带别名映射的 OpenAI APIKey 账号。
func accountTestMappedOpenAIAccount(id int64, mapping map[string]any) *Account {
	return &Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":       "sk-test",
			"base_url":      "https://compat-upstream.example/v1",
			"model_mapping": mapping,
		},
	}
}

// TestAccountTestService_OpenAIResponsesDiagnosticUsesMappedModel 验证
// 显式 Responses 诊断也发送映射后的上游真名，而不是客户端别名。
func TestAccountTestService_OpenAIResponsesDiagnosticUsesMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()

	upstream := &httpUpstreamRecorder{resp: newJSONResponse(http.StatusOK, `{"id":"resp_test","status":"completed"}`)}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := accountTestMappedOpenAIAccount(331, map[string]any{"luna-alias": "gpt-5.6-luna"})

	err := svc.testOpenAIAccountConnection(ctx, account, "luna-alias", "", AccountTestModeResponses)

	require.NoError(t, err)
	require.Equal(t, "https://compat-upstream.example/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "gpt-5.6-luna", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Contains(t, recorder.Body.String(), "Codex++ 诊断响应")
}

// TestAccountTestService_GeminiUsesWildcardMappedModel 验证 Gemini APIKey 账号
// 的通配符映射在测试中同样生效。
func TestAccountTestService_GeminiUsesWildcardMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()

	upstream := &httpUpstreamRecorder{resp: newJSONResponse(http.StatusOK, `data: {"candidates":[{"content":{"parts":[{"text":"gemini ok"}]}}]}

`)}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          332,
		Platform:    PlatformGemini,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":       "gemini-test",
			"model_mapping": map[string]any{"gemini-flash-*": "gemini-3.1-pro-high"},
		},
	}

	err := svc.testGeminiAccountConnection(ctx, account, "gemini-flash-latest", "hello")

	require.NoError(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Contains(t, upstream.lastReq.URL.Path, "gemini-3.1-pro-high")
	require.Contains(t, recorder.Body.String(), `"model":"gemini-3.1-pro-high"`)
}
