package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// 复现组清单生成：多账号候选下 multi_agent_version 是否被保留为 v1。
func TestGroupCodexModelMetadataKeepsMultiAgentDefault(t *testing.T) {
	accounts := []Account{
		{ID: 150, Name: "SwiftAPI", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-a", "base_url": "https://up-a.example/v1",
				"model_mapping": map[string]any{"flash-glm": "glm-5.3-flash"}}},
		{ID: 157, Name: "r4", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-b", "base_url": "https://up-b.example/v1",
				"model_mapping": map[string]any{"flash-deepseek": "union-alpha"}}},
	}
	group := &Group{ID: 9, Platform: PlatformOpenAI}

	for _, modelID := range []string{"flash-glm", "flash-deepseek"} {
		metadata, ok := groupCodexModelMetadata(PlatformOpenAI, modelID, accounts, group, nil, true)
		require.True(t, ok, modelID)
		raw, err := json.Marshal(metadata.CodexToolCapabilities["multi_agent_version"])
		require.NoError(t, err, modelID)
		t.Logf("%s multi_agent_version=%s", modelID, string(raw))
		require.Equal(t, `"v1"`, string(raw), modelID)
	}
}

// 镜像线上 group 9 的真实账号形态（含 astra 单候选、flash-glm 双候选），
// 验证完整组清单构建后 multi_agent_version 是否仍为 v1。
func TestBuildGroupCodexModelsManifestKeepsMultiAgentDefault(t *testing.T) {
	accounts := []Account{
		{ID: 147, Name: "WB2", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-w", "base_url": "http://127.0.0.1:7863",
				"model_mapping": map[string]any{"flash-glm": "glm-5.3-flash", "flash-deepseek": "deepseek-v4.1-flash"}}},
		{ID: 150, Name: "SwiftAPI", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-a", "base_url": "https://up-a.example/v1",
				"model_mapping": map[string]any{"flash-glm": "glm-5.3-flash"}}},
		{ID: 152, Name: "QuickRouter", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-q", "base_url": "https://up-q.example/v1",
				"model_mapping": map[string]any{"gpt-6-astra": "gpt-6-astra"}}},
		{ID: 157, Name: "r4", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "sk-b", "base_url": "https://up-b.example/v1",
				"model_mapping": map[string]any{"flash-deepseek": "union-alpha"}}},
	}
	group := &Group{ID: 9, Platform: PlatformOpenAI}
	modelIDs := []string{"flash-deepseek", "flash-glm", "gpt-6-astra"}

	body, err := buildCodexModelsManifestForAccounts(PlatformOpenAI, modelIDs, accounts, group, nil, true)
	require.NoError(t, err)
	var envelope struct {
		Models []map[string]any `json:"models"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	for _, model := range envelope.Models {
		slug, _ := model["slug"].(string)
		t.Logf("%s multi_agent_version=%v", slug, model["multi_agent_version"])
		if slug == "flash-glm" || slug == "flash-deepseek" {
			require.Equal(t, "v1", model["multi_agent_version"], slug)
		}
	}
}
