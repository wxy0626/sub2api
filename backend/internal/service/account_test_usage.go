package service

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/gin-gonic/gin"
)

type accountTestUsageCaptureKey struct{}

// accountTestUsageCapture 收集一次管理员账号测试的非计费信息。
type accountTestUsageCapture struct {
	mu                  sync.Mutex
	accountID           int64
	platform            string
	redactionValues     []string // 账号凭据原文，用于清理上游错误详情中的裸密钥。
	model               string
	testMode            string
	endpoint            string
	inputTokens         int
	outputTokens        int
	cacheCreationTokens int
	cacheReadTokens     int
	statusCode          int
	startedAt           time.Time
}

func newAccountTestUsageCapture(account *Account, mode string) *accountTestUsageCapture {
	platform := ""
	accountID := int64(0)
	redactionValues := make([]string, 0, 3)
	if account != nil {
		platform = account.Platform
		accountID = account.ID
		for _, credentialKey := range []string{"api_key", "access_token", "refresh_token"} {
			if credential := strings.TrimSpace(account.GetCredential(credentialKey)); credential != "" {
				redactionValues = append(redactionValues, credential)
			}
		}
	}
	return &accountTestUsageCapture{
		accountID:       accountID,
		platform:        platform,
		redactionValues: redactionValues,
		testMode:        strings.TrimSpace(mode),
		startedAt:       time.Now(),
	}
}

func (capture *accountTestUsageCapture) setRequest(model, endpoint string) {
	if capture == nil {
		return
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if strings.TrimSpace(model) != "" {
		capture.model = strings.TrimSpace(model)
	}
	if strings.TrimSpace(endpoint) != "" {
		capture.endpoint = strings.TrimSpace(endpoint)
	}
}

func (capture *accountTestUsageCapture) setStatusCode(statusCode int) {
	if capture == nil {
		return
	}
	capture.mu.Lock()
	capture.statusCode = statusCode
	capture.mu.Unlock()
}

func (capture *accountTestUsageCapture) addUsage(inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens int) {
	if capture == nil {
		return
	}
	capture.mu.Lock()
	capture.inputTokens = maxAccountTestUsageInt(capture.inputTokens, inputTokens)
	capture.outputTokens = maxAccountTestUsageInt(capture.outputTokens, outputTokens)
	capture.cacheCreationTokens = maxAccountTestUsageInt(capture.cacheCreationTokens, cacheCreationTokens)
	capture.cacheReadTokens = maxAccountTestUsageInt(capture.cacheReadTokens, cacheReadTokens)
	capture.mu.Unlock()
}

func (capture *accountTestUsageCapture) snapshot(err error) usagestats.AccountTestUsageRecord {
	capture.mu.Lock()
	defer capture.mu.Unlock()

	errorMessage := ""
	success := err == nil
	if err != nil {
		errorMessage = strings.TrimSpace(err.Error())
		for _, secret := range capture.redactionValues {
			errorMessage = strings.ReplaceAll(errorMessage, secret, "[REDACTED_CREDENTIAL]")
		}
		errorMessage = redactAccountTestCredentialFields(errorMessage)
		errorMessage = truncateAccountTestUsageError(errorMessage)
	}
	durationMs := time.Since(capture.startedAt).Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}
	return usagestats.AccountTestUsageRecord{
		AccountID:           capture.accountID,
		Platform:            capture.platform,
		Model:               capture.model,
		TestMode:            capture.testMode,
		Endpoint:            capture.endpoint,
		InputTokens:         capture.inputTokens,
		OutputTokens:        capture.outputTokens,
		CacheCreationTokens: capture.cacheCreationTokens,
		CacheReadTokens:     capture.cacheReadTokens,
		DurationMs:          durationMs,
		Success:             success,
		StatusCode:          capture.statusCode,
		ErrorMessage:        errorMessage,
		CreatedAt:           time.Now(),
	}
}

func truncateAccountTestUsageError(message string) string {
	const maxLength = 4000
	message = strings.TrimSpace(message)
	if len(message) <= maxLength {
		return message
	}
	return message[:maxLength] + "..."
}

// maxAccountTestUsageInt 保留上游流中同一 usage 字段的最大快照。
func maxAccountTestUsageInt(left, right int) int {
	if right > left {
		return right
	}
	return left
}

func accountTestUsageCaptureFromContext(c *gin.Context) *accountTestUsageCapture {
	if c == nil || c.Request == nil {
		return nil
	}
	capture, _ := c.Request.Context().Value(accountTestUsageCaptureKey{}).(*accountTestUsageCapture)
	return capture
}

func beginAccountTestUsageRequest(c *gin.Context, model, endpoint string) {
	capture := accountTestUsageCaptureFromContext(c)
	if capture == nil {
		return
	}
	capture.setRequest(model, endpoint)
}

func recordAccountTestUsageStatus(c *gin.Context, statusCode int) {
	capture := accountTestUsageCaptureFromContext(c)
	if capture == nil {
		return
	}
	capture.setStatusCode(statusCode)
}

// recordAccountTestUsageRequest 从实际 HTTP 请求提取最终 endpoint，避免各平台重复拼接路径。
func recordAccountTestUsageRequest(c *gin.Context, req *http.Request) {
	if req == nil || req.URL == nil {
		return
	}
	beginAccountTestUsageRequest(c, "", req.URL.Path)
}

// addAccountTestUsageRedaction 为本次测试补充运行时取得的临时凭据脱敏值。
func addAccountTestUsageRedaction(c *gin.Context, value string) {
	capture := accountTestUsageCaptureFromContext(c)
	if capture == nil {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	for _, existing := range capture.redactionValues {
		if existing == value {
			return
		}
	}
	capture.redactionValues = append(capture.redactionValues, value)
}

// redactAccountTestErrorForContext 清理本次账号测试错误中的运行时凭据，避免错误日志、SSE 和账号状态泄露密钥。
func redactAccountTestErrorForContext(c *gin.Context, message string) string {
	capture := accountTestUsageCaptureFromContext(c)
	if capture == nil {
		return message
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	for _, secret := range capture.redactionValues {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED_CREDENTIAL]")
		}
	}
	return redactAccountTestCredentialFields(message)
}

// recordAccountTestUsageJSON 从上游 usage 对象提取真实 token，避免管理员测试伪造费用。
func recordAccountTestUsageJSON(c *gin.Context, payload any) {
	capture := accountTestUsageCaptureFromContext(c)
	if capture == nil {
		return
	}
	recordAccountTestUsageJSONValue(capture, payload)
}

func recordAccountTestUsageJSONValue(capture *accountTestUsageCapture, payload any) {
	switch value := payload.(type) {
	case map[string]any:
		if usage, ok := value["usage"]; ok {
			if usageMap, ok := usage.(map[string]any); ok {
				recordAccountTestUsageMap(capture, usageMap)
			}
		}
		if usage, ok := value["usageMetadata"]; ok {
			if usageMap, ok := usage.(map[string]any); ok {
				recordAccountTestUsageMap(capture, usageMap)
			}
		}
		for _, child := range value {
			recordAccountTestUsageJSONValue(capture, child)
		}
	case []any:
		for _, child := range value {
			recordAccountTestUsageJSONValue(capture, child)
		}
	}
}

// recordAccountTestUsageMap 兼容顶层和 input/prompt_tokens_details 中的缓存字段。
func recordAccountTestUsageMap(capture *accountTestUsageCapture, usageMap map[string]any) {
	if capture == nil {
		return
	}
	cacheCreationTokens := accountTestUsageTokenValue(usageMap, "cache_creation_input_tokens", "cache_creation_tokens", "prompt_cache_miss_tokens", "cache_write_tokens", "cachedContentTokenCount")
	cacheReadTokens := accountTestUsageTokenValue(usageMap, "cache_read_input_tokens", "cache_read_tokens", "cached_tokens", "prompt_cache_hit_tokens", "cache_hit_tokens")
	for _, detailKey := range []string{"input_tokens_details", "prompt_tokens_details"} {
		details, ok := usageMap[detailKey].(map[string]any)
		if !ok {
			continue
		}
		cacheCreationTokens = maxAccountTestUsageInt(cacheCreationTokens, accountTestUsageTokenValue(details, "cache_creation_input_tokens", "cache_creation_tokens", "prompt_cache_miss_tokens", "cache_write_tokens"))
		cacheReadTokens = maxAccountTestUsageInt(cacheReadTokens, accountTestUsageTokenValue(details, "cache_read_input_tokens", "cache_read_tokens", "cached_tokens", "prompt_cache_hit_tokens", "cache_hit_tokens"))
	}
	capture.addUsage(
		accountTestUsageTokenValue(usageMap, "input_tokens", "prompt_tokens", "promptTokenCount"),
		accountTestUsageTokenValue(usageMap, "output_tokens", "completion_tokens", "candidatesTokenCount"),
		cacheCreationTokens,
		cacheReadTokens,
	)
}

func accountTestUsageTokenValue(values map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := values[key].(type) {
		case float64:
			if value > 0 {
				return int(value)
			}
		case int:
			if value > 0 {
				return value
			}
		case int64:
			if value > 0 {
				return int(value)
			}
		}
	}
	return 0
}

func (s *AccountTestService) SetAccountTestUsageRepository(repo AccountTestUsageRepository) {
	s.accountTestUsageRepo = repo
}

// supportsAccountTestUsage 保留统一入口，所有已知和未来新增平台都自动记录。
func supportsAccountTestUsage(platform string) bool {
	return strings.TrimSpace(platform) != ""
}

func (s *AccountTestService) startAccountTestUsage(c *gin.Context, account *Account, modelID, mode string) func(error) {
	if s == nil || s.accountTestUsageRepo == nil || account == nil {
		return func(error) {}
	}
	if !supportsAccountTestUsage(account.Platform) {
		return func(error) {}
	}
	if c == nil || c.Request == nil {
		return func(error) {}
	}
	if accountTestUsageCaptureFromContext(c) != nil {
		return func(error) {}
	}

	capture := newAccountTestUsageCapture(account, normalizeAccountTestMode(mode))
	capture.setRequest(modelID, "")
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), accountTestUsageCaptureKey{}, capture))
	return func(testErr error) {
		record := capture.snapshot(testErr)
		if err := s.accountTestUsageRepo.Create(c.Request.Context(), &record); err != nil {
			log.Printf("failed to persist account test usage: account_id=%d error=%v", account.ID, err)
		}
	}
}
