//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestClaudeTestPayloadUsesLowReasoningEffort 覆盖 Claude 支持与不支持 effort 参数的测试模型。
func TestClaudeTestPayloadUsesLowReasoningEffort(t *testing.T) {
	t.Parallel()

	supportedPayload, err := createTestPayload("claude-opus-4-6")
	require.NoError(t, err)
	require.Equal(t, modelTestReasoningEffort, supportedPayload["output_config"].(map[string]string)["effort"])

	unsupportedPayload, err := createTestPayload("claude-sonnet-4-5-20250929")
	require.NoError(t, err)
	require.NotContains(t, unsupportedPayload, "output_config")
}

// TestGeminiTestPayloadUsesLowReasoningEffort 覆盖 Gemini 支持低推理与不支持模型的请求体分支。
func TestGeminiTestPayloadUsesLowReasoningEffort(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		model        string
		wantBudget   int64
		wantLevel    string
		wantThinking bool
	}{
		{name: "Gemini 2.5", model: "gemini-2.5-flash", wantBudget: geminiLowThinkingBudget, wantThinking: true},
		{name: "Gemini 3 text", model: "gemini-3-pro-preview", wantLevel: geminiLowThinkingLevel, wantThinking: true},
		{name: "Gemini 2.0", model: "gemini-2.0-flash", wantThinking: false},
		{name: "Gemini 2.5 image", model: "gemini-2.5-flash-image", wantThinking: false},
		{name: "Gemini 3 image", model: "gemini-3.1-flash-image-preview", wantThinking: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			payload := createGeminiTestPayload(testCase.model, "hi")
			if !testCase.wantThinking {
				require.False(t, gjson.GetBytes(payload, "generationConfig.thinkingConfig").Exists())
				return
			}
			if testCase.wantBudget != 0 {
				require.Equal(t, testCase.wantBudget, gjson.GetBytes(payload, "generationConfig.thinkingConfig.thinkingBudget").Int())
			}
			require.Equal(t, testCase.wantLevel, gjson.GetBytes(payload, "generationConfig.thinkingConfig.thinkingLevel").String())
		})
	}
}

// TestBuildGrokQuotaProbeBodyUsesLowReasoningEffort 覆盖 Grok 支持与 Composer 兼容分支。
func TestBuildGrokQuotaProbeBodyUsesLowReasoningEffort(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		model         string
		wantReasoning bool
	}{
		{name: "supported model", model: "grok-4.5", wantReasoning: true},
		{name: "Composer compatibility", model: "grok-composer-2.5-fast", wantReasoning: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			payload, err := buildGrokQuotaProbeBody(testCase.model)
			require.NoError(t, err)
			require.Equal(t, testCase.wantReasoning, gjson.GetBytes(payload, "reasoning").Exists())
			if testCase.wantReasoning {
				require.Equal(t, modelTestReasoningEffort, gjson.GetBytes(payload, "reasoning.effort").String())
			}
		})
	}
}

// TestAntigravityTestPayloadsUseLowReasoningEffort 覆盖 Antigravity 的 Gemini 与 Claude 模型测试请求。
func TestAntigravityTestPayloadsUseLowReasoningEffort(t *testing.T) {
	t.Parallel()

	svc := &AntigravityGatewayService{}
	geminiPayload, err := svc.buildGeminiTestRequest("test-project", "gemini-2.5-flash")
	require.NoError(t, err)
	require.Equal(t, int64(geminiLowThinkingBudget), gjson.GetBytes(geminiPayload, "request.generationConfig.thinkingConfig.thinkingBudget").Int())

	claudePayload, err := svc.buildClaudeTestRequest("test-project", "claude-opus-4-6")
	require.NoError(t, err)
	require.Equal(t, int64(geminiLowThinkingBudget), gjson.GetBytes(claudePayload, "request.generationConfig.thinkingConfig.thinkingBudget").Int())
}
