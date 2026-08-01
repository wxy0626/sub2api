//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// accountTestUsageMemoryRepo 在内存中保存账号测试用量，供回归测试检查最终落库记录。
type accountTestUsageMemoryRepo struct {
	mu      sync.Mutex
	records []usagestats.AccountTestUsageRecord
}

// Create 捕获一次账号测试结果。
func (r *accountTestUsageMemoryRepo) Create(_ context.Context, record *usagestats.AccountTestUsageRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, *record)
	return nil
}

// GetStats 不是本组回归测试的验证面。
func (r *accountTestUsageMemoryRepo) GetStats(context.Context, int64, time.Time, time.Time) (*usagestats.AccountTestUsageStatsResponse, error) {
	return nil, nil
}

// ListGlobal 不是账号测试写入回归测试的验证面。
func (r *accountTestUsageMemoryRepo) ListGlobal(context.Context, pagination.PaginationParams, usagestats.AccountTestUsageGlobalFilters) ([]usagestats.AccountTestUsageRecordView, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

// GetGlobalStats 不是账号测试写入回归测试的验证面。
func (r *accountTestUsageMemoryRepo) GetGlobalStats(context.Context, usagestats.AccountTestUsageGlobalFilters) (*usagestats.AccountTestUsageGlobalStats, error) {
	return nil, nil
}

// onlyAccountTestUsageRecord 返回唯一捕获记录并断言测试不会重复保存。
func (r *accountTestUsageMemoryRepo) onlyAccountTestUsageRecord(t *testing.T) usagestats.AccountTestUsageRecord {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.records, 1)
	return r.records[0]
}

// newAccountUsageTestService 创建启用账号测试用量捕获的服务。
func newAccountUsageTestService(repo AccountRepository, usageRepo AccountTestUsageRepository, upstream HTTPUpstream) *AccountTestService {
	return &AccountTestService{
		accountRepo:          repo,
		accountTestUsageRepo: usageRepo,
		httpUpstream:         upstream,
		cfg: &config.Config{Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		}},
	}
}

// newAccountTestUsageContext 创建管理员账号测试使用的 Gin 上下文。
func newAccountTestUsageContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)
	return c, recorder
}

func TestAccountTestUsage_GrokSuccessSavesOneRecord(t *testing.T) {
	account := &Account{
		ID: 401, Platform: PlatformGrok, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "grok-test-key", "base_url": "https://api.x.ai"},
	}
	accountRepo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	usageRepo := &accountTestUsageMemoryRepo{}
	upstream := &httpUpstreamRecorder{resp: newJSONResponse(http.StatusOK,
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"+
			"data: {\"type\":\"response.completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":4,\"input_tokens_details\":{\"cached_tokens\":2}}}\n\n")}
	service := newAccountUsageTestService(accountRepo, usageRepo, upstream)
	c, _ := newAccountTestUsageContext()

	require.NoError(t, service.TestAccountConnection(c, account.ID, "grok-4", "", AccountTestModeDefault))
	record := usageRepo.onlyAccountTestUsageRecord(t)
	require.Equal(t, "/v1/responses", record.Endpoint)
	require.Equal(t, http.StatusOK, record.StatusCode)
	require.Equal(t, 3, record.InputTokens)
	require.Equal(t, 4, record.OutputTokens)
	require.Equal(t, 2, record.CacheReadTokens)
	require.True(t, record.Success)
}

// TestAccountTestUsage_GrokHTTPFailureRedactsCredential 验证 Grok 错误响应不会把 API Key 写入 SSE、账号状态或测试记录。
func TestAccountTestUsage_GrokHTTPFailureRedactsCredential(t *testing.T) {
	account := &Account{
		ID: 405, Platform: PlatformGrok, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "grok-error-test-key", "base_url": "https://api.x.ai"},
	}
	accountRepo := &openAIAccountTestRepo{}
	accountRepo.accountsByID = map[int64]*Account{account.ID: account}
	usageRepo := &accountTestUsageMemoryRepo{}
	upstream := &httpUpstreamRecorder{resp: newJSONResponse(http.StatusUnauthorized, "{\"error\":{\"message\":\"invalid key grok-error-test-key\"}}")}
	service := newAccountUsageTestService(accountRepo, usageRepo, upstream)
	c, recorder := newAccountTestUsageContext()

	require.Error(t, service.TestAccountConnection(c, account.ID, "grok-4", "", AccountTestModeDefault))
	record := usageRepo.onlyAccountTestUsageRecord(t)
	require.False(t, record.Success)
	require.Equal(t, http.StatusUnauthorized, record.StatusCode)
	require.NotContains(t, record.ErrorMessage, "grok-error-test-key")
	require.NotContains(t, recorder.Body.String(), "grok-error-test-key")
	require.NotContains(t, accountRepo.setErrorMsg, "grok-error-test-key")
}

func TestAccountTestUsage_DeepSeekChatSuccessSavesOneRecord(t *testing.T) {
	account := deepSeekTestAccount()
	account.ID = 402
	accountRepo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	usageRepo := &accountTestUsageMemoryRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{newJSONResponse(http.StatusOK,
		"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"+
			"data: {\"choices\":[{\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":6,\"prompt_cache_hit_tokens\":2,\"prompt_cache_miss_tokens\":1}}\n\n"+
			"data: [DONE]\n\n")}}
	service := newAccountUsageTestService(accountRepo, usageRepo, upstream)
	c, _ := newAccountTestUsageContext()

	require.NoError(t, service.TestAccountConnection(c, account.ID, "deepseek-chat", "hi", AccountTestModeDefault))
	record := usageRepo.onlyAccountTestUsageRecord(t)
	require.Equal(t, "/v1/chat/completions", record.Endpoint)
	require.Equal(t, http.StatusOK, record.StatusCode)
	require.Equal(t, 5, record.InputTokens)
	require.Equal(t, 6, record.OutputTokens)
	require.Equal(t, 1, record.CacheCreationTokens)
	require.Equal(t, 2, record.CacheReadTokens)
	require.True(t, record.Success)
}

func TestAccountTestUsage_DeepSeekResponsesSuccessSavesOneRecord(t *testing.T) {
	account := deepSeekTestAccount()
	account.ID = 403
	accountRepo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	usageRepo := &accountTestUsageMemoryRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{newJSONResponse(http.StatusOK,
		"{\"id\":\"resp-test\",\"status\":\"completed\",\"usage\":{\"input_tokens\":7,\"output_tokens\":8}}")}}
	service := newAccountUsageTestService(accountRepo, usageRepo, upstream)
	c, _ := newAccountTestUsageContext()

	require.NoError(t, service.TestAccountConnection(c, account.ID, DeepSeekResponsesModel, "hi", AccountTestModeResponses))
	record := usageRepo.onlyAccountTestUsageRecord(t)
	require.Equal(t, "/v1/responses", record.Endpoint)
	require.Equal(t, http.StatusOK, record.StatusCode)
	require.Equal(t, 7, record.InputTokens)
	require.Equal(t, 8, record.OutputTokens)
	require.True(t, record.Success)
}

func TestAccountTestUsage_DeepSeekResponsesHTTPFailureSavesOneRedactedRecord(t *testing.T) {
	account := deepSeekTestAccount()
	account.ID = 404
	accountRepo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	usageRepo := &accountTestUsageMemoryRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{newJSONResponse(http.StatusUnauthorized,
		"{\"error\":{\"message\":\"invalid key sk-deepseek-test\"}}")}}
	service := newAccountUsageTestService(accountRepo, usageRepo, upstream)
	c, recorder := newAccountTestUsageContext()

	err := service.TestAccountConnection(c, account.ID, DeepSeekResponsesModel, "hi", AccountTestModeResponses)
	require.Error(t, err)
	record := usageRepo.onlyAccountTestUsageRecord(t)
	require.Equal(t, "/v1/responses", record.Endpoint)
	require.Equal(t, http.StatusUnauthorized, record.StatusCode)
	require.False(t, record.Success)
	require.NotContains(t, record.ErrorMessage, "sk-deepseek-test")
	require.NotContains(t, recorder.Body.String(), "sk-deepseek-test")
}

// TestAccountTestUsage_FuturePlatformUsesUnifiedCapture 验证未知平台也复用统一记录入口。
func TestAccountTestUsage_FuturePlatformUsesUnifiedCapture(t *testing.T) {
	account := &Account{
		ID: 406, Platform: "future-provider", Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "future-provider-test-key", "base_url": "https://api.example.test"},
	}
	accountRepo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	usageRepo := &accountTestUsageMemoryRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{newJSONResponse(http.StatusOK,
		"data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":2}}}\n\n"+
			"data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"ok\"}}\n\n"+
			"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\n"+
			"data: {\"type\":\"message_stop\"}\n\n")}}
	service := newAccountUsageTestService(accountRepo, usageRepo, upstream)
	c, _ := newAccountTestUsageContext()

	require.NoError(t, service.TestAccountConnection(c, account.ID, "future-model", "hi", AccountTestModeDefault))
	record := usageRepo.onlyAccountTestUsageRecord(t)
	require.Equal(t, "future-provider", record.Platform)
	require.Equal(t, "/v1/messages", record.Endpoint)
	require.Equal(t, 2, record.InputTokens)
	require.Equal(t, 3, record.OutputTokens)
	require.True(t, record.Success)
}

// TestAccountTestUsageHelpersIgnoreMissingCapture 验证非标准调用上下文不会触发空指针 panic。
func TestAccountTestUsageHelpersIgnoreMissingCapture(t *testing.T) {
	c, _ := newAccountTestUsageContext()
	require.NotPanics(t, func() {
		beginAccountTestUsageRequest(c, "model", "/endpoint")
		recordAccountTestUsageStatus(c, http.StatusOK)
	})
}

var _ AccountTestUsageRepository = (*accountTestUsageMemoryRepo)(nil)
