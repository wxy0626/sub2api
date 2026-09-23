package service

import (
	"net/url"
	"strings"
)

// isOpenRouterOpenAIAccount 判断 OpenAI APIKey 账号是否直连 OpenRouter。
// OpenRouter 对 Responses 原生服务端工具的兼容性不稳定，需要在出站前降级。
func isOpenRouterOpenAIAccount(account *Account) bool {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return false
	}
	baseURL := strings.TrimSpace(account.GetOpenAIBaseURL())
	if baseURL == "" {
		return false
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "openrouter.ai" || strings.HasSuffix(host, ".openrouter.ai")
}
