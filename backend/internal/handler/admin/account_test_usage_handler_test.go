package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// accountTestUsageHandlerRepoCapture 捕获 handler 转发的独立测试查询参数。
type accountTestUsageHandlerRepoCapture struct {
	service.AccountTestUsageRepository
	listParams   pagination.PaginationParams
	listFilters  usagestats.AccountTestUsageGlobalFilters
	statsFilters usagestats.AccountTestUsageGlobalFilters
}

// ListGlobal 返回稳定的空分页结果，供 handler 参数测试使用。
func (r *accountTestUsageHandlerRepoCapture) ListGlobal(_ context.Context, params pagination.PaginationParams, filters usagestats.AccountTestUsageGlobalFilters) ([]usagestats.AccountTestUsageRecordView, *pagination.PaginationResult, error) {
	r.listParams = params
	r.listFilters = filters
	return []usagestats.AccountTestUsageRecordView{}, &pagination.PaginationResult{Total: 0, Page: params.Page, PageSize: params.Limit(), Pages: 1}, nil
}

// GetGlobalStats 返回稳定的空统计结果，供 handler 参数测试使用。
func (r *accountTestUsageHandlerRepoCapture) GetGlobalStats(_ context.Context, filters usagestats.AccountTestUsageGlobalFilters) (*usagestats.AccountTestUsageGlobalStats, error) {
	r.statsFilters = filters
	return &usagestats.AccountTestUsageGlobalStats{ByPlatform: []usagestats.AccountTestUsagePlatformStats{}, ByModel: []usagestats.AccountTestUsageModelStats{}}, nil
}

// newAccountTestUsageHandlerTestRouter 创建只挂载独立账号测试端点的测试路由。
func newAccountTestUsageHandlerTestRouter(repo *accountTestUsageHandlerRepoCapture) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewUsageHandlerWithAccountTestUsage(nil, nil, nil, nil, repo)
	router := gin.New()
	router.GET("/admin/usage/test-logs", h.ListTestLogs)
	router.GET("/admin/usage/test-stats", h.TestStats)
	return router
}

// TestListTestLogsParsesIndependentFilters 验证列表接口不会复用正式 usage_logs 筛选器。
func TestListTestLogsParsesIndependentFilters(t *testing.T) {
	repo := &accountTestUsageHandlerRepoCapture{}
	router := newAccountTestUsageHandlerTestRouter(repo)
	req := httptest.NewRequest(http.MethodGet, "/admin/usage/test-logs?page=2&page_size=37&start_date=2026-08-01&end_date=2026-08-03&timezone=Asia/Shanghai&account_id=42&platform=deepseek&model=deepseek-chat&test_mode=responses&success=false", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 2, repo.listParams.Page)
	require.Equal(t, 37, repo.listParams.PageSize)
	require.Equal(t, "created_at", repo.listParams.SortBy)
	require.Equal(t, "desc", repo.listParams.SortOrder)
	require.Equal(t, int64(42), repo.listFilters.AccountID)
	require.Equal(t, "deepseek", repo.listFilters.Platform)
	require.Equal(t, "deepseek-chat", repo.listFilters.Model)
	require.Equal(t, "responses", repo.listFilters.TestMode)
	require.NotNil(t, repo.listFilters.Success)
	require.False(t, *repo.listFilters.Success)
	require.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60)), repo.listFilters.StartTime.In(time.FixedZone("CST", 8*60*60)))
	require.Equal(t, time.Date(2026, 8, 4, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60)), repo.listFilters.EndTime.In(time.FixedZone("CST", 8*60*60)))
}

// TestTestStatsParsesIndependentFilters 验证统计接口和列表接口共享同一套筛选语义。
func TestTestStatsParsesIndependentFilters(t *testing.T) {
	repo := &accountTestUsageHandlerRepoCapture{}
	router := newAccountTestUsageHandlerTestRouter(repo)
	req := httptest.NewRequest(http.MethodGet, "/admin/usage/test-stats?platform=grok&success=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "grok", repo.statsFilters.Platform)
	require.NotNil(t, repo.statsFilters.Success)
	require.True(t, *repo.statsFilters.Success)
}

// TestListTestLogsRejectsInvalidFilters 验证非法参数返回明确的中文 400 错误。
func TestListTestLogsRejectsInvalidFilters(t *testing.T) {
	repo := &accountTestUsageHandlerRepoCapture{}
	router := newAccountTestUsageHandlerTestRouter(repo)
	req := httptest.NewRequest(http.MethodGet, "/admin/usage/test-logs?success=maybe", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestListTestLogsRejectsReversedDateRange 验证倒置日期不会被当作有效查询范围。
func TestListTestLogsRejectsReversedDateRange(t *testing.T) {
	repo := &accountTestUsageHandlerRepoCapture{}
	router := newAccountTestUsageHandlerTestRouter(repo)
	req := httptest.NewRequest(http.MethodGet, "/admin/usage/test-logs?start_date=2026-08-04&end_date=2026-08-01", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "start_date")
}
