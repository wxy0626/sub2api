package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type claude55RawUpstreamRecorder struct {
	lastBody []byte
	resp     *http.Response
}

func (u *claude55RawUpstreamRecorder) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	if req != nil && req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(body))
		u.lastBody = body
	}
	return u.resp, nil
}

func (u *claude55RawUpstreamRecorder) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestNormalizeClaude55ChatReasoning(t *testing.T) {
	tests := []struct {
		name        string
		model       string
		effort      string
		body        string
		wantChanged bool
		wantThink   string
		wantEffort  string
	}{
		{
			name:        "sonnet5.5 xhigh gets output effort",
			model:       "anthropic/claude-sonnet-5-5",
			effort:      "xhigh",
			body:        `{"model":"anthropic/claude-sonnet-5-5","reasoning_effort":"xhigh","messages":[]}`,
			wantChanged: true,
			wantEffort:  "xhigh",
		},
		{
			name:        "opus5.5 max gets output effort",
			model:       "claude-opus-5-5",
			effort:      "max",
			body:        `{"model":"claude-opus-5-5","reasoning_effort":"max","messages":[]}`,
			wantChanged: true,
			wantEffort:  "max",
		},
		{
			name:        "effort alias is canonicalized",
			model:       "anthropic/claude-sonnet-5-5",
			effort:      "x-high",
			body:        `{"model":"anthropic/claude-sonnet-5-5","reasoning_effort":"x-high","messages":[]}`,
			wantChanged: true,
			wantEffort:  "xhigh",
		},
		{
			name:        "minimal effort still writes output effort",
			model:       "anthropic/claude-sonnet-5-5",
			effort:      "minimal",
			body:        `{"model":"anthropic/claude-sonnet-5-5","reasoning_effort":"minimal","messages":[]}`,
			wantChanged: true,
			wantEffort:  "minimal",
		},
		{
			name:        "legacy enabled thinking is removed without effort",
			model:       "anthropic/claude-sonnet-5-5",
			effort:      "",
			body:        `{"model":"anthropic/claude-sonnet-5-5","thinking":{"type":"enabled"},"messages":[]}`,
			wantChanged: true,
		},
		{
			name:        "non 5.5 model untouched",
			model:       "anthropic/claude-sonnet-4-5",
			effort:      "xhigh",
			body:        `{"model":"anthropic/claude-sonnet-4-5","reasoning_effort":"xhigh","messages":[]}`,
			wantChanged: false,
		},
		{
			name:        "none effort does not enable thinking",
			model:       "anthropic/claude-sonnet-5-5",
			effort:      "none",
			body:        `{"model":"anthropic/claude-sonnet-5-5","messages":[]}`,
			wantChanged: false,
		},
		{
			name:        "empty effort does not enable thinking",
			model:       "anthropic/claude-sonnet-5-5",
			effort:      "",
			body:        `{"model":"anthropic/claude-sonnet-5-5","messages":[]}`,
			wantChanged: false,
		},
		{
			name:        "explicit thinking is removed and effort is preserved",
			model:       "anthropic/claude-sonnet-5-5",
			effort:      "xhigh",
			body:        `{"model":"anthropic/claude-sonnet-5-5","thinking":{"type":"adaptive"},"output_config":{"effort":"high"}}`,
			wantChanged: true,
			wantEffort:  "high",
		},
		{
			name:        "sibling output_config fields survive",
			model:       "anthropic/claude-sonnet-5-5",
			effort:      "medium",
			body:        `{"model":"anthropic/claude-sonnet-5-5","output_config":{"format":{"type":"json_schema"}}}`,
			wantChanged: true,
			wantEffort:  "medium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed, err := normalizeClaude55ChatReasoning([]byte(tt.body), tt.model, tt.effort)
			require.NoError(t, err)
			require.Equal(t, tt.wantChanged, changed)
			require.Equal(t, tt.wantThink, gjson.GetBytes(got, "thinking.type").String())
			require.Equal(t, tt.wantEffort, gjson.GetBytes(got, "output_config.effort").String())
			if isClaude55SignedThinkingModel(tt.model) {
				require.False(t, gjson.GetBytes(got, "reasoning_effort").Exists())
			}
		})
	}
}

func TestNormalizeClaude55ChatReasoningKeepsSiblingOutputConfig(t *testing.T) {
	body := []byte(`{"output_config":{"format":{"type":"json_schema"}}}`)
	got, changed, err := normalizeClaude55ChatReasoning(body, "anthropic/claude-sonnet-5-5", "high")
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "json_schema", gjson.GetBytes(got, "output_config.format.type").String())
}

func TestResolveClaude55ChatReasoningEffortPreservesMinimal(t *testing.T) {
	body := []byte(`{"reasoning":{"effort":"minimal"}}`)
	require.Equal(t, "minimal", resolveClaude55ChatReasoningEffort(body, nil))
}

func TestForwardAsRawChatCompletions_NormalizesClaude55ReasoningForUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"anthropic/claude-sonnet-5-5","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"xhigh","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &claude55RawUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_claude55_effort"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl_claude55","object":"chat.completion","model":"anthropic/claude-sonnet-5-5","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`)),
	}}
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{
					Enabled:           false,
					AllowInsecureHTTP: true,
				},
			},
		},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          153,
		Name:        "Cindy2api",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "http://upstream.example",
		},
	}

	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "thinking").Exists())
	require.Equal(t, "xhigh", gjson.GetBytes(upstream.lastBody, "output_config.effort").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "reasoning_effort").Exists())
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "xhigh", *result.ReasoningEffort)
}

func TestForwardResponsesViaRawChatCompletions_NormalizesClaude55MinimalReasoning(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"anthropic/claude-sonnet-5-5","input":"reply ok","reasoning":{"effort":"minimal"},"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &claude55RawUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_claude55_minimal"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_claude55_minimal","object":"chat.completion","model":"anthropic/claude-sonnet-5-5","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{
					Enabled:           false,
					AllowInsecureHTTP: true,
				},
			},
		},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          153,
		Name:        "Cindy2api",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "http://upstream.example",
		},
	}

	result, err := svc.forwardResponsesViaRawChatCompletions(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "thinking").Exists())
	require.Equal(t, "minimal", gjson.GetBytes(upstream.lastBody, "output_config.effort").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "reasoning_effort").Exists())
}
