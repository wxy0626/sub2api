package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type availableModelsAdminService struct {
	*stubAdminService
	account service.Account
}

func (s *availableModelsAdminService) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	if s.account.ID == id {
		acc := s.account
		return &acc, nil
	}
	return s.stubAdminService.GetAccount(context.Background(), id)
}

func setupAvailableModelsRouter(adminSvc service.AdminService) *gin.Engine {
	return setupAvailableModelsRouterWithUpstream(adminSvc, nil)
}

// setupAvailableModelsRouterWithUpstream 创建可注入上游模型响应的管理端测试路由。
func setupAvailableModelsRouterWithUpstream(adminSvc service.AdminService, upstream service.HTTPUpstream) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	accountTestSvc := newAvailableModelsAccountTestService(upstream)
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, accountTestSvc, nil, nil, nil, nil, nil)
	router.GET("/api/v1/admin/accounts/:id/models", handler.GetAvailableModels)
	return router
}

// newAvailableModelsAccountTestService 创建使用测试上游客户端的账号模型同步服务。
func newAvailableModelsAccountTestService(upstream service.HTTPUpstream) *service.AccountTestService {
	return service.NewAccountTestService(
		nil,
		nil,
		nil,
		nil,
		nil,
		upstream,
		&config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		nil,
	)
}

type syncUpstreamHTTPUpstream struct {
	resp      *http.Response
	err       error
	lastReq   *http.Request
	responses []*http.Response
}

func (u *syncUpstreamHTTPUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.lastReq = req
	if u.err != nil {
		return nil, u.err
	}
	if len(u.responses) > 0 {
		resp := u.responses[0]
		u.responses = u.responses[1:]
		return resp, nil
	}
	return u.resp, nil
}

func (u *syncUpstreamHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func setupSyncUpstreamModelsRouter(adminSvc service.AdminService, upstream service.HTTPUpstream) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	accountTestSvc := newAvailableModelsAccountTestService(upstream)
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, accountTestSvc, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/:id/models/sync-upstream", handler.SyncUpstreamModels)
	router.POST("/api/v1/admin/accounts/models/sync-upstream-preview", handler.SyncUpstreamModelsPreview)
	return router
}

// setupSyncUpstreamModelsPreviewRouter 创建可注入上游错误的模型目录预览测试路由。
func setupSyncUpstreamModelsPreviewRouter(upstream service.HTTPUpstream) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	accountTestSvc := newAvailableModelsAccountTestService(upstream)
	handler := NewAccountHandler(newStubAdminService(), nil, nil, nil, nil, nil, nil, nil, accountTestSvc, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/models/sync-upstream-preview", handler.SyncUpstreamModelsPreview)
	return router
}

func TestAccountHandlerGetAvailableModels_GrokUsesXAIModels(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       44,
			Name:     "grok-oauth",
			Platform: service.PlatformGrok,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"grok-4.3": "grok-4.3",
				},
			},
		},
	}
	router := setupAvailableModelsRouter(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/44/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	require.Equal(t, "grok-4.3", resp.Data[0].ID)
}

func TestAccountHandlerGetAvailableModels_GrokDefaultsToXAIModelsWithoutMapping(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       45,
			Name:     "grok-oauth-defaults",
			Platform: service.PlatformGrok,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
		},
	}
	router := setupAvailableModelsRouter(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/45/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Data)

	var ids []string
	for _, model := range resp.Data {
		id := model.ID
		ids = append(ids, id)
		require.NotContains(t, strings.ToLower(id), "claude")
	}
	require.Contains(t, ids, "grok-4.3")
	require.Contains(t, ids, "grok-build-0.1")
}

// TestAccountHandlerGetAvailableModels_GrokAPIKeyFetchesCallableModels 验证模型测试下拉只返回上游 /v1/models 目录。
func TestAccountHandlerGetAvailableModels_GrokAPIKeyFetchesUpstreamModels(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       50,
			Name:     "grok-apikey-upstream",
			Platform: service.PlatformGrok,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"api_key":  "grok-api-key",
				"base_url": "https://api.x.ai",
			},
		},
	}
	upstream := &syncUpstreamHTTPUpstream{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"grok-4.5"},{"id":"grok-api-only"}]}`)),
	}}
	router := setupAvailableModelsRouterWithUpstream(svc, upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/50/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, []string{"grok-4.5", "grok-api-only"}, []string{resp.Data[0].ID, resp.Data[1].ID})
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://api.x.ai/v1/models", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer grok-api-key", upstream.lastReq.Header.Get("Authorization"))
}

// TestAccountHandlerGetAvailableModels_GrokAPIKeyUsesSavedWhitelist 验证模型测试下拉与已保存白名单完全一致。
func TestAccountHandlerGetAvailableModels_GrokAPIKeyUsesSavedWhitelist(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       51,
			Name:     "grok-apikey-whitelist",
			Platform: service.PlatformGrok,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"api_key": "grok-api-key",
				"model_mapping": map[string]any{
					"grok-4.5":      "grok-4.5",
					"grok-api-only": "grok-api-only",
				},
			},
		},
	}
	upstream := &syncUpstreamHTTPUpstream{err: errors.New("上游不应被已保存白名单请求")}
	router := setupAvailableModelsRouterWithUpstream(svc, upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/51/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, []string{"grok-4.5", "grok-api-only"}, []string{resp.Data[0].ID, resp.Data[1].ID})
	require.Nil(t, upstream.lastReq)
}

// TestAccountHandlerGetAvailableModels_GrokAPIKeyErrorReturnsChineseDetails 验证 Grok API Key 获取模型失败时返回脱敏技术详情。
func TestAccountHandlerGetAvailableModels_GrokAPIKeyErrorReturnsChineseDetails(t *testing.T) {
	apiKey := "grok-api-key-secret"
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       52,
			Name:     "grok-apikey-error",
			Platform: service.PlatformGrok,
			Type:     service.AccountTypeAPIKey,
			Credentials: map[string]any{
				"api_key":  apiKey,
				"base_url": "https://api.x.ai",
			},
		},
	}
	upstream := &syncUpstreamHTTPUpstream{err: errors.New("request failed: Authorization: Bearer " + apiKey + "; Cookie=session-secret; token=token-secret")}
	router := setupAvailableModelsRouterWithUpstream(svc, upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/52/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "获取 Grok 上游模型失败")
	require.Contains(t, body, "原始技术详情")
	require.Contains(t, body, "Failed to request upstream model list")
	require.NotContains(t, body, apiKey)
	require.NotContains(t, body, "grok-4.5")
}

// TestAccountHandlerSyncUpstreamModels_GrokAPIKeyErrorReturnsChineseDetails 验证 Grok 同步失败不回退静态模型且会脱敏凭据。
func TestAccountHandlerSyncUpstreamModels_GrokAPIKeyErrorReturnsChineseDetails(t *testing.T) {
	apiKey := "grok-sync-api-key-secret"
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       53,
			Name:     "grok-apikey-sync-error",
			Platform: service.PlatformGrok,
			Type:     service.AccountTypeAPIKey,
			Credentials: map[string]any{
				"api_key":  apiKey,
				"base_url": "https://api.x.ai",
			},
		},
	}
	upstream := &syncUpstreamHTTPUpstream{err: errors.New("request failed: Authorization: Bearer " + apiKey + "; api_key=" + apiKey + "; access_token=access-secret")}
	router := setupSyncUpstreamModelsRouter(svc, upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/53/models/sync-upstream", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "获取 Grok 上游模型失败")
	require.Contains(t, body, "原始技术详情")
	require.Contains(t, body, "Failed to request upstream model list")
	require.NotContains(t, body, apiKey)
	require.NotContains(t, body, "access-secret")
	require.NotContains(t, body, "grok-4.5")
}

// TestAccountHandlerSyncUpstreamModelsPreview_GrokAPIKeyErrorReturnsChineseDetails 验证 Grok 预览同步错误路径同样统一脱敏。
func TestAccountHandlerSyncUpstreamModelsPreview_GrokAPIKeyErrorReturnsChineseDetails(t *testing.T) {
	apiKey := "grok-preview-api-key-secret"
	router := setupSyncUpstreamModelsPreviewRouter(&syncUpstreamHTTPUpstream{err: errors.New("request failed: Authorization: Bearer " + apiKey + "; password=password-secret")})

	rec := httptest.NewRecorder()
	body := strings.NewReader("{\"platform\":\"grok\",\"type\":\"apikey\",\"base_url\":\"https://api.x.ai\",\"api_key\":\"" + apiKey + "\"}")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/models/sync-upstream-preview", body)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	responseBody := rec.Body.String()
	require.Contains(t, responseBody, "获取 Grok 上游模型失败")
	require.Contains(t, responseBody, "原始技术详情")
	require.Contains(t, responseBody, "Failed to request upstream model list")
	require.NotContains(t, responseBody, apiKey)
	require.NotContains(t, responseBody, "password-secret")
	require.NotContains(t, responseBody, "grok-4.5")
}

func TestAccountHandlerGetAvailableModels_OpenAIOAuthUsesExplicitModelMapping(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       42,
			Name:     "openai-oauth",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"gpt-5": "gpt-5.1",
				},
			},
		},
	}
	router := setupAvailableModelsRouter(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/42/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	require.Equal(t, "gpt-5", resp.Data[0].ID)
}

func TestAccountHandlerGetAvailableModels_OpenAIOAuthPassthroughFallsBackToDefaults(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       43,
			Name:     "openai-oauth-passthrough",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"gpt-5": "gpt-5.1",
				},
			},
			Extra: map[string]any{
				"openai_passthrough": true,
			},
		},
	}
	router := setupAvailableModelsRouter(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/43/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Data)
	require.NotEqual(t, "gpt-5", resp.Data[0].ID)
}

func TestAccountHandlerGetAvailableModels_OpenAIAPIKeyDefaultsToConcreteGPT56Sol(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       46,
			Name:     "openai-apikey",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"api_key": "test-key",
			},
		},
	}
	router := setupAvailableModelsRouter(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/46/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Data)
	require.Equal(t, "gpt-5.6-sol", resp.Data[0].ID)
}

// TestAccountHandlerGetAvailableModels_OpenAICompatibleAPIKeyReturnsFullUpstreamCatalog
// 验证 OpenAI 兼容端点（第三方 v1 代理）返回上游全部模型，不再被 GPT 白名单裁剪。
func TestAccountHandlerGetAvailableModels_OpenAICompatibleAPIKeyReturnsFullUpstreamCatalog(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       60,
			Name:     "openai-compatible-proxy",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"api_key":  "proxy-key",
				"base_url": "https://proxy.example.com/v1",
			},
		},
	}
	upstream := &syncUpstreamHTTPUpstream{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"data":[{"id":"gpt-4o"},{"id":"gemini-3-pro-preview"},{"id":"gpt-5.6-luna"},{"id":"claude-sonnet-4"}]}`,
		)),
	}}
	router := setupAvailableModelsRouterWithUpstream(svc, upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/60/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data []struct {
			ID          string `json:"id"`
			OwnedBy     string `json:"owned_by"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	ids := make([]string, 0, len(resp.Data))
	for _, model := range resp.Data {
		ids = append(ids, model.ID)
		// owned_by 标记上游目录来源，前端据此放行全部上游模型。
		require.Equal(t, "upstream", model.OwnedBy)
	}
	require.Equal(t, []string{"claude-sonnet-4", "gemini-3-pro-preview", "gpt-4o", "gpt-5.6-luna"}, ids)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://proxy.example.com/v1/models", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer proxy-key", upstream.lastReq.Header.Get("Authorization"))
}

// TestAccountHandlerGetAvailableModels_OpenAIAPIKeyUpstreamFailureFallsBackToDefaults
// 验证上游不支持 /v1/models 或请求失败时静默回落内置默认模型集，模型测试不被阻断。
func TestAccountHandlerGetAvailableModels_OpenAIAPIKeyUpstreamFailureFallsBackToDefaults(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       61,
			Name:     "openai-apikey-upstream-unavailable",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"api_key":  "proxy-key-secret",
				"base_url": "https://proxy.example.com/v1",
			},
		},
	}
	upstream := &syncUpstreamHTTPUpstream{err: errors.New("connection refused: Authorization: Bearer proxy-key-secret")}
	router := setupAvailableModelsRouterWithUpstream(svc, upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/61/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "proxy-key-secret")

	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Data)
	require.Equal(t, "gpt-5.6-sol", resp.Data[0].ID)
}

// TestAccountHandlerGetAvailableModels_OpenAIAPIKeyWithMappingSkipsUpstream
// 验证管理员已显式配置 model_mapping 时仍以映射为准，不额外访问上游。
func TestAccountHandlerGetAvailableModels_OpenAIAPIKeyWithMappingSkipsUpstream(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       62,
			Name:     "openai-apikey-mapped",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"api_key": "proxy-key",
				"model_mapping": map[string]any{
					"gemini:130": "gemini-3-pro-preview",
				},
			},
		},
	}
	upstream := &syncUpstreamHTTPUpstream{err: errors.New("上游不应被已保存映射请求")}
	router := setupAvailableModelsRouterWithUpstream(svc, upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/62/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	require.Equal(t, "gemini:130", resp.Data[0].ID)
	require.Nil(t, upstream.lastReq)
}

func TestAccountHandlerGetAvailableModels_OpenAISparkShadowReturnsMappingModels(t *testing.T) {
	parentID := int64(100)
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:              44,
			Name:            "openai-spark-shadow",
			Platform:        service.PlatformOpenAI,
			Type:            service.AccountTypeOAuth,
			Status:          service.StatusActive,
			ParentAccountID: &parentID,
			QuotaDimension:  service.QuotaDimensionSpark,
			Credentials: map[string]any{
				"model_mapping": map[string]any{
					"gpt-5.3-codex-spark": "gpt-5.3-codex-spark",
				},
			},
		},
	}
	router := setupAvailableModelsRouter(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/44/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ids := make([]string, 0, len(resp.Data))
	for _, m := range resp.Data {
		ids = append(ids, m.ID)
	}
	require.ElementsMatch(t, []string{
		"gpt-5.3-codex-spark",
	}, ids, "影子可用模型由 model_mapping 派生（非写死）")
}

// TestAccountHandlerGetAvailableModels_DeepSeekFetchesUpstreamModelsWithoutMapping 验证无映射时使用上游模型目录。
func TestAccountHandlerGetAvailableModels_DeepSeekFetchesUpstreamModelsWithoutMapping(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       47,
			Name:     "deepseek-apikey-defaults",
			Platform: service.PlatformDeepSeek,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"api_key":  "test-key",
				"base_url": "https://api.deepseek.com",
			},
		},
	}
	upstream := &syncUpstreamHTTPUpstream{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"deepseek-v4-pro"},{"id":"deepseek-live-only"}]}`)),
	}}
	router := setupAvailableModelsRouterWithUpstream(svc, upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/47/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data []struct {
			ID          string `json:"id"`
			Type        string `json:"type"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	wantModelIDs := []string{"deepseek-live-only", "deepseek-v4-pro"}
	require.Len(t, resp.Data, len(wantModelIDs))
	require.Equal(t, []string{"deepseek-v4-flash", "deepseek-v4-pro"}, service.DeepSeekDefaultModelIDs)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://api.deepseek.com/v1/models", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer test-key", upstream.lastReq.Header.Get("Authorization"))

	for index, model := range resp.Data {
		require.Equal(t, wantModelIDs[index], model.ID)
		require.Equal(t, model.ID, model.DisplayName)
		require.Equal(t, "model", model.Type)
	}
}

// TestAccountHandlerGetAvailableModels_DeepSeekUpstreamErrorReturnsChineseDetails 验证同步失败不会静默回退静态目录。
func TestAccountHandlerGetAvailableModels_DeepSeekUpstreamErrorReturnsChineseDetails(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       49,
			Name:     "deepseek-apikey-upstream-error",
			Platform: service.PlatformDeepSeek,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"api_key": "test-key",
			},
		},
	}
	upstream := &syncUpstreamHTTPUpstream{resp: &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":"UPSTREAM_SECRET should not be exposed"}`)),
	}}
	router := setupAvailableModelsRouterWithUpstream(svc, upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/49/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "获取 DeepSeek 上游模型失败")
	require.Contains(t, rec.Body.String(), "原始技术详情")
	require.Contains(t, rec.Body.String(), "HTTP 502")
	require.NotContains(t, rec.Body.String(), "UPSTREAM_SECRET")
	require.NotContains(t, rec.Body.String(), "deepseek-v4-flash")
	require.NotContains(t, rec.Body.String(), "deepseek-v4-pro")
}

func TestAccountHandlerGetAvailableModels_DeepSeekMappingReturnsSortedModelObjects(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       48,
			Name:     "deepseek-apikey-mapped",
			Platform: service.PlatformDeepSeek,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"api_key": "test-key",
				"model_mapping": map[string]any{
					"deepseek-zeta":  "deepseek-chat",
					"deepseek-alpha": "deepseek-reasoner",
				},
			},
		},
	}
	router := setupAvailableModelsRouter(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/48/models", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data []struct {
			ID          string `json:"id"`
			Type        string `json:"type"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2)

	require.Equal(t, "deepseek-alpha", resp.Data[0].ID)
	require.Equal(t, "deepseek-zeta", resp.Data[1].ID)
	for _, model := range resp.Data {
		require.Equal(t, model.ID, model.DisplayName)
		require.Equal(t, "model", model.Type)
	}
}

// TestAccountHandlerSyncUpstreamModels_OpenAICompatibleProxyReturnsFullCatalog
// 验证「同步上游模型」对第三方 OpenAI 兼容端点返回全量目录，gemini-* 等模型可写入账号白名单。
func TestAccountHandlerSyncUpstreamModels_OpenAICompatibleProxyReturnsFullCatalog(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       63,
			Name:     "openai-compatible-sync",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"api_key":  "proxy-key",
				"base_url": "http://host.docker.internal:7860/v1",
			},
		},
	}
	upstream := &syncUpstreamHTTPUpstream{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"data":[{"id":"gemini-3.5-flash"},{"id":"gemini-3-pro-preview"},{"id":"gpt-4o"}]}`,
		)),
	}}
	router := setupSyncUpstreamModelsRouter(svc, upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/63/models/sync-upstream", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data struct {
			Models []string `json:"models"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, []string{"gemini-3-pro-preview", "gemini-3.5-flash", "gpt-4o"}, resp.Data.Models)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "http://host.docker.internal:7860/v1/models", upstream.lastReq.URL.String())
}

func TestAccountHandlerSyncUpstreamModels_ConfigErrorReturnsBadRequest(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       44,
			Name:     "openai-apikey-missing-key",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"base_url": "https://openai.example.com/v1",
			},
		},
	}
	router := setupSyncUpstreamModelsRouter(svc, &syncUpstreamHTTPUpstream{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/44/models/sync-upstream", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "No OpenAI API key is available")
}

func TestAccountHandlerSyncUpstreamModelsReturnsCapabilityMetadata(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID: 48, Name: "custom-openai", Platform: service.PlatformOpenAI,
			Type: service.AccountTypeAPIKey, Status: service.StatusActive,
			Credentials: map[string]any{"api_key": "key", "base_url": "https://provider.example/v1"},
		},
	}
	upstream := &syncUpstreamHTTPUpstream{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"models":[{
			"id":"custom-thinking-model",
			"reasoning":true,
			"default_reasoning_level":"high",
			"supported_reasoning_levels":["low","high"],
			"input_modalities":["text","image"],
			"context_window":256000
		}]}`)),
	}}
	router := setupSyncUpstreamModelsRouter(svc, upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/48/models/sync-upstream", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data service.UpstreamModelCatalog `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, []string{"custom-thinking-model"}, resp.Data.Models)
	metadata := resp.Data.Metadata["custom-thinking-model"]
	require.NotNil(t, metadata.Reasoning)
	require.True(t, *metadata.Reasoning)
	require.Equal(t, []string{"low", "high"}, metadata.SupportedReasoningLevels)
	require.Equal(t, []string{"text", "image"}, metadata.InputModalities)
}

// Scenario: 创建账号 preview 将具体 mapping 传给 404/405 配置回退。
func TestAccountHandlerSyncUpstreamModelsPreviewUsesProvidedModelMapping(t *testing.T) {
	upstream := &syncUpstreamHTTPUpstream{responses: []*http.Response{
		{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":"not found"}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"configured-provider": {
					"api": "https://provider.example/v1",
					"models": {
						"glm-5.3": {
							"id": "glm-5.3",
							"reasoning": true,
							"reasoning_options": [{"type":"effort","values":["low","high"]}],
							"modalities": {"input":["text"],"output":["text"]},
							"limit": {"context":1000000,"output":131072}
						}
					}
				}
			}`)),
		},
	}}
	router := setupSyncUpstreamModelsRouter(newStubAdminService(), upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/accounts/models/sync-upstream-preview",
		strings.NewReader(`{
			"platform":"openai",
			"type":"apikey",
			"base_url":"https://provider.example/v1",
			"api_key":"key",
			"model_mapping":{"public-glm":"glm-5.3"}
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data service.UpstreamModelCatalog `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, []string{"glm-5.3"}, resp.Data.Models)
	require.Equal(t, []string{"low", "high"}, resp.Data.Metadata["glm-5.3"].SupportedReasoningLevels)
}

func TestAccountHandlerSyncUpstreamModels_UpstreamErrorDoesNotExposeBody(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       45,
			Name:     "openai-apikey-upstream-error",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"api_key":  "openai-key",
				"base_url": "https://openai.example.com/v1",
			},
		},
	}
	upstream := &syncUpstreamHTTPUpstream{resp: &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":"SECRET_TOKEN should not be exposed"}`)),
	}}
	router := setupSyncUpstreamModelsRouter(svc, upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/45/models/sync-upstream", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "Upstream model list request failed with HTTP 502")
	require.NotContains(t, rec.Body.String(), "SECRET_TOKEN")
}

// TestAccountHandlerSyncUpstreamModels_DeepSeekErrorReturnsChineseDetailsAndRedactsCredentials 验证 DeepSeek 同步失败不回退静态模型且不泄露凭据。
func TestAccountHandlerSyncUpstreamModels_DeepSeekErrorReturnsChineseDetailsAndRedactsCredentials(t *testing.T) {
	svc := &availableModelsAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       46,
			Name:     "deepseek-apikey-sync-error",
			Platform: service.PlatformDeepSeek,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"api_key":  "deepseek-api-key-secret",
				"base_url": "https://api.deepseek.com",
			},
		},
	}
	upstream := &syncUpstreamHTTPUpstream{err: errors.New(
		"request failed: Authorization: Bearer deepseek-api-key-secret; api_key=deepseek-api-key-secret; access_token=deepseek-access-token; token=deepseek-token",
	)}
	router := setupSyncUpstreamModelsRouter(svc, upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/46/models/sync-upstream", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "获取 DeepSeek 上游模型失败")
	require.Contains(t, body, "原始技术详情")
	require.Contains(t, body, "Failed to request upstream model list")
	require.NotContains(t, body, "deepseek-api-key-secret")
	require.NotContains(t, body, "deepseek-access-token")
	require.NotContains(t, body, "deepseek-token")
	require.NotContains(t, body, "deepseek-v4-flash")
	require.NotContains(t, body, "deepseek-v4-pro")
}

// TestAccountHandlerSyncUpstreamModelsPreview_DeepSeekErrorReturnsChineseDetailsAndRedactsCredentials 验证创建账号预览同步沿用 DeepSeek 错误边界。
func TestAccountHandlerSyncUpstreamModelsPreview_DeepSeekErrorReturnsChineseDetailsAndRedactsCredentials(t *testing.T) {
	upstream := &syncUpstreamHTTPUpstream{err: errors.New(
		"request failed: Authorization: Bearer deepseek-preview-key; api_key=deepseek-preview-key; token=deepseek-preview-token",
	)}
	router := setupSyncUpstreamModelsPreviewRouter(upstream)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/models/sync-upstream-preview", strings.NewReader("{\"platform\":\"deepseek\",\"type\":\"apikey\",\"base_url\":\"https://api.deepseek.com\",\"api_key\":\"deepseek-preview-key\"}"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "获取 DeepSeek 上游模型失败")
	require.Contains(t, body, "原始技术详情")
	require.Contains(t, body, "Failed to request upstream model list")
	require.NotContains(t, body, "deepseek-preview-key")
	require.NotContains(t, body, "deepseek-preview-token")
	require.NotContains(t, body, "deepseek-v4-flash")
	require.NotContains(t, body, "deepseek-v4-pro")
}
