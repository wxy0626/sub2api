//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// deepSeekTestAccount 构造带测试密钥的 DeepSeek API Key 账号。
func deepSeekTestAccount() *Account {
	return &Account{
		ID:       301,
		Platform: PlatformDeepSeek,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "sk-deepseek-test",
		},
	}
}

func TestDeepSeekAccountUsesAPIKeyAndDefaultBaseURL(t *testing.T) {
	account := deepSeekTestAccount()

	require.True(t, account.IsDeepSeek())
	require.Equal(t, "sk-deepseek-test", account.GetOpenAIApiKey())
	require.Equal(t, "https://api.deepseek.com", account.GetDeepSeekBaseURL())
	require.Equal(t, "https://api.deepseek.com", account.GetOpenAIBaseURL())

	account.Credentials["base_url"] = " https://proxy.example/deepseek/ "
	require.Equal(t, "https://proxy.example/deepseek", account.GetDeepSeekBaseURL())
}

func TestDeepSeekResponsesCapabilityIsModelScoped(t *testing.T) {
	account := deepSeekTestAccount()

	require.True(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions))
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponses))
	require.False(t, account.ShouldUseOpenAIResponsesForModel("deepseek-chat"))
	require.False(t, account.ShouldUseOpenAIResponsesForModel("deepseek-reasoner"))
	require.True(t, account.ShouldUseOpenAIResponsesForModel(DeepSeekResponsesModel))
	require.True(t, account.ShouldUseOpenAIResponsesForModel("DEEPSEEK-V4-FLASH"))
	require.False(t, account.ShouldUseOpenAIResponsesForModel("deepseek-v4-pro"))

	account.Credentials["model_mapping"] = map[string]any{"deepseek-flash-alias": DeepSeekResponsesModel}
	require.True(t, account.ShouldUseOpenAIResponsesForModel("deepseek-flash-alias"))
}

func TestDeepSeekOnlyAcceptsAPIKeyAccountType(t *testing.T) {
	require.NoError(t, ValidateAccountPlatformType(PlatformDeepSeek, AccountTypeAPIKey))
	err := ValidateAccountPlatformType(PlatformDeepSeek, AccountTypeOAuth)
	require.Error(t, err)
	require.ErrorContains(t, err, "DeepSeek")
	require.ErrorContains(t, err, "仅支持 API Key")
	require.ErrorContains(t, err, "credentials.api_key")
}

func TestDeepSeekChatCompletionsURLUsesV1Endpoint(t *testing.T) {
	account := deepSeekTestAccount()
	require.Equal(t,
		"https://api.deepseek.com/v1/chat/completions",
		buildOpenAIChatCompletionsURL(account.GetDeepSeekBaseURL()),
	)
	require.Equal(t,
		"https://api.deepseek.com/v1/responses",
		buildOpenAIResponsesURL(account.GetDeepSeekBaseURL()),
	)
}

func TestDeepSeekV4FlashBuildsNativeResponsesRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"deepseek-v4-flash","input":"hello"}`)))

	service := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
	account := deepSeekTestAccount()
	request, err := service.buildUpstreamRequest(context.Background(), c, account, []byte(`{"model":"deepseek-v4-flash","input":"hello"}`), "sk-deepseek-test", false, "", false)
	require.NoError(t, err)
	require.Equal(t, "https://api.deepseek.com/v1/responses", request.URL.String())
	require.Equal(t, "Bearer sk-deepseek-test", request.Header.Get("Authorization"))
}

func TestDeepSeekOrdinaryChatRequestDoesNotEnterResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"deepseek-reasoner","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl_ds","object":"chat.completion","model":"deepseek-reasoner","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)),
	}}
	service := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := deepSeekTestAccount()

	result, err := service.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://api.deepseek.com/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-deepseek-test", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "deepseek-reasoner", gjson.GetBytes(upstream.lastBody, "model").String())
}

func TestDeepSeekAccountTestUsesChatCompletionsAndPreservesUpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"invalid deepseek key","type":"authentication_error"}}`)),
	}}
	service := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	err := service.testDeepSeekAccountConnection(ctx, deepSeekTestAccount(), "deepseek-chat", "hello", "")
	require.Error(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://api.deepseek.com/v1/chat/completions", upstream.lastReq.URL.String())
	require.Contains(t, err.Error(), "通过 /v1/chat/completions 测试 DeepSeek 连接失败")
	require.Contains(t, err.Error(), "HTTP 401")
	require.Contains(t, err.Error(), "invalid deepseek key")
	require.Contains(t, recorder.Body.String(), "/v1/chat/completions")
	require.Contains(t, recorder.Body.String(), "invalid deepseek key")
	require.NotContains(t, err.Error(), "sk-deepseek-test")
	require.NotContains(t, recorder.Body.String(), "sk-deepseek-test")
}

// TestDeepSeekV4FlashResponsesTestSuccess 验证 deepseek-v4-flash 的手动 Responses 测试使用原生端点。
func TestDeepSeekV4FlashResponsesTestSuccess(t *testing.T) {
	ctx, recorder := newTestContext()
	upstream := &queuedHTTPUpstream{responses: []*http.Response{
		newJSONResponse(http.StatusOK, "{\"id\":\"resp_ds\",\"status\":\"completed\",\"output\":[]}"),
	}}
	service := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	err := service.testDeepSeekAccountConnection(ctx, deepSeekTestAccount(), DeepSeekResponsesModel, "hello", AccountTestModeResponses)
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://api.deepseek.com/v1/responses", upstream.requests[0].URL.String())
	require.Equal(t, "Bearer sk-deepseek-test", upstream.requests[0].Header.Get("Authorization"))
	require.Contains(t, recorder.Body.String(), "DeepSeek Responses 诊断响应")
	require.Contains(t, recorder.Body.String(), "\"success\":true")
}

// TestDeepSeekV4FlashResponsesTestErrors 验证 Responses 测试的配置、网络错误均可执行且不泄露密钥。
func TestDeepSeekV4FlashResponsesTestErrors(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		upstream *queuedHTTPUpstream
		want     []string
	}{
		{name: "missing api key", account: func() *Account { a := deepSeekTestAccount(); delete(a.Credentials, "api_key"); return a }(), want: []string{"DeepSeek API Key 未配置", "credentials.api_key"}},
		{name: "invalid base url", account: func() *Account { a := deepSeekTestAccount(); a.Credentials["base_url"] = "://invalid"; return a }(), want: []string{"DeepSeek Base URL 无效", "credentials.base_url", "原始技术详情"}},
		{name: "transport", account: deepSeekTestAccount(), upstream: &queuedHTTPUpstream{errors: []error{errors.New("dial tcp: responses connection refused")}}, want: []string{"通过 /v1/responses 测试 DeepSeek 连接失败", "responses connection refused", "原始技术详情"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, recorder := newTestContext()
			service := &AccountTestService{
				httpUpstream: tt.upstream,
				cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
			}

			err := service.testDeepSeekAccountConnection(ctx, tt.account, DeepSeekResponsesModel, "hello", AccountTestModeResponses)
			require.Error(t, err)
			for _, part := range tt.want {
				require.Contains(t, err.Error(), part)
				require.Contains(t, recorder.Body.String(), part)
			}
			require.NotContains(t, err.Error(), "sk-deepseek-test")
			require.NotContains(t, recorder.Body.String(), "sk-deepseek-test")
		})
	}
}

// TestDeepSeekV4FlashResponsesTestHTTPErrorPreservesUpstreamDetails 验证 Responses HTTP 错误保留原始上游详情。
func TestDeepSeekV4FlashResponsesTestHTTPErrorPreservesUpstreamDetails(t *testing.T) {
	ctx, recorder := newTestContext()
	upstream := &queuedHTTPUpstream{responses: []*http.Response{
		newJSONResponse(http.StatusUnauthorized, "{\"error\":{\"message\":\"invalid deepseek responses key\",\"type\":\"authentication_error\"}}"),
	}}
	service := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	err := service.testDeepSeekAccountConnection(ctx, deepSeekTestAccount(), DeepSeekResponsesModel, "hello", AccountTestModeResponses)
	require.Error(t, err)
	require.Equal(t, "https://api.deepseek.com/v1/responses", upstream.requests[0].URL.String())
	require.Contains(t, err.Error(), "通过 /v1/responses 测试 DeepSeek 连接失败")
	require.Contains(t, err.Error(), "HTTP 401")
	require.Contains(t, err.Error(), "invalid deepseek responses key")
	require.Contains(t, recorder.Body.String(), "invalid deepseek responses key")
	require.NotContains(t, err.Error(), "sk-deepseek-test")
	require.NotContains(t, recorder.Body.String(), "sk-deepseek-test")
}

func TestDeepSeekAccountTestChatCompletionsConfigAndTransportErrors(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		upstream *queuedHTTPUpstream
		want     []string
	}{
		{name: "missing api key", account: func() *Account { a := deepSeekTestAccount(); delete(a.Credentials, "api_key"); return a }(), want: []string{"DeepSeek API Key 未配置", "credentials.api_key"}},
		{name: "invalid base url", account: func() *Account { a := deepSeekTestAccount(); a.Credentials["base_url"] = "://invalid"; return a }(), want: []string{"DeepSeek Base URL 无效", "credentials.base_url", "原始技术详情"}},
		{name: "transport", account: deepSeekTestAccount(), upstream: &queuedHTTPUpstream{errors: []error{errors.New("dial tcp: connection refused")}}, want: []string{"通过 /v1/chat/completions 测试 DeepSeek 连接失败", "connection refused", "原始技术详情"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, recorder := newTestContext()
			service := &AccountTestService{httpUpstream: tt.upstream, cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}}}
			err := service.testDeepSeekAccountConnection(ctx, tt.account, "deepseek-chat", "hello", "")
			require.Error(t, err)
			for _, part := range tt.want {
				require.Contains(t, err.Error(), part)
				require.Contains(t, recorder.Body.String(), part)
			}
			require.NotContains(t, err.Error(), "sk-deepseek-test")
			require.NotContains(t, recorder.Body.String(), "sk-deepseek-test")
		})
	}
}

func TestDeepSeekAccountTestChatCompletionsParseError(t *testing.T) {
	ctx, recorder := newTestContext()
	service := &AccountTestService{httpUpstream: &queuedHTTPUpstream{responses: []*http.Response{newJSONResponse(http.StatusOK, "data: {broken-json}\n")}}, cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}}}
	err := service.testDeepSeekAccountConnection(ctx, deepSeekTestAccount(), "deepseek-chat", "hello", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "DeepSeek Chat Completions 响应解析失败")
	require.Contains(t, err.Error(), "原始技术详情")
	require.Contains(t, recorder.Body.String(), "invalid character")
	require.NotContains(t, recorder.Body.String(), "sk-deepseek-test")
}

func TestDeepSeekMessagesOrdinaryModelUsesChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"deepseek-reasoner","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl_ds_messages","object":"chat.completion","model":"deepseek-reasoner","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)),
	}}
	service := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := service.ForwardAsAnthropic(context.Background(), c, deepSeekTestAccount(), body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://api.deepseek.com/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "deepseek-reasoner", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Contains(t, recorder.Body.String(), "ok")
	require.NotContains(t, upstream.lastReq.URL.String(), "/responses")
}

// newDeepSeekForwardTestContext 创建调用 Forward 所需的 Responses 测试请求上下文。
func newDeepSeekForwardTestContext(body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, recorder
}

// TestDeepSeekV4FlashForwardUsesNativeResponsesAndSemanticSSE 验证 DeepSeek Responses 原生流式转发。
func TestDeepSeekV4FlashForwardUsesNativeResponsesAndSemanticSSE(t *testing.T) {
	body := []byte(`{"model":"deepseek-v4-flash","input":"hello","stream":true}`)
	c, recorder := newDeepSeekForwardTestContext(body)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid-deepseek-responses"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.output_text.delta",
			`data: {"type":"response.output_text.delta","delta":"hello"}`,
			"",
			"event: response.completed",
			`data: {"type":"response.completed","response":{"id":"resp_ds","status":"completed","output":[]}}`,
			"",
		}, "\n"))),
	}}
	service := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := service.Forward(context.Background(), c, deepSeekTestAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://api.deepseek.com/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-deepseek-test", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "deepseek-v4-flash", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "input").String(), "hello")
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.Contains(t, recorder.Body.String(), "response.output_text.delta")
	require.Contains(t, recorder.Body.String(), `"delta":"hello"`)
	require.Contains(t, recorder.Body.String(), "response.completed")
	require.NotContains(t, recorder.Body.String(), "data: [DONE]")
}

// TestDeepSeekOrdinaryResponsesForwardUsesChatCompletions 验证普通 DeepSeek 模型从 Responses 入口分流到 Chat Completions。
func TestDeepSeekOrdinaryResponsesForwardUsesChatCompletions(t *testing.T) {
	models := []string{"deepseek-chat", "deepseek-reasoner"}
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":%q,"input":"hello","stream":false}`, model))
			c, recorder := newDeepSeekForwardTestContext(body)
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl_ds_fallback","object":"chat.completion","model":"` + model + `","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)),
			}}
			service := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			result, err := service.Forward(context.Background(), c, deepSeekTestAccount(), body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, result.Stream)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "https://api.deepseek.com/v1/chat/completions", upstream.lastReq.URL.String())
			require.Equal(t, "Bearer sk-deepseek-test", upstream.lastReq.Header.Get("Authorization"))
			require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, "hello", gjson.GetBytes(upstream.lastBody, "messages.0.content").String())
			require.Contains(t, recorder.Body.String(), `"object":"response"`)
			require.Contains(t, recorder.Body.String(), "ok")
		})
	}
}

// TestDeepSeekV4FlashForwardAcceptsResponseIncomplete 验证 DeepSeek Responses 的 incomplete 终止事件能正常收尾。
func TestDeepSeekV4FlashForwardAcceptsResponseIncomplete(t *testing.T) {
	body := []byte(`{"model":"deepseek-v4-flash","input":"hello","stream":true}`)
	c, recorder := newDeepSeekForwardTestContext(body)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"partial"}`,
			"",
			`data: {"type":"response.incomplete","response":{"id":"resp_ds_incomplete","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`,
			"",
		}, "\n"))),
	}}
	service := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := service.Forward(context.Background(), c, deepSeekTestAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Contains(t, recorder.Body.String(), "response.output_text.delta")
	require.Contains(t, recorder.Body.String(), `"delta":"partial"`)
	require.Contains(t, recorder.Body.String(), "response.incomplete")
}

// TestDeepSeekV4FlashForwardPropagatesResponseFailed 验证 Responses failed 终止事件返回错误并保留上游详情。
func TestDeepSeekV4FlashForwardPropagatesResponseFailed(t *testing.T) {
	body := []byte(`{"model":"deepseek-v4-flash","input":"hello","stream":true}`)
	c, recorder := newDeepSeekForwardTestContext(body)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"partial"}`,
			"",
			`event: response.failed`,
			`data: {"type":"response.failed","response":{"id":"resp_ds_failed","status":"failed","error":{"code":"server_error","message":"DeepSeek failed"}}}`,
			"",
		}, "\n"))),
	}}
	service := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := service.Forward(context.Background(), c, deepSeekTestAccount(), body)
	require.Error(t, err)
	// 已经向下游写出部分内容后，当前 Forward 契约允许仅返回错误而不返回结果对象。
	if result != nil {
		require.True(t, result.Stream)
	}
	require.Contains(t, err.Error(), "DeepSeek failed")
	require.Contains(t, recorder.Body.String(), "response.failed")
	require.Contains(t, recorder.Body.String(), "DeepSeek failed")
}
