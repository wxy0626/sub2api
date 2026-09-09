//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardResponses_ForceChatCompletionsRoutesNonStreamingToChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false,"service_tier":"priority"}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_resp_chat_json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_json","object":"chat.completion","model":"gpt-5.4","service_tier":"default","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5,"prompt_tokens_details":{"cached_tokens":1}}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	SetActualOpenAIUpstreamEndpoint(c, "/v1/responses")

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "/v1/chat/completions", GetActualOpenAIUpstreamEndpoint(c))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
	require.Equal(t, "hello", gjson.GetBytes(upstream.lastBody, "messages.0.content").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	promptCacheKey := gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String()
	require.NotEmpty(t, promptCacheKey, "兼容 GPT 请求应向 Chat fallback 传递稳定缓存 key")
	require.Equal(t, generateSessionUUID(promptCacheKey), upstream.lastReq.Header.Get("session_id"), "Chat fallback 必须用缓存 key 固定上游 session_id")
	require.Equal(t, "response", gjson.Get(rec.Body.String(), "object").String())
	require.Equal(t, "ok", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 1, result.Usage.CacheReadInputTokens)
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, "priority", *result.ServiceTier)
	require.Equal(t, "default", result.UpstreamResponseServiceTier)
	require.False(t, result.Stream)
}

func TestResolveResponsesFallbackPromptCacheKey_PreservesExplicitAndIsolatesAutoKey(t *testing.T) {
	account := rawChatCompletionsTestAccount()
	chatReq := &apicompat.ChatCompletionsRequest{
		Model: "gpt-6-astra",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: json.RawMessage("\\\"hello\\\"")},
		},
	}
	responsesReq := &apicompat.ResponsesRequest{Model: "gpt-6-astra"}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Set("api_key", &APIKey{ID: 701})

	key1, auto1 := resolveResponsesFallbackPromptCacheKey(c, account, responsesReq, chatReq, "gpt-6-astra")
	key2, auto2 := resolveResponsesFallbackPromptCacheKey(c, account, responsesReq, chatReq, "gpt-6-astra")
	require.True(t, auto1)
	require.True(t, auto2)
	require.NotEmpty(t, key1)
	require.Equal(t, key1, key2, "同一账号、同一稳定前缀必须复用缓存 key")

	c.Set("api_key", &APIKey{ID: 702})
	keyOtherTenant, autoOtherTenant := resolveResponsesFallbackPromptCacheKey(c, account, responsesReq, chatReq, "gpt-6-astra")
	require.True(t, autoOtherTenant)
	require.NotEqual(t, key1, keyOtherTenant, "不同 API Key 不得共享自动生成的缓存 key")

	responsesReq.PromptCacheKey = "client-cache-key"
	c.Set("api_key", &APIKey{ID: 701})
	explicit, autoExplicit := resolveResponsesFallbackPromptCacheKey(c, account, responsesReq, chatReq, "gpt-6-astra")
	require.False(t, autoExplicit)
	require.Equal(t, "client-cache-key", explicit, "客户端显式 key 必须原样保留")
}

// Scenario: 第三方无推理模型不收到兼容档位。
func TestForwardResponses_ForceChatCompletionsOmitsNoneReasoningEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"company-coding-model","input":"hello","reasoning":{"effort":"none"},"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_none","object":"chat.completion","model":"company-coding-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "company-coding-model", gjson.GetBytes(upstream.lastBody, "model").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "reasoning_effort").Exists())
	require.Nil(t, result.ReasoningEffort)
}

func TestForwardResponses_PassthroughFlagWithUnsupportedResponsesUsesAccountMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("/v1/responses", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.4-channel","input":"hello","stream":false}`)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"id":"chatcmpl_mapping","object":"chat.completion","model":"gpt-5.4-account","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			)),
		}}
		svc := &OpenAIGatewayService{
			cfg:          rawChatCompletionsTestConfig(),
			httpUpstream: upstream,
		}
		account := rawChatCompletionsTestAccount()
		account.Credentials["model_mapping"] = map[string]any{
			"gpt-5.4-channel": "gpt-5.4-account",
		}
		account.Extra = map[string]any{
			"openai_passthrough":                     true,
			openai_compat.ExtraKeyResponsesSupported: false,
		}

		result, err := svc.Forward(context.Background(), c, account, body)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
		require.Equal(t, "gpt-5.4-account", gjson.GetBytes(upstream.lastBody, "model").String())
	})

	// 压缩请求即使在 passthrough + responses_supported=false 账号上也禁止
	// 回落 raw Chat Completions（完整历史会被重新展开、再次触发
	// context_length_exceeded）；最终边界必须显式拒绝。
	t.Run("/v1/responses/compact", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.4-channel","input":"hello","stream":false}`)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		upstream := &httpUpstreamRecorder{}
		svc := &OpenAIGatewayService{
			cfg:          rawChatCompletionsTestConfig(),
			httpUpstream: upstream,
		}
		account := rawChatCompletionsTestAccount()
		account.Credentials["model_mapping"] = map[string]any{
			"gpt-5.4-channel": "gpt-5.4-account",
		}
		account.Credentials["compact_model_mapping"] = map[string]any{
			"gpt-5.4-account": "gpt-5.4-compact",
		}
		account.Extra = map[string]any{
			"openai_passthrough":                     true,
			openai_compat.ExtraKeyResponsesSupported: false,
		}

		result, err := svc.Forward(context.Background(), c, account, body)
		require.Error(t, err)
		require.Nil(t, result)
		require.Empty(t, upstream.requests, "compact must not call /chat/completions")
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		require.Equal(t, "responses_compaction_not_supported", gjson.Get(rec.Body.String(), "error.type").String())
	})
}

func TestForwardResponses_ForceChatCompletionsRoutesStreamingToChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"he"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"llo"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_resp_chat_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream_options.include_usage").Bool())
	promptCacheKey := gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String()
	require.NotEmpty(t, promptCacheKey, "Astra fallback 必须携带稳定缓存 key")
	require.Equal(t, generateSessionUUID(promptCacheKey), upstream.lastReq.Header.Get("session_id"), "Astra fallback 必须用缓存 key 固定上游 session_id")
	require.Contains(t, rec.Body.String(), "event: response.output_text.delta")
	require.Contains(t, rec.Body.String(), `"delta":"he"`)
	require.Contains(t, rec.Body.String(), "event: response.completed")
	require.Contains(t, rec.Body.String(), `"input_tokens":4`)
	require.Contains(t, rec.Body.String(), "data: [DONE]")
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.True(t, result.Stream)
	require.NotNil(t, result.FirstTokenMs)
}

func TestForwardResponses_ForceChatCompletionsContextWindow400KeepsClientError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"large prompt","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	// 模拟截图中 Chat fallback 上游返回的英文上下文超限详情。
	const upstreamMessage = "prompt is too long: Your input exceeds the context window of this model"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_resp_chat_context"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"` + upstreamMessage + `","type":"invalid_request_error"}}`)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)

	require.Error(t, err)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "确定性上下文超限不得触发账号切换")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
	require.Equal(t, "context_length_exceeded", gjson.Get(rec.Body.String(), "error.code").String())
	require.Contains(t, gjson.Get(rec.Body.String(), "error.message").String(), "请求内容超过模型的上下文窗口")
	require.Contains(t, gjson.Get(rec.Body.String(), "error.message").String(), upstreamMessage)
	require.Len(t, upstream.requests, 1, "确定性上下文超限不得重试或切换账号")
}

func TestForwardResponses_DeepSeekReasoningOnlyStreamProducesVisibleText(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-reasoner","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_reasoning","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"role":"assistant","content":null,"reasoning_content":""},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_reasoning","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"reasoning_content":"visible fallback"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_reasoning","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"content":""},"finish_reason":"length"}],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_deepseek_reasoning_responses_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Contains(t, rec.Body.String(), "event: response.output_text.delta")
	require.Contains(t, rec.Body.String(), `"delta":"visible fallback"`)
	require.Contains(t, rec.Body.String(), `"status":"incomplete"`)
	require.Contains(t, rec.Body.String(), "data: [DONE]")
}

func TestForwardResponses_AutoSupportedAccountStillUsesResponsesEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_resp_native"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_native","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}],"status":"completed"}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeAuto),
		openai_compat.ExtraKeyResponsesSupported: true,
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://upstream.example/v1/responses", upstream.lastReq.URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "messages").Exists())
	require.Equal(t, "ok", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
}

func TestForwardResponses_CompactRequestNeverFallsBackToChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		path   string
		body   []byte
		header string
	}{
		{
			name: "explicit compact path",
			path: "/v1/responses/compact",
			body: []byte(`{"model":"gpt-5.4","input":[{"type":"compaction_trigger"}],"stream":false}`),
		},
		{
			name:   "native remote compaction v2",
			path:   "/v1/responses",
			body:   []byte(`{"model":"gpt-5.4","input":[{"type":"compaction_trigger"}],"stream":true}`),
			header: "remote_compaction_v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			if tt.header != "" {
				c.Request.Header.Set("x-codex-beta-features", tt.header)
			}

			upstream := &httpUpstreamRecorder{}
			svc := &OpenAIGatewayService{
				cfg:          rawChatCompletionsTestConfig(),
				httpUpstream: upstream,
			}

			result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), tt.body)
			require.Error(t, err)
			require.Nil(t, result)
			require.Empty(t, upstream.requests, "compact must not call /chat/completions")
			require.Equal(t, http.StatusServiceUnavailable, rec.Code)
			require.Equal(t, "responses_compaction_not_supported", gjson.Get(rec.Body.String(), "error.type").String())
			require.Contains(t, gjson.Get(rec.Body.String(), "error.message").String(), "会话压缩请求必须使用上游 Responses API")
		})
	}
}

func forceChatResponsesFallbackAccount() *Account {
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
	}
	return account
}

// reasoningRecordingCache 记录 reasoning 缓存写入、并按需响应回查。
type reasoningRecordingCache struct {
	stubGatewayCache
	mu      sync.Mutex
	sets    map[string]string
	getResp map[string]string
}

func (c *reasoningRecordingCache) SetReasoningContent(_ context.Context, itemID string, content string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sets == nil {
		c.sets = make(map[string]string)
	}
	c.sets[itemID] = content
	return nil
}

func (c *reasoningRecordingCache) GetReasoningContent(_ context.Context, itemID string) (string, error) {
	if v, ok := c.getResp[itemID]; ok {
		return v, nil
	}
	return "", ErrReasoningContentNotFound
}

func (c *reasoningRecordingCache) snapshotSets() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]string, len(c.sets))
	for k, v := range c.sets {
		out[k] = v
	}
	return out
}

// 流式响应里的 reasoning_content 应按 reasoning item id 写入缓存，供后续轮次
// 客户端不回传明文 summary 时回注（DeepSeek thinking mode 400 修复的写入侧）。
func TestForwardResponses_ChatFallbackCachesStreamedReasoning(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-reasoner","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_rc","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_rc","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"reasoning_content":"think "},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_rc","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"reasoning_content":"first"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_rc","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"content":"answer"},"finish_reason":"stop"}]}`,
		"",
		`data: {"id":"chatcmpl_rc","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_reasoning_cache_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	cache := &reasoningRecordingCache{}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
		cache:        cache,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)

	sets := cache.snapshotSets()
	require.Len(t, sets, 1, "应恰好缓存一个 reasoning item")
	for itemID, content := range sets {
		require.NotEmpty(t, itemID)
		require.Equal(t, "think first", content)
	}
}

// 请求侧：encrypted-only reasoning item（无明文 summary）经缓存回查补回
// reasoning_content；带明文 summary 的 item 顺手回写缓存（自愈）。
func TestForwardResponses_ChatFallbackRestoresReasoningFromCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"deepseek-reasoner",
		"stream":false,
		"input":[
			{"type":"reasoning","id":"item_plain","summary":[{"type":"summary_text","text":"plain thinking"}]},
			{"type":"function_call","call_id":"call_0","name":"get_value","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_0","output":"ok"},
			{"type":"reasoning","id":"item_enc1","summary":[],"encrypted_content":"opaque"},
			{"type":"function_call","call_id":"call_1","name":"get_value","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_1","output":"ok"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"go on"}]}
		]
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_reasoning_cache_restore"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_restore","object":"chat.completion","model":"deepseek-reasoner","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
		)),
	}}
	cache := &reasoningRecordingCache{
		getResp: map[string]string{"item_enc1": "cached thinking"},
	}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
		cache:        cache,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)

	// 明文 summary 的 assistant 工具调用消息：reasoning_content 来自 summary 本身。
	require.Equal(t, "plain thinking", gjson.GetBytes(upstream.lastBody, "messages.0.reasoning_content").String())
	require.Equal(t, "call_0", gjson.GetBytes(upstream.lastBody, "messages.0.tool_calls.0.id").String())
	require.Equal(t, "tool", gjson.GetBytes(upstream.lastBody, "messages.1.role").String())
	// encrypted-only 的 assistant 工具调用消息：reasoning_content 来自缓存回查。
	require.Equal(t, "cached thinking", gjson.GetBytes(upstream.lastBody, "messages.2.reasoning_content").String())
	require.Equal(t, "call_1", gjson.GetBytes(upstream.lastBody, "messages.2.tool_calls.0.id").String())
	require.Equal(t, "tool", gjson.GetBytes(upstream.lastBody, "messages.3.role").String())

	// 明文 summary 的 item 被回写进缓存（自愈）。
	require.Equal(t, "plain thinking", cache.snapshotSets()["item_plain"])
}

// TestForwardResponses_ChatFallbackEmptyStreamFailsOver 验证 CC fallback 流式路径的
// 静默空流防御：上游只回 role chunk + 空 content 收尾（无 usage、无语义输出）时，
// 必须转成 UpstreamFailoverError 交 handler 换号重试，且客户端 0 字节（可透明重放）。
// 对应 napi.origintask.cn gpt-6-astra 坏路径与原生路径 issue #5009。
func TestForwardResponses_ChatFallbackEmptyStreamFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-6-astra","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_empty","object":"chat.completion.chunk","model":"gpt-6-astra","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_empty","object":"chat.completion.chunk","model":"gpt-6-astra","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_cc_empty_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	_, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "空流必须产生 UpstreamFailoverError, got: %v", err)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Empty(t, rec.Body.String(), "空流不得向客户端写出任何字节，否则 handler 无法透明换号重试")
}

// TestForwardResponses_ChatFallbackEmptyStreamWithUsageSucceeds 镜像原生路径语义：
// 空内容但上游报了非零 usage 时不视为静默拒答，照常交付（由上层按 0 输出计费）。
func TestForwardResponses_ChatFallbackEmptyStreamWithUsageSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-6-astra","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_usage","object":"chat.completion.chunk","model":"gpt-6-astra","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_usage","object":"chat.completion.chunk","model":"gpt-6-astra","choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop"}]}`,
		"",
		`data: {"id":"chatcmpl_usage","object":"chat.completion.chunk","model":"gpt-6-astra","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":0,"total_tokens":4}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_cc_empty_usage"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 0, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), "data: [DONE]")
}

// TestForwardResponses_ChatFallbackEmptyJSONResponseFailsOver 验证非流式路径的
// 静默空响应防御：无语义输出且无 usage 的 200 JSON 必须转成 failover 错误。
func TestForwardResponses_ChatFallbackEmptyJSONResponseFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-6-astra","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_cc_empty_json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_empty_json","object":"chat.completion","model":"gpt-6-astra","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}]}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.Error(t, err)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "空 JSON 响应必须产生 UpstreamFailoverError, got: %v", err)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Empty(t, rec.Body.String(), "空 JSON 响应不得向客户端写出任何字节")
}

// TestForwardResponses_ChatFallbackEmptyJSONResponseWithUsageSucceeds 非流式镜像：
// 空 content 但带非零 usage 时正常交付。
func TestForwardResponses_ChatFallbackEmptyJSONResponseWithUsageSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-6-astra","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_cc_empty_json_usage"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_empty_json_usage","object":"chat.completion","model":"gpt-6-astra","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":0,"total_tokens":3}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 0, result.Usage.OutputTokens)
}
