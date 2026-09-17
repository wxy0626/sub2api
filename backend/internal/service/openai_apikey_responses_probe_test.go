package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
)

func TestProbeOpenAIAPIKeyResponsesSupportUsesCodexProbeHeaders(t *testing.T) {
	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          96,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://compat-upstream.example/v1",
		},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"output":[{"type":"function_call","name":"probe_ping","namespace":"functions.collaboration"}]}`)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	svc.ProbeOpenAIAPIKeyResponsesSupport(context.Background(), account.ID)

	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://compat-upstream.example/v1/responses", upstream.lastReq.URL.String())
	requireOpenAICodexProbeHeaders(t, upstream.lastReq.Header)
	// 探测载荷必须用 namespace 容器包裹工具，才能暴露"透传会丢 namespace"的上游。
	require.Contains(t, string(upstream.lastBody), `"type":"namespace"`)
	require.Contains(t, string(upstream.lastBody), `"name":"functions.collaboration"`)
	updates := <-updateCalls
	require.Equal(t, true, updates[openai_compat.ExtraKeyResponsesSupported])
}

func TestProbeOpenAIAPIKeyResponsesSupportCNProviders(t *testing.T) {
	tests := []struct {
		name        string
		id          int64
		platform    string
		protocol    string
		wantSupport bool
		wantMode    string
	}{
		{name: "deepseek adaptive supports responses", id: 201, platform: PlatformDeepseek, protocol: APIProtocolAdaptive, wantSupport: true, wantMode: string(openai_compat.ResponsesSupportModeForceResponses)},
		{name: "deepseek chat clears forced responses", id: 202, platform: PlatformDeepseek, protocol: APIProtocolChatCompletions, wantSupport: false, wantMode: string(openai_compat.ResponsesSupportModeAuto)},
		{name: "kimi adaptive supports responses", id: 203, platform: PlatformKimi, protocol: APIProtocolAdaptive, wantSupport: true, wantMode: string(openai_compat.ResponsesSupportModeForceResponses)},
		{name: "kimi responses protocol supports responses", id: 205, platform: PlatformKimi, protocol: APIProtocolResponses, wantSupport: true, wantMode: string(openai_compat.ResponsesSupportModeForceResponses)},
		{name: "zhipu adaptive falls back to chat", id: 204, platform: PlatformZhipu, protocol: APIProtocolAdaptive, wantSupport: false, wantMode: string(openai_compat.ResponsesSupportModeAuto)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updateCalls := make(chan map[string]any, 1)
			account := Account{
				ID: tc.id, Platform: tc.platform, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "sk-test", "api_protocol": tc.protocol},
				Extra: map[string]any{
					openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses),
				},
			}
			repo := &snapshotUpdateAccountRepo{
				stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
				updateExtraCalls:      updateCalls,
			}
			svc := &AccountTestService{accountRepo: repo}

			svc.ProbeOpenAIAPIKeyResponsesSupport(context.Background(), account.ID)

			updates := <-updateCalls
			require.Equal(t, tc.wantSupport, updates[openai_compat.ExtraKeyResponsesSupported])
			require.Equal(t, tc.wantMode, updates[openai_compat.ExtraKeyResponsesMode])
		})
	}
}

func TestDecideResponsesProbeSupport(t *testing.T) {
	// 判定基准：function_call 必须带与探测声明一致的 namespace 字段。
	nsFnCall := []byte(`{"output":[{"type":"reasoning"},{"type":"function_call","name":"probe_ping","namespace":"functions.collaboration"}]}`)
	// 上游自行摊平（回传扁平名、缺 namespace 字段）时 Codex 无法路由执行器。
	flatFnCall := []byte(`{"output":[{"type":"function_call","name":"functions.collaboration__probe_ping"}]}`)
	reasoningOnly := []byte(`{"output":[{"type":"reasoning"}]}`)

	cases := []struct {
		name   string
		status int
		body   []byte
		want   bool
	}{
		// Endpoint clearly absent on third-party OpenAI-compatible upstreams.
		{"404 endpoint absent", 404, nsFnCall, false},
		{"405 method not allowed", 405, nsFnCall, false},
		// 2xx: tool capability is judged by presence of a function_call output
		// item with the declared namespace field.
		{"200 with namespaced function_call", 200, nsFnCall, true},
		{"200 flat function_call without namespace", 200, flatFnCall, false},
		// Volcengine Ark coding/v3 × kimi-k2.6: reasoning only, no function_call.
		{"200 reasoning only", 200, reasoningOnly, false},
		{"200 invalid json", 200, []byte("not-json"), false},
		{"200 no output field", 200, []byte(`{"status":"completed"}`), false},
		// Non-2xx (other than 404/405): endpoint exists, capability undecidable -> conservative true.
		{"400 conservative true", 400, reasoningOnly, true},
		{"401 conservative true", 401, nil, true},
		{"500 conservative true", 500, nil, true},
		// 422 with explicit Responses rejection -> unsupported (GLM-style).
		{"422 chinese responses unsupported", 422, []byte(`{"error":{"message":"该模型不支持 /v1/responses"}}`), false},
		{"422 english responses unsupported", 422, []byte(`{"error":{"message":"The model does not support the responses API"}}`), false},
		{"422 responses unsupported short", 422, []byte(`{"error":{"message":"responses unsupported"}}`), false},
		// 422 without Responses rejection -> conservative true.
		{"422 unrelated validation", 422, []byte(`{"error":{"message":"Invalid model ID"}}`), true},
		{"422 no message", 422, []byte(`{}`), true},
		// 400/422 明确拒绝工具类型（litellm/智谱链路）→ 不支持，走内建 CC 桥。
		{"400 tool type illegal", 400, []byte(`{"error":{"message":"tools[0].type:type is illegal"}}`), false},
		{"400 unrelated tool-free error", 400, []byte(`{"error":{"message":"Invalid model ID"}}`), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, decideResponsesProbeSupport(tc.status, tc.body))
		})
	}
}

func TestResponsesProbeBodyRejectsResponses(t *testing.T) {
	cases := []struct {
		name string
		body []byte
		want bool
	}{
		{"chinese unsupported", []byte(`{"error":{"message":"该模型不支持 /v1/responses"}}`), true},
		{"english not support", []byte(`{"error":{"message":"The model does not support responses"}}`), true},
		{"english unsupported", []byte(`{"error":{"message":"Responses API is unsupported"}}`), true},
		{"uppercase", []byte(`{"error":{"message":"RESPONSES API IS NOT SUPPORTED"}}`), true},
		{"message field", []byte(`{"message":"responses not supported"}`), true},
		{"raw body fallback", []byte(`this responses api is unsupported`), true},
		{"missing responses keyword", []byte(`{"error":{"message":"Model not supported"}}`), false},
		{"missing rejection signal", []byte(`{"error":{"message":"Bad responses request"}}`), false},
		{"empty body", []byte(``), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, responsesProbeBodyRejectsResponses(tc.body))
		})
	}
}

func TestResponsesProbeBodyHasFunctionCall(t *testing.T) {
	require.True(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[{"type":"function_call"}]}`)))
	require.True(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[{"type":"reasoning"},{"type":"function_call"}]}`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[{"type":"reasoning"}]}`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[]}`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`{}`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`garbage`)))
}

func TestResponsesProbeBodyHasNamespaceFunctionCall(t *testing.T) {
	require.True(t, responsesProbeBodyHasNamespaceFunctionCall([]byte(
		`{"output":[{"type":"function_call","name":"probe_ping","namespace":"functions.collaboration"}]}`)))
	require.True(t, responsesProbeBodyHasNamespaceFunctionCall([]byte(
		`{"output":[{"type":"reasoning"},{"type":"function_call","name":"probe_ping","namespace":"functions.collaboration"}]}`)))
	// 缺 namespace 字段（上游自行摊平）→ Codex 无法路由，判 false。
	require.False(t, responsesProbeBodyHasNamespaceFunctionCall([]byte(
		`{"output":[{"type":"function_call","name":"probe_ping"}]}`)))
	// namespace 不匹配 → 判 false。
	require.False(t, responsesProbeBodyHasNamespaceFunctionCall([]byte(
		`{"output":[{"type":"function_call","name":"probe_ping","namespace":"functions.other"}]}`)))
	require.False(t, responsesProbeBodyHasNamespaceFunctionCall([]byte(`{"output":[]}`)))
	require.False(t, responsesProbeBodyHasNamespaceFunctionCall([]byte(`{}`)))
}

func TestResponsesProbeBodyRejectsToolType(t *testing.T) {
	cases := []struct {
		name string
		body []byte
		want bool
	}{
		{"litellm illegal type", []byte(`{"error":{"message":"tools[0].type:type is illegal"}}`), true},
		{"english unsupported tool type", []byte(`{"error":{"message":"Unsupported tool type: namespace"}}`), true},
		{"chinese not support", []byte(`{"error":{"message":"不支持该工具类型"}}`), true},
		// 与工具无关的错误不触发（避免误判模型/鉴权类 400/422）。
		{"invalid model", []byte(`{"error":{"message":"Invalid model ID"}}`), false},
		{"auth error", []byte(`{"error":{"message":"Incorrect API key"}}`), false},
		{"empty", []byte(``), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, responsesProbeBodyRejectsToolType(tc.body))
		})
	}
}

func TestSelectResponsesProbeModel(t *testing.T) {
	// No model_mapping -> fall back to DefaultTestModel (OpenAI official APIKey).
	require.Equal(t, openai.DefaultTestModel, selectResponsesProbeModel(&Account{}))

	// model_mapping values are upstream models; pick first by sort for reproducibility.
	acct := &Account{Credentials: map[string]any{
		"model_mapping": map[string]any{
			"client-b": "zeta-model",
			"client-a": "alpha-model",
		},
	}}
	require.Equal(t, "alpha-model", selectResponsesProbeModel(acct))

	// Wildcard / blank upstream values are skipped.
	acctWild := &Account{Credentials: map[string]any{
		"model_mapping": map[string]any{
			"a": "*",
			"b": "  ",
			"c": "real-model",
		},
	}}
	require.Equal(t, "real-model", selectResponsesProbeModel(acctWild))

	// Only wildcard mappings -> DefaultTestModel.
	acctAllWild := &Account{Credentials: map[string]any{
		"model_mapping": map[string]any{"a": "gpt-*"},
	}}
	require.Equal(t, openai.DefaultTestModel, selectResponsesProbeModel(acctAllWild))
}
