package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIResponsesRejectedFieldRetryStateRejectsDuplicateBodyAndCap(t *testing.T) {
	initialBody := []byte(`{"model":"gpt-5.5"}`)
	state := newOpenAIResponsesRejectedFieldRetryState(initialBody)

	require.False(t, state.Allow(initialBody))
	for attempt := 0; attempt < maxOpenAIResponsesRejectedFieldRetries; attempt++ {
		nextBody := []byte(fmt.Sprintf(`{"model":"gpt-5.5","variant":%d}`, attempt))
		require.True(t, state.Allow(nextBody))
		require.False(t, state.Allow(nextBody))
	}
	require.False(t, state.Allow([]byte(`{"model":"gpt-5.5","variant":"overflow"}`)))
}

func TestOpenAIResponsesRejectedFieldRetryStateForRequestAllowsSameTransformAcrossAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	initialBody := []byte(`{"model":"gpt-5.5","truncation":"auto"}`)
	retryBody := []byte(`{"model":"gpt-5.5"}`)

	accountA := openAIResponsesRejectedFieldRetryStateForRequest(c, initialBody)
	require.True(t, accountA.Allow(retryBody))
	require.False(t, accountA.Allow(retryBody), "one account must not repeat the same transform")

	accountB := openAIResponsesRejectedFieldRetryStateForRequest(c, initialBody)
	require.NotSame(t, accountA, accountB)
	require.Same(t, accountA.budget, accountB.budget)
	require.True(t, accountB.Allow(retryBody), "a failover account must be allowed to apply the same transform")
}

func TestOpenAIResponsesRejectedFieldRetryStateForRequestSharesBoundedBudgetAcrossAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for attempt := 0; attempt < maxOpenAIResponsesRejectedFieldRetries; attempt++ {
		state := openAIResponsesRejectedFieldRetryStateForRequest(c, []byte(fmt.Sprintf(`{"account":%d}`, attempt)))
		require.True(t, state.Allow([]byte(`{"same":"retry"}`)))
	}
	overflow := openAIResponsesRejectedFieldRetryStateForRequest(c, []byte(`{"account":"overflow"}`))
	require.False(t, overflow.Allow([]byte(`{"new":"retry"}`)))
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRejectsAmbiguousErrors(t *testing.T) {
	tests := []struct {
		name         string
		body         []byte
		responseBody []byte
	}{
		{
			name:         "namespace belongs to message",
			body:         []byte(`{"input":[{"type":"message","namespace":"keep"}]}`),
			responseBody: []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[0].namespace'.","param":"input[0].namespace"}}`),
		},
		{
			name:         "max output tokens only mentioned",
			body:         []byte(`{"max_output_tokens":4096}`),
			responseBody: []byte(`{"error":{"code":"invalid_request_error","message":"max_output_tokens must be positive","param":"max_output_tokens"}}`),
		},
		{
			name:         "structured param overrides namespace mention",
			body:         []byte(`{"input":[{"type":"function_call","namespace":"keep","arguments":"{}"}]}`),
			responseBody: []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[0].namespace'.","param":"tools"}}`),
		},
		{
			name:         "nested max output tokens param is not top level",
			body:         []byte(`{"max_output_tokens":4096,"input":[{"type":"message","content":{"max_output_tokens":"keep"}}]}`),
			responseBody: []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: input[0].content.max_output_tokens","param":"input[0].content.max_output_tokens"}}`),
		},
		{
			name:         "structured target conflicts with message target",
			body:         []byte(`{"max_output_tokens":4096,"truncation":"auto"}`),
			responseBody: []byte(`{"error":{"code":"unsupported_parameter","message":"Unsupported parameter: truncation.","param":"max_output_tokens"}}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, tt.body, tt.responseBody)
			require.NoError(t, err)
			require.False(t, changed)
			require.Nil(t, retryBody)
		})
	}
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRepairsAutomationMissingRootType(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","name":"automation_update","parameters":{"oneOf":[{"type":"object"},{"type":"object","properties":{}}]}}]}`)
	responseBody := []byte(`{"error":{"code":"invalid_function_parameters","message":"Invalid schema for function 'automation_update': got 'type: \"None\"'.","param":"tools[0].parameters"}}`)

	retryBody, reason, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "tool parameter root type rejection", reason)
	require.Equal(t, "object", gjson.GetBytes(retryBody, "tools.0.parameters.type").String())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyDoesNotGuessAutomationRootType(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","name":"automation_update","parameters":{"oneOf":[{"type":"object"}]}}]}`)
	tests := []string{
		`{"error":{"code":"invalid_function_parameters","message":"got type: \"None\"","param":"metadata.parameters"}}`,
		`{"error":{"code":"invalid_request_error","message":"got type: \"None\"","param":"tools[0].parameters"}}`,
		`{"error":{"code":"invalid_function_parameters","message":"expected an object","param":"tools[0].parameters"}}`,
	}
	for _, response := range tests {
		retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, []byte(response))
		require.NoError(t, err)
		require.False(t, changed)
		require.Nil(t, retryBody)
	}
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyFindsNamespacePathInMessage(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call","namespace":"keep","arguments":"{}"},{"type":"function_call","namespace":"remove","arguments":"{}"}]}`)
	responseBody := []byte(`{"error":{"code":"unknown_parameter","message":"input[0] was accepted; Unknown parameter: 'input[1].namespace'."}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "keep", gjson.GetBytes(retryBody, "input.0.namespace").String())
	require.False(t, gjson.GetBytes(retryBody, "input.1.namespace").Exists())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyBindsNamespacePathToRejectionPhrase(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call","namespace":"keep","arguments":"{}"},{"type":"function_call","namespace":"remove","arguments":"{}"}]}`)
	responseBody := []byte(`{"error":{"code":"unknown_parameter","message":"input[0].namespace is supported; Unknown parameter: input[1].namespace."}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "keep", gjson.GetBytes(retryBody, "input.0.namespace").String())
	require.False(t, gjson.GetBytes(retryBody, "input.1.namespace").Exists())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyDoesNotTreatMaxOutputTokensSuggestionAsRejection(t *testing.T) {
	body := []byte(`{"max_tokens":4096,"max_output_tokens":2048}`)
	responseBody := []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: max_tokens. Use max_output_tokens instead."}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.False(t, changed)
	require.Nil(t, retryBody)
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyBindsMaxOutputTokensToRejectionPhrase(t *testing.T) {
	body := []byte(`{"max_output_tokens":2048}`)
	responseBody := []byte(`{"error":{"code":"unsupported_parameter","message":"Unsupported parameter: max_output_tokens."}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(retryBody, "max_output_tokens").Exists())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRemovesExactIndexedStatus(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","status":"keep","content":"one"},{"type":"reasoning","status":"remove","summary":[]}]}`)
	responses := []struct {
		name string
		body []byte
	}{
		{
			name: "structured param",
			body: []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[1].status'.","param":"input[1].status"}}`),
		},
		{
			name: "message param",
			body: []byte(`{"error":{"code":"unsupported_parameter","message":"Unsupported parameter: input[1].status."}}`),
		},
	}
	for _, tt := range responses {
		t.Run(tt.name, func(t *testing.T) {
			retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, tt.body)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, "keep", gjson.GetBytes(retryBody, "input.0.status").String())
			require.False(t, gjson.GetBytes(retryBody, "input.1.status").Exists())
		})
	}
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyNormalizesExactNullContent(t *testing.T) {
	tests := []struct {
		name       string
		body       []byte
		wantChange bool
		wantValue  string
		wantExists bool
	}{
		{
			name:       "message becomes empty string",
			body:       []byte(`{"input":[{"type":"message","role":"assistant","content":null}]}`),
			wantChange: true,
			wantValue:  "",
			wantExists: true,
		},
		{
			name:       "reasoning content is removed",
			body:       []byte(`{"input":[{"type":"reasoning","content":null,"summary":[]}]}`),
			wantChange: true,
			wantExists: false,
		},
		{
			name:       "unknown item is unchanged",
			body:       []byte(`{"input":[{"type":"future_item","content":null}]}`),
			wantChange: false,
		},
		{
			name:       "non null content is unchanged",
			body:       []byte(`{"input":[{"type":"message","content":"keep"}]}`),
			wantChange: false,
		},
	}
	responseBody := []byte(`{"error":{"code":"invalid_type","message":"Invalid type for 'input[0].content': expected one of a string or a list of input items, but got null instead.","param":"input[0].content"}}`)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, tt.body, responseBody)
			require.NoError(t, err)
			require.Equal(t, tt.wantChange, changed)
			if !tt.wantChange {
				require.Nil(t, retryBody)
				return
			}
			content := gjson.GetBytes(retryBody, "input.0.content")
			require.Equal(t, tt.wantExists, content.Exists())
			if tt.wantExists {
				require.Equal(t, tt.wantValue, content.String())
				require.Equal(t, gjson.String, content.Type)
			}
		})
	}
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRemovesExactReasoningContentAboveMaximumZero(t *testing.T) {
	body := []byte(`{"input":[{"type":"reasoning","content":[{"type":"reasoning_text","text":"remove"}],"summary":[]},{"type":"message","content":[{"type":"input_text","text":"keep"}]}]}`)
	responseBody := []byte(`{"error":{"code":"array_above_max_length","message":"Invalid 'input[0].content': array too long. Expected an array with maximum length 0, but got an array with length 1 instead.","param":"input[0].content","type":"invalid_request_error"}}`)

	retryBody, reason, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "indexed reasoning content maximum-length rejection", reason)
	require.False(t, gjson.GetBytes(retryBody, "input.0.content").Exists())
	require.Equal(t, "keep", gjson.GetBytes(retryBody, "input.1.content.0.text").String())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRejectsUnsafeReasoningMaximumZeroMutations(t *testing.T) {
	tests := []struct {
		name         string
		body         []byte
		responseBody []byte
	}{
		{
			name:         "message content is not reasoning",
			body:         []byte(`{"input":[{"type":"message","content":[{"type":"input_text","text":"keep"}]}]}`),
			responseBody: []byte(`{"error":{"code":"array_above_max_length","message":"Invalid 'input[0].content': array too long. Expected an array with maximum length 0, but got an array with length 1 instead.","param":"input[0].content"}}`),
		},
		{
			name:         "structured param and message disagree",
			body:         []byte(`{"input":[{"type":"reasoning","content":[{"text":"keep"}]},{"type":"reasoning","content":[{"text":"keep too"}]}]}`),
			responseBody: []byte(`{"error":{"code":"array_above_max_length","message":"Invalid 'input[1].content': array too long. Expected an array with maximum length 0, but got an array with length 1 instead.","param":"input[0].content"}}`),
		},
		{
			name:         "different error code",
			body:         []byte(`{"input":[{"type":"reasoning","content":[{"text":"keep"}]}]}`),
			responseBody: []byte(`{"error":{"code":"invalid_request_error","message":"Invalid 'input[0].content': array too long. Expected an array with maximum length 0, but got an array with length 1 instead.","param":"input[0].content"}}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, tt.body, tt.responseBody)
			require.NoError(t, err)
			require.False(t, changed)
			require.Nil(t, retryBody)
		})
	}
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRemovesExplicitlyRejectedTopLevelTruncation(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","truncation":"auto","input":"keep"}`)
	responses := [][]byte{
		[]byte(`{"error":{"code":"unsupported_parameter","message":"Unsupported parameter: 'truncation'.","param":"truncation"}}`),
		[]byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: truncation."}}`),
	}
	for _, responseBody := range responses {
		retryBody, reason, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, "truncation parameter rejection", reason)
		require.False(t, gjson.GetBytes(retryBody, "truncation").Exists())
		require.Equal(t, "keep", gjson.GetBytes(retryBody, "input").String())
	}
}

func TestOpenAIGatewayService_APIKeyRetriesExplicitlyRejectedTopLevelTruncation(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"truncation":"auto","input":"keep"}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusBadRequest, `{"error":{"code":"unsupported_parameter","message":"Unsupported parameter: 'truncation'.","param":"truncation"}}`),
		newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(), newOpenAIRejectedFieldTestContext(body), newOpenAIRejectedFieldTestAccount(), body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "auto", gjson.GetBytes(upstream.bodies[0], "truncation").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "truncation").Exists())
	// API Key 账号发送前会把字符串 input 预规范化为 message 块数组
	// （normalizeOpenAIAPIKeyResponsesStringInput）；重试只负责删除 truncation，
	// 已规范化的 input 原样保留，不再回退为字符串。
	input := gjson.GetBytes(upstream.bodies[1], "input")
	require.True(t, input.IsArray())
	require.Equal(t, "keep", input.Array()[0].Get("content.0.text").String())
	require.Equal(t, "input_text", input.Array()[0].Get("content.0.type").String())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRejectsUnsafeIndexedMutations(t *testing.T) {
	tests := []struct {
		name         string
		body         []byte
		responseBody []byte
	}{
		{
			name:         "nested status path",
			body:         []byte(`{"input":[{"type":"message","content":{"status":"keep"}}]}`),
			responseBody: []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: input[0].content.status.","param":"input[0].content.status"}}`),
		},
		{
			name:         "status index out of bounds",
			body:         []byte(`{"input":[{"type":"message","status":"keep"}]}`),
			responseBody: []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: input[4].status.","param":"input[4].status"}}`),
		},
		{
			name:         "status path only mentioned",
			body:         []byte(`{"input":[{"type":"message","status":"keep"}]}`),
			responseBody: []byte(`{"error":{"code":"invalid_request_error","message":"input[0].status must be completed","param":"input[0].status"}}`),
		},
		{
			name:         "content param and message disagree",
			body:         []byte(`{"input":[{"type":"message","content":null},{"type":"message","content":null}]}`),
			responseBody: []byte(`{"error":{"code":"invalid_type","message":"Invalid type for input[1].content: got null instead.","param":"input[0].content"}}`),
		},
		{
			name:         "content error only mentions null",
			body:         []byte(`{"input":[{"type":"message","content":null}]}`),
			responseBody: []byte(`{"error":{"code":"invalid_type","message":"content cannot be null","param":"input[0].content"}}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, tt.body, tt.responseBody)
			require.NoError(t, err)
			require.False(t, changed)
			require.Nil(t, retryBody)
		})
	}
}

func TestOpenAIGatewayService_OAuthRetriesExactRejectedStatus(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":true,"instructions":"test","input":[{"type":"message","role":"user","status":"completed","content":"hello"}]}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusBadRequest, `{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[0].status'.","param":"input[0].status"}}`),
		newOpenAIRejectedFieldTestResponse(http.StatusOK, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_ok\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\ndata: [DONE]\n\n"),
	}}
	upstream.responses[1].Header.Set("Content-Type", "text/event-stream")

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(), newOpenAIRejectedFieldTestContext(body), newOpenAIOAuthNamespaceTestAccount(), body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "completed", gjson.GetBytes(upstream.bodies[0], "input.0.status").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "input.0.status").Exists())
}

func TestOpenAIGatewayService_APIKeyRetriesExactRejectedNullMessageContent(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"input":[{"type":"message","role":"assistant","content":null},{"type":"message","role":"user","content":"continue"}]}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusBadRequest, `{"error":{"code":"invalid_type","message":"Invalid type for 'input[0].content': expected one of a string or a list of input items, but got null instead.","param":"input[0].content"}}`),
		newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(), newOpenAIRejectedFieldTestContext(body), newOpenAIRejectedFieldTestAccount(), body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, gjson.Null, gjson.GetBytes(upstream.bodies[0], "input.0.content").Type)
	require.Equal(t, gjson.String, gjson.GetBytes(upstream.bodies[1], "input.0.content").Type)
	require.Equal(t, "continue", gjson.GetBytes(upstream.bodies[1], "input.1.content").String())
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRemovesModelRejectedPromptCacheBreakpoint(t *testing.T) {
	tests := []struct {
		name         string
		body         []byte
		responseBody []byte
		removedPath  string
		preserved    string
		reason       string
	}{
		{
			name:         "top level",
			body:         []byte(`{"model":"gpt-5.6-sol","prompt_cache_breakpoint":{"type":"message_start"},"input":"hello"}`),
			responseBody: []byte(`{"error":{"code":"invalid_parameter","message":"prompt_cache_breakpoint is not supported on this model","param":"prompt_cache_breakpoint"}}`),
			removedPath:  "prompt_cache_breakpoint",
			preserved:    "input",
			reason:       "prompt_cache_breakpoint parameter rejection",
		},
		{
			name:         "indexed path from message",
			body:         []byte(`{"input":[{"type":"message","prompt_cache_breakpoint":{"type":"message_start"}},{"type":"message","prompt_cache_breakpoint":{"type":"message_end"}}]}`),
			responseBody: []byte(`{"error":{"code":"invalid_parameter","message":"input[1].prompt_cache_breakpoint is not supported on this model"}}`),
			removedPath:  "input.1.prompt_cache_breakpoint",
			preserved:    "input.0.prompt_cache_breakpoint",
			reason:       "indexed prompt_cache_breakpoint parameter rejection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryBody, reason, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, tt.body, tt.responseBody)

			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, tt.reason, reason)
			require.False(t, gjson.GetBytes(retryBody, tt.removedPath).Exists())
			require.True(t, gjson.GetBytes(retryBody, tt.preserved).Exists())
		})
	}
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyRejectsAmbiguousPromptCacheBreakpointErrors(t *testing.T) {
	body := []byte(`{"prompt_cache_breakpoint":{"type":"message_start"},"input":[{"type":"message","prompt_cache_breakpoint":{"type":"message_end"}}]}`)
	tests := []struct {
		name         string
		responseBody []byte
	}{
		{
			name:         "structured param disagrees",
			responseBody: []byte(`{"error":{"code":"invalid_parameter","message":"input[0].prompt_cache_breakpoint is not supported on this model","param":"prompt_cache_breakpoint"}}`),
		},
		{
			name:         "index out of bounds",
			responseBody: []byte(`{"error":{"code":"invalid_parameter","message":"input[4].prompt_cache_breakpoint is not supported on this model","param":"input[4].prompt_cache_breakpoint"}}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, tt.responseBody)

			require.NoError(t, err)
			require.False(t, changed)
			require.Nil(t, retryBody)
		})
	}
}

func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyAcceptsEitherCacheModelRejectionSignal(t *testing.T) {
	tests := []struct {
		name         string
		responseBody []byte
	}{
		{
			name:         "invalid parameter code",
			responseBody: []byte(`{"error":{"code":"invalid_parameter","message":"This optional cache hint cannot be used here","param":"prompt_cache_breakpoint"}}`),
		},
		{
			name:         "model rejection message",
			responseBody: []byte(`{"error":{"code":"invalid_request_error","message":"prompt_cache_breakpoint is not supported on this model","param":"prompt_cache_breakpoint"}}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, []byte(`{"prompt_cache_breakpoint":true,"input":"keep"}`), tt.responseBody)

			require.NoError(t, err)
			require.True(t, changed)
			require.False(t, gjson.GetBytes(retryBody, "prompt_cache_breakpoint").Exists())
			require.Equal(t, "keep", gjson.GetBytes(retryBody, "input").String())
		})
	}
}

func TestOpenAIResponsesRejectedFieldRetryStateAllowsPromptCacheBreakpointVariantOnce(t *testing.T) {
	body := []byte(`{"input":[{"prompt_cache_breakpoint":{"type":"message_start"}}]}`)
	responseBody := []byte(`{"error":{"code":"invalid_parameter","message":"input[0].prompt_cache_breakpoint is not supported on this model","param":"input[0].prompt_cache_breakpoint"}}`)
	state := newOpenAIResponsesRejectedFieldRetryState(body)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, state.Allow(retryBody))
	require.False(t, state.Allow(retryBody))
}

func TestOpenAIGatewayService_APIKeyStripsAllIndexedNamespacesBeforeFirstForward(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"input":[{"type":"function_call","name":"first","namespace":"remove-first","arguments":"{}"},{"type":"custom_tool_call","name":"second","namespace":"remove-second","input":"{}"}]}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(),
		newOpenAIRejectedFieldTestContext(body),
		newOpenAIRejectedFieldTestAccount(),
		body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 1)
	require.False(t, gjson.GetBytes(upstream.bodies[0], "input.0.namespace").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[0], "input.1.namespace").Exists())
}

func TestOpenAIGatewayServiceProactivelyStripsCrossProviderReasoningContent(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"store":true,"input":[` +
		`{"type":"message","role":"user","content":"one"},` +
		`{"type":"message","role":"assistant","content":"two"},` +
		`{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"call_1","output":"ok"},` +
		`{"type":"message","role":"user","content":"five"},` +
		`{"type":"reasoning","summary":[{"type":"summary_text","text":"keep"}],"content":[{"type":"reasoning_text","text":"remove"}]}` +
		`]}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(),
		newOpenAIRejectedFieldTestContext(body),
		newOpenAIRejectedFieldTestAccount(),
		body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 1, "reasoning content should be normalized before the first upstream request")
	require.Equal(t, "reasoning", gjson.GetBytes(upstream.bodies[0], "input.5.type").String())
	require.False(t, gjson.GetBytes(upstream.bodies[0], "input.5.content").Exists())
	require.Equal(t, "keep", gjson.GetBytes(upstream.bodies[0], "input.5.summary.0.text").String())
}

func TestOpenAIGatewayService_OpenAIHTTPStripsInputNamespacesBeforeFirstForward(t *testing.T) {
	accounts := []struct {
		name    string
		account *Account
	}{
		{name: "oauth", account: newOpenAIOAuthNamespaceTestAccount()},
		{name: "apikey", account: newOpenAIRejectedFieldTestAccount()},
	}
	for _, tt := range accounts {
		for _, path := range []string{"/v1/responses", "/v1/responses/compact"} {
			t.Run(tt.name+path, func(t *testing.T) {
				body := []byte(`{"model":"gpt-5.5","stream":false,"instructions":"test","input":[{"type":"message","role":"user","namespace":"remove","content":[{"type":"input_text","text":"hello","namespace":"nested-keep"}]}]}`)
				upstream := &httpUpstreamRecorder{responses: []*http.Response{
					newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"id":"resp_namespace_ok","output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
				}}
				c := newOpenAIRejectedFieldTestContext(body)
				c.Request.URL.Path = path

				result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
					context.Background(),
					c,
					tt.account,
					body,
				)

				require.NoError(t, err)
				require.NotNil(t, result)
				require.Len(t, upstream.bodies, 1, "namespace must be removed before the first upstream request")
				require.False(t, gjson.GetBytes(upstream.bodies[0], "input.0.namespace").Exists())
				require.Equal(t, "nested-keep", gjson.GetBytes(upstream.bodies[0], "input.0.content.0.namespace").String())
			})
		}
	}
}

func TestOpenAIGatewayService_RetriesExplicitMaxOutputTokensRejection(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"max_output_tokens":4096,"input":[{"type":"message","role":"user","content":{"max_output_tokens":"keep"}}]}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusBadRequest, `{"error":{"code":"unsupported_parameter","message":"Unsupported parameter: max_output_tokens","param":"max_output_tokens","type":"invalid_request_error"}}`),
		newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(),
		newOpenAIRejectedFieldTestContext(body),
		newOpenAIRejectedFieldTestAccount(),
		body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, int64(4096), gjson.GetBytes(upstream.bodies[0], "max_output_tokens").Int())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "max_output_tokens").Exists())
	require.Equal(t, "keep", gjson.GetBytes(upstream.bodies[1], "input.0.content.max_output_tokens").String())
}

func TestOpenAIGatewayService_ComposesProactiveNamespaceStripWithRejectedFieldRetry(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"max_output_tokens":2048,"input":[{"type":"function_call","name":"first","namespace":"remove-first","arguments":"{}"},{"type":"custom_tool_call","name":"second","namespace":"remove-second","input":"{}"}]}`)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		newOpenAIRejectedFieldTestResponse(http.StatusBadRequest, `{"error":{"code":"unsupported_parameter","message":"Unsupported parameter: max_output_tokens","param":"max_output_tokens"}}`),
		newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`),
	}}

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(),
		newOpenAIRejectedFieldTestContext(body),
		newOpenAIRejectedFieldTestAccount(),
		body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	for _, forwardedBody := range upstream.bodies {
		require.False(t, gjson.GetBytes(forwardedBody, "input.0.namespace").Exists())
		require.False(t, gjson.GetBytes(forwardedBody, "input.1.namespace").Exists())
	}
	require.Equal(t, int64(2048), gjson.GetBytes(upstream.bodies[0], "max_output_tokens").Int())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "max_output_tokens").Exists())
}

func newOpenAIRejectedFieldTestService(upstream *httpUpstreamRecorder) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg: &config.Config{Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		}},
		httpUpstream: upstream,
	}
}

func newOpenAIRejectedFieldTestContext(body []byte) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "curl/8.0")
	return c
}

func newOpenAIRejectedFieldTestAccount() *Account {
	return &Account{
		ID:          5107,
		Name:        "responses-compatible",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://compat.example",
		},
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeAuto),
			openai_compat.ExtraKeyResponsesSupported: true,
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func newOpenAIOAuthNamespaceTestAccount() *Account {
	return &Account{
		ID:          5108,
		Name:        "openai-oauth-namespace",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-account",
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func newOpenAIRejectedFieldTestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// A replayed conversation carries many items of the same type, each with a
// status the upstream schema rejects. One rejection must clear all of them:
// clearing one index per round trip exhausts the bounded retry budget.
func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyClearsStatusForWholeType(t *testing.T) {
	input := make([]string, 0, 12)
	for i := 0; i < 10; i++ {
		input = append(input, `{"type":"tool_search_output","status":"completed","call_id":"call_`+strconv.Itoa(i)+`","tools":[]}`)
	}
	input = append(input, `{"type":"message","role":"user","status":"completed","content":"hi"}`)
	body := []byte(`{"input":[` + strings.Join(input, ",") + `]}`)

	responseBody := []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[7].status'.","param":"input[7].status"}}`)
	retryBody, reason, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotEmpty(t, reason)

	for i := 0; i < 10; i++ {
		require.False(t, gjson.GetBytes(retryBody, "input."+strconv.Itoa(i)+".status").Exists(),
			"every tool_search_output must lose its status in a single retry, index %d did not", i)
		require.Equal(t, "call_"+strconv.Itoa(i), gjson.GetBytes(retryBody, "input."+strconv.Itoa(i)+".call_id").String(),
			"unrelated fields must survive")
	}
	require.Equal(t, "completed", gjson.GetBytes(retryBody, "input.10.status").String(),
		"a different item type keeps its status: the rejection only proves this type has none")
}

// The rejected item may carry no type to match on.
func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyClearsUntypedStatusAtIndexOnly(t *testing.T) {
	body := []byte(`{"input":[{"status":"keep_a"},{"status":"remove"}]}`)
	responseBody := []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'input[1].status'.","param":"input[1].status"}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "keep_a", gjson.GetBytes(retryBody, "input.0.status").String())
	require.False(t, gjson.GetBytes(retryBody, "input.1.status").Exists())
}

// 部分兼容上游（platform.experientiallabs.ai）不接受 message content 的字符串
// 简写：官方 API 允许 "content":"hi"，该上游要求 input_text 块数组。命中后应
// 一次修复 input 里全部 message 字符串 content（按角色选块类型），其余项不动。
func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyNormalizesStringContent(t *testing.T) {
	// 构造 9 项 input（索引 0-8），上游点名的 input.8 真实存在。
	padding := make([]string, 0, 9)
	padding = append(padding,
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"already block"}]}`,
		`{"type":"message","role":"assistant","content":"string answer"}`,
		`{"type":"function_call_output","call_id":"call_1","output":"string output stays"}`)
	for i := 3; i < 8; i++ {
		padding = append(padding, `{"type":"message","role":"user","content":[{"type":"input_text","text":"round `+strconv.Itoa(i)+`"}]}`)
	}
	padding = append(padding, `{"type":"message","role":"user","content":"string question"}`)
	body := []byte(`{"input":[` + strings.Join(padding, ",") + `]}`)
	responseBody := []byte(`{"error":{"code":"invalid_parameter","message":"Invalid value for 'input.8.content.0.type': expected one of 'input_text', but got a string instead.","param":"input.8.content.0.type"}}`)

	retryBody, reason, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "string content shorthand rejection", reason)

	// 字符串 content 被规范化为块数组，assistant 用 output_text。
	assistant := gjson.GetBytes(retryBody, "input.1.content")
	require.True(t, assistant.IsArray())
	require.Equal(t, "output_text", assistant.Array()[0].Get("type").String())
	require.Equal(t, "string answer", assistant.Array()[0].Get("text").String())

	user := gjson.GetBytes(retryBody, "input.8.content")
	require.True(t, user.IsArray())
	require.Equal(t, "input_text", user.Array()[0].Get("type").String())
	require.Equal(t, "string question", user.Array()[0].Get("text").String())

	// 已是块数组的 user 消息与 function_call_output 的字符串 output 原样保留。
	require.True(t, gjson.GetBytes(retryBody, "input.0.content").IsArray())
	require.Equal(t, "already block", gjson.GetBytes(retryBody, "input.0.content.0.text").String())
	require.Equal(t, "string output stays", gjson.GetBytes(retryBody, "input.2.output").String())
}

// 上游不报 param（或 param 为点分形态）时，也能从错误消息中提取索引并修复。
func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyNormalizesStringContentWithoutParam(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","role":"user","content":"plain text"}]}`)
	responseBody := []byte(`{"error":{"code":"invalid_parameter","message":"Invalid value for 'input.0.content.0.type': expected one of 'input_text', but got a string instead."}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "plain text", gjson.GetBytes(retryBody, "input.0.content.0.text").String())
	require.Equal(t, "input_text", gjson.GetBytes(retryBody, "input.0.content.0.type").String())
}

// 上游不支持 Responses 原生工具声明（web_search 等服务端工具）时，剥离全部
// 非 function 工具条目后重试；function 工具与其字段完整保留。
func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyStripsUnsupportedServerTools(t *testing.T) {
	body := []byte(`{"model":"claude-fable-5.1","tools":[` +
		`{"type":"web_search"},` +
		`{"type":"function","name":"get_weather","description":"weather","parameters":{"type":"object","properties":{}}},` +
		`{"type":"web_search_preview"}]}`)
	responseBody := []byte(`{"error":{"code":"unsupported_parameter","message":"The request carries native Responses tool declarations (custom, namespace, web_search, or tool_search entries) that only a native OpenAI Responses route can serve. Remove those tools or choose a different model alias.","param":"tools"}}`)

	retryBody, reason, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "native responses tools rejection", reason)

	tools := gjson.GetBytes(retryBody, "tools").Array()
	require.Len(t, tools, 1)
	require.Equal(t, "function", tools[0].Get("type").String())
	require.Equal(t, "get_weather", tools[0].Get("name").String())
	require.Equal(t, "weather", tools[0].Get("description").String())
}

// tools 全部为非 function 类型时，连同字段一起删除，避免提交空数组。
func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyDropsToolsWhenAllRejected(t *testing.T) {
	body := []byte(`{"model":"claude-fable-5.1","tools":[{"type":"web_search"},{"type":"web_search_preview"}]}`)
	responseBody := []byte(`{"error":{"code":"unsupported_parameter","message":"The request carries native Responses tool declarations that only a native OpenAI Responses route can serve.","param":"tools"}}`)

	retryBody, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(retryBody, "tools").Exists())
	require.Equal(t, "claude-fable-5.1", gjson.GetBytes(retryBody, "model").String())
}

// 官方上游对 function 工具的正常拒绝（unknown parameter）不应触发工具剥离，
// 避免误删合法能力。
func TestNormalizeOpenAIResponsesRejectedFieldRetryBodyDoesNotStripToolsOnUnknownParameter(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","name":"f"},{"type":"web_search"}]}`)
	responseBody := []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: 'tools.0.something'.","param":"tools.0.something"}}`)

	_, _, changed, err := normalizeOpenAIResponsesRejectedFieldRetryBody(http.StatusBadRequest, body, responseBody)
	require.NoError(t, err)
	require.False(t, changed)
}
