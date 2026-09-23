package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayServiceForwardsOpenRouterServerToolsAsFunctions(t *testing.T) {
	body := []byte(`{
		"model":"flash",
		"stream":false,
		"input":"ok",
		"tools":[
			{"type":"tool_search"},
			{"type":"web_search"},
			{"type":"custom","name":"exec"},
			{"type":"function","name":"get_weather","parameters":{"type":"object"}}
		]
	}`)
	upstream := &httpUpstreamRecorder{
		resp: newOpenAIRejectedFieldTestResponse(http.StatusOK, `{"output":[],"usage":{"input_tokens":1,"output_tokens":1}}`),
	}
	account := newOpenAIRejectedFieldTestAccount()
	account.Credentials["base_url"] = "https://openrouter.ai/api/v1"

	result, err := newOpenAIRejectedFieldTestService(upstream).Forward(
		context.Background(),
		newOpenAIRejectedFieldTestContext(body),
		account,
		body,
	)

	require.NoError(t, err)
	require.NotNil(t, result)

	tools := gjson.GetBytes(upstream.lastBody, "tools").Array()
	require.Len(t, tools, 3)
	toolTypes := make([]string, 0, len(tools))
	toolNames := make(map[string]string)
	for _, tool := range tools {
		typ := tool.Get("type").String()
		toolTypes = append(toolTypes, typ)
		toolNames[tool.Get("name").String()] = typ
		require.Equal(t, "function", typ, "OpenRouter must receive function tools only")
	}
	require.NotContains(t, toolTypes, "web_search")
	require.Equal(t, "function", toolNames["tool_search"])
	require.Equal(t, "function", toolNames["exec"])
	require.Equal(t, "function", toolNames["get_weather"])
}

func TestIsOpenRouterOpenAIAccount(t *testing.T) {
	account := newOpenAIRejectedFieldTestAccount()
	account.Credentials["base_url"] = "https://openrouter.ai/api/v1"
	require.True(t, isOpenRouterOpenAIAccount(account))

	account.Credentials["base_url"] = "https://compat.example/v1"
	require.False(t, isOpenRouterOpenAIAccount(account))

	account.Platform = PlatformAnthropic
	account.Credentials["base_url"] = "https://openrouter.ai/api/v1"
	require.False(t, isOpenRouterOpenAIAccount(account))
}
