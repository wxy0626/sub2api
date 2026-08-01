// Package usagestats provides types for usage statistics and reporting.
package usagestats

import "time"

const (
	ModelSourceRequested = "requested"
	ModelSourceUpstream  = "upstream"
	ModelSourceMapping   = "mapping"
)

func IsValidModelSource(source string) bool {
	switch source {
	case ModelSourceRequested, ModelSourceUpstream, ModelSourceMapping:
		return true
	default:
		return false
	}
}

func NormalizeModelSource(source string) string {
	if IsValidModelSource(source) {
		return source
	}
	return ModelSourceRequested
}

// DashboardStats 仪表盘统计
type DashboardStats struct {
	// 用户统计
	TotalUsers    int64 `json:"total_users"`
	TodayNewUsers int64 `json:"today_new_users"` // 今日新增用户数
	ActiveUsers   int64 `json:"active_users"`    // 今日有请求的用户数
	// 小时活跃用户数（UTC 当前小时）
	HourlyActiveUsers int64 `json:"hourly_active_users"`

	// 预聚合新鲜度
	StatsUpdatedAt string `json:"stats_updated_at"`
	StatsStale     bool   `json:"stats_stale"`

	// API Key 统计
	TotalAPIKeys  int64 `json:"total_api_keys"`
	ActiveAPIKeys int64 `json:"active_api_keys"` // 状态为 active 的 API Key 数

	// 账户统计
	TotalAccounts     int64 `json:"total_accounts"`
	NormalAccounts    int64 `json:"normal_accounts"`    // 正常账户数 (schedulable=true, status=active)
	ErrorAccounts     int64 `json:"error_accounts"`     // 异常账户数 (status=error)
	RateLimitAccounts int64 `json:"ratelimit_accounts"` // 限流账户数
	OverloadAccounts  int64 `json:"overload_accounts"`  // 过载账户数

	// 累计 Token 使用统计
	TotalRequests            int64   `json:"total_requests"`
	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64   `json:"total_cache_read_tokens"`
	TotalTokens              int64   `json:"total_tokens"`
	TotalCost                float64 `json:"total_cost"`         // 累计标准计费
	TotalActualCost          float64 `json:"total_actual_cost"`  // 累计实际扣除
	TotalAccountCost         float64 `json:"total_account_cost"` // 累计账号成本

	// 今日 Token 使用统计
	TodayRequests            int64   `json:"today_requests"`
	TodayInputTokens         int64   `json:"today_input_tokens"`
	TodayOutputTokens        int64   `json:"today_output_tokens"`
	TodayCacheCreationTokens int64   `json:"today_cache_creation_tokens"`
	TodayCacheReadTokens     int64   `json:"today_cache_read_tokens"`
	TodayTokens              int64   `json:"today_tokens"`
	TodayCost                float64 `json:"today_cost"`         // 今日标准计费
	TodayActualCost          float64 `json:"today_actual_cost"`  // 今日实际扣除
	TodayAccountCost         float64 `json:"today_account_cost"` // 今日账号成本

	// 系统运行统计
	AverageDurationMs float64 `json:"average_duration_ms"` // 平均响应时间

	// 性能指标
	Rpm int64 `json:"rpm"` // 近5分钟平均每分钟请求数
	Tpm int64 `json:"tpm"` // 近5分钟平均每分钟Token数
}

// TrendDataPoint represents a single point in trend data
type TrendDataPoint struct {
	Date                string  `json:"date"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	Cost                float64 `json:"cost"`        // 标准计费
	ActualCost          float64 `json:"actual_cost"` // 实际扣除
}

// ModelStat represents usage statistics for a single model
type ModelStat struct {
	Model               string  `json:"model"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	Cost                float64 `json:"cost"`         // 标准计费
	ActualCost          float64 `json:"actual_cost"`  // 实际扣除
	AccountCost         float64 `json:"account_cost"` // 账号成本
}

// EndpointStat represents usage statistics for a single request endpoint.
type EndpointStat struct {
	Endpoint    string  `json:"endpoint"`
	Requests    int64   `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	Cost        float64 `json:"cost"`        // 标准计费
	ActualCost  float64 `json:"actual_cost"` // 实际扣除
}

// GroupUsageSummary represents today's and cumulative cost for a single group.
type GroupUsageSummary struct {
	GroupID   int64   `json:"group_id"`
	TodayCost float64 `json:"today_cost"`
	TotalCost float64 `json:"total_cost"`
}

// GroupStat represents usage statistics for a single group
type GroupStat struct {
	GroupID     int64   `json:"group_id"`
	GroupName   string  `json:"group_name"`
	Requests    int64   `json:"requests"`
	TotalTokens int64   `json:"total_tokens"`
	Cost        float64 `json:"cost"`         // 标准计费
	ActualCost  float64 `json:"actual_cost"`  // 实际扣除
	AccountCost float64 `json:"account_cost"` // 账号成本
}

// UserUsageTrendPoint represents user usage trend data point
type UserUsageTrendPoint struct {
	Date       string  `json:"date"`
	UserID     int64   `json:"user_id"`
	Email      string  `json:"email"`
	Username   string  `json:"username"`
	Requests   int64   `json:"requests"`
	Tokens     int64   `json:"tokens"`
	Cost       float64 `json:"cost"`        // 标准计费
	ActualCost float64 `json:"actual_cost"` // 实际扣除
}

// UserSpendingRankingItem represents a user spending ranking row.
type UserSpendingRankingItem struct {
	UserID     int64   `json:"user_id"`
	Email      string  `json:"email"`
	ActualCost float64 `json:"actual_cost"` // 实际扣除
	Requests   int64   `json:"requests"`
	Tokens     int64   `json:"tokens"`
}

// UserSpendingRankingResponse represents ranking rows plus total spend for the time range.
type UserSpendingRankingResponse struct {
	Ranking         []UserSpendingRankingItem `json:"ranking"`
	TotalActualCost float64                   `json:"total_actual_cost"`
	TotalRequests   int64                     `json:"total_requests"`
	TotalTokens     int64                     `json:"total_tokens"`
}

// UserBreakdownItem represents per-user usage breakdown within a dimension (group, model, endpoint).
type UserBreakdownItem struct {
	UserID       int64   `json:"user_id"`
	Email        string  `json:"email"`
	Requests     int64   `json:"requests"`
	InputTokens  int64   `json:"input_tokens"`  // 输入 token 累计
	OutputTokens int64   `json:"output_tokens"` // 输出 token 累计
	CacheTokens  int64   `json:"cache_tokens"`  // 缓存创建 + 读取 token 累计
	TotalTokens  int64   `json:"total_tokens"`  // 输入+输出+缓存 token 累计
	Cost         float64 `json:"cost"`          // 标准计费
	ActualCost   float64 `json:"actual_cost"`   // 实际扣除
	AccountCost  float64 `json:"account_cost"`  // 账号成本
}

// UserBreakdownDimension specifies the dimension to filter for user breakdown.
type UserBreakdownDimension struct {
	GroupID      int64  // filter by group_id (>0 to enable)
	Model        string // filter by model name (non-empty to enable)
	ModelType    string // "requested", "upstream", or "mapping"
	Endpoint     string // filter by endpoint value (non-empty to enable)
	EndpointType string // "inbound", "upstream", or "path"
	// Additional filter conditions
	UserID      int64  // filter by user_id (>0 to enable)
	APIKeyID    int64  // filter by api_key_id (>0 to enable)
	AccountID   int64  // filter by account_id (>0 to enable)
	RequestType *int16 // filter by request_type (non-nil to enable)
	Stream      *bool  // filter by stream flag (non-nil to enable)
	BillingType *int8  // filter by billing_type (non-nil to enable)
	// SortBy 指定排序列(空 = 默认按 actual_cost)。合法值由 repo 层 allowlist 校验。
	SortBy string
}

// APIKeyUsageTrendPoint represents API key usage trend data point
type APIKeyUsageTrendPoint struct {
	Date     string `json:"date"`
	APIKeyID int64  `json:"api_key_id"`
	KeyName  string `json:"key_name"`
	Requests int64  `json:"requests"`
	Tokens   int64  `json:"tokens"`
}

// APIKeyDailyUsagePoint represents one day of usage for a single API key.
type APIKeyDailyUsagePoint struct {
	Date             string  `json:"date"`
	Requests         int64   `json:"requests"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Cost             float64 `json:"cost"`        // 标准计费
	ActualCost       float64 `json:"actual_cost"` // 实际扣除
}

// UserDashboardStats 用户仪表盘统计
type UserDashboardStats struct {
	// API Key 统计
	TotalAPIKeys  int64 `json:"total_api_keys"`
	ActiveAPIKeys int64 `json:"active_api_keys"`

	// 累计 Token 使用统计
	TotalRequests            int64   `json:"total_requests"`
	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64   `json:"total_cache_read_tokens"`
	TotalTokens              int64   `json:"total_tokens"`
	TotalCost                float64 `json:"total_cost"`        // 累计标准计费
	TotalActualCost          float64 `json:"total_actual_cost"` // 累计实际扣除

	// 今日 Token 使用统计
	TodayRequests            int64   `json:"today_requests"`
	TodayInputTokens         int64   `json:"today_input_tokens"`
	TodayOutputTokens        int64   `json:"today_output_tokens"`
	TodayCacheCreationTokens int64   `json:"today_cache_creation_tokens"`
	TodayCacheReadTokens     int64   `json:"today_cache_read_tokens"`
	TodayTokens              int64   `json:"today_tokens"`
	TodayCost                float64 `json:"today_cost"`        // 今日标准计费
	TodayActualCost          float64 `json:"today_actual_cost"` // 今日实际扣除

	// 性能统计
	AverageDurationMs float64 `json:"average_duration_ms"`

	// 性能指标
	Rpm int64 `json:"rpm"` // 近5分钟平均每分钟请求数
	Tpm int64 `json:"tpm"` // 近5分钟平均每分钟Token数

	// 按"有效平台"维度拆分（与 ops 路径口径一致：group.platform 优先，否则 account.platform）
	ByPlatform []PlatformDashboardStats `json:"by_platform,omitempty"`
}

// PlatformDashboardStats 单个平台的用量明细。
type PlatformDashboardStats struct {
	Platform        string  `json:"platform"`
	TotalRequests   int64   `json:"total_requests"`
	TotalTokens     int64   `json:"total_tokens"`
	TotalActualCost float64 `json:"total_actual_cost"`
	TodayRequests   int64   `json:"today_requests"`
	TodayTokens     int64   `json:"today_tokens"`
	TodayActualCost float64 `json:"today_actual_cost"`
}

// UsageLogFilters represents filters for usage log queries
type UsageLogFilters struct {
	UserID    int64
	APIKeyID  int64
	AccountID int64
	GroupID   int64
	RequestID string
	Model     string
	// ModelFilterSource controls how Model is matched. Empty preserves raw usage_logs.model semantics.
	ModelFilterSource string
	RequestType       *int16
	Stream            *bool
	BillingType       *int8
	BillingMode       string
	StartTime         *time.Time
	EndTime           *time.Time
	// ExactTotal requests exact COUNT(*) for pagination. Default false for fast large-table paging.
	ExactTotal bool
}

// UsageStats represents usage statistics
type UsageStats struct {
	TotalRequests            int64          `json:"total_requests"`
	TotalInputTokens         int64          `json:"total_input_tokens"`
	TotalOutputTokens        int64          `json:"total_output_tokens"`
	TotalCacheTokens         int64          `json:"total_cache_tokens"`
	TotalCacheCreationTokens int64          `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64          `json:"total_cache_read_tokens"`
	TotalTokens              int64          `json:"total_tokens"`
	TotalCost                float64        `json:"total_cost"`
	TotalActualCost          float64        `json:"total_actual_cost"`
	TotalAccountCost         *float64       `json:"total_account_cost,omitempty"`
	AverageDurationMs        float64        `json:"average_duration_ms"`
	Endpoints                []EndpointStat `json:"endpoints,omitempty"`
	UpstreamEndpoints        []EndpointStat `json:"upstream_endpoints,omitempty"`
	EndpointPaths            []EndpointStat `json:"endpoint_paths,omitempty"`
}

// PlatformUsage 表示某用户/某 API key 在单个"有效平台"维度的用量明细。
// Platform 取值与 ops 路径口径一致：优先 groups.platform，否则 accounts.platform。
type PlatformUsage struct {
	Platform        string  `json:"platform"`
	TodayActualCost float64 `json:"today_actual_cost"`
	TotalActualCost float64 `json:"total_actual_cost"`
}

// BatchUserUsageStats represents usage stats for a single user
type BatchUserUsageStats struct {
	UserID          int64           `json:"user_id"`
	TodayActualCost float64         `json:"today_actual_cost"`
	TotalActualCost float64         `json:"total_actual_cost"`
	ByPlatform      []PlatformUsage `json:"by_platform,omitempty"`
}

// BatchAPIKeyUsageStats represents usage stats for a single API key
type BatchAPIKeyUsageStats struct {
	APIKeyID        int64   `json:"api_key_id"`
	TodayActualCost float64 `json:"today_actual_cost"`
	TotalActualCost float64 `json:"total_actual_cost"`
}

// AccountUsageHistory represents daily usage history for an account
type AccountUsageHistory struct {
	Date       string  `json:"date"`
	Label      string  `json:"label"`
	Requests   int64   `json:"requests"`
	Tokens     int64   `json:"tokens"`
	Cost       float64 `json:"cost"`        // 标准计费（total_cost）
	ActualCost float64 `json:"actual_cost"` // 账号口径费用（total_cost * account_rate_multiplier）
	UserCost   float64 `json:"user_cost"`   // 用户口径费用（actual_cost，受分组倍率影响）
}

// AccountUsageSummary represents summary statistics for an account
type AccountUsageSummary struct {
	Days              int     `json:"days"`
	ActualDaysUsed    int     `json:"actual_days_used"`
	TotalCost         float64 `json:"total_cost"`      // 账号口径费用
	TotalUserCost     float64 `json:"total_user_cost"` // 用户口径费用
	TotalStandardCost float64 `json:"total_standard_cost"`
	TotalRequests     int64   `json:"total_requests"`
	TotalTokens       int64   `json:"total_tokens"`
	AvgDailyCost      float64 `json:"avg_daily_cost"` // 账号口径日均
	AvgDailyUserCost  float64 `json:"avg_daily_user_cost"`
	AvgDailyRequests  float64 `json:"avg_daily_requests"`
	AvgDailyTokens    float64 `json:"avg_daily_tokens"`
	AvgDurationMs     float64 `json:"avg_duration_ms"`
	Today             *struct {
		Date     string  `json:"date"`
		Cost     float64 `json:"cost"`
		UserCost float64 `json:"user_cost"`
		Requests int64   `json:"requests"`
		Tokens   int64   `json:"tokens"`
	} `json:"today"`
	HighestCostDay *struct {
		Date     string  `json:"date"`
		Label    string  `json:"label"`
		Cost     float64 `json:"cost"`
		UserCost float64 `json:"user_cost"`
		Requests int64   `json:"requests"`
	} `json:"highest_cost_day"`
	HighestRequestDay *struct {
		Date     string  `json:"date"`
		Label    string  `json:"label"`
		Requests int64   `json:"requests"`
		Cost     float64 `json:"cost"`
		UserCost float64 `json:"user_cost"`
	} `json:"highest_request_day"`
}

// AccountUsageStatsResponse represents the full usage statistics response for an account
type AccountUsageStatsResponse struct {
	History           []AccountUsageHistory          `json:"history"`
	Summary           AccountUsageSummary            `json:"summary"`
	Models            []ModelStat                    `json:"models"`
	Endpoints         []EndpointStat                 `json:"endpoints"`
	UpstreamEndpoints []EndpointStat                 `json:"upstream_endpoints"`
	TestUsage         *AccountTestUsageStatsResponse `json:"test_usage,omitempty"`
}

// AccountTestUsageRecord 是管理员账号测试的独立记录，不进入用户计费链路。
type AccountTestUsageRecord struct {
	AccountID           int64
	Platform            string
	Model               string
	TestMode            string
	Endpoint            string
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	DurationMs          int64
	Success             bool
	StatusCode          int
	ErrorMessage        string
	CreatedAt           time.Time
}

// AccountTestUsageHistory 是账号测试按日期聚合的统计。
type AccountTestUsageHistory struct {
	Date               string `json:"date"`
	Label              string `json:"label"`
	Requests           int64  `json:"requests"`
	SuccessfulRequests int64  `json:"successful_requests"`
	FailedRequests     int64  `json:"failed_requests"`
	InputTokens        int64  `json:"input_tokens"`
	OutputTokens       int64  `json:"output_tokens"`
	CacheTokens        int64  `json:"cache_tokens"`
	Tokens             int64  `json:"tokens"`
}

// AccountTestUsageSummary 是账号测试期间的汇总统计。
type AccountTestUsageSummary struct {
	Days               int                      `json:"days"`
	ActualDaysUsed     int                      `json:"actual_days_used"`
	TotalRequests      int64                    `json:"total_requests"`
	SuccessfulRequests int64                    `json:"successful_requests"`
	FailedRequests     int64                    `json:"failed_requests"`
	InputTokens        int64                    `json:"input_tokens"`
	OutputTokens       int64                    `json:"output_tokens"`
	CacheTokens        int64                    `json:"cache_tokens"`
	TotalTokens        int64                    `json:"total_tokens"`
	AvgDurationMs      float64                  `json:"avg_duration_ms"`
	Today              *AccountTestUsageHistory `json:"today"`
}

// AccountTestUsageStatsResponse 是账号统计弹窗使用的独立测试统计响应。
type AccountTestUsageStatsResponse struct {
	History []AccountTestUsageHistory    `json:"history"`
	Summary AccountTestUsageSummary      `json:"summary"`
	Records []AccountTestUsageRecordView `json:"records"`
}

// AccountTestUsageRecordView 是管理员账号测试记录的脱敏展示结构。
type AccountTestUsageRecordView struct {
	ID                  int64     `json:"id"`
	AccountID           int64     `json:"account_id"`
	AccountName         string    `json:"account_name"`
	Platform            string    `json:"platform"`
	Model               string    `json:"model"`
	TestMode            string    `json:"test_mode"`
	Endpoint            string    `json:"endpoint"`
	InputTokens         int64     `json:"input_tokens"`
	OutputTokens        int64     `json:"output_tokens"`
	CacheCreationTokens int64     `json:"cache_creation_tokens"`
	CacheReadTokens     int64     `json:"cache_read_tokens"`
	Tokens              int64     `json:"tokens"`
	DurationMs          int64     `json:"duration_ms"`
	Success             bool      `json:"success"`
	StatusCode          int       `json:"status_code"`
	ErrorMessage        string    `json:"error_message,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

// AccountTestUsageGlobalFilters 是全局账号测试列表和统计共用的筛选条件。
type AccountTestUsageGlobalFilters struct {
	StartTime *time.Time `json:"-"`
	EndTime   *time.Time `json:"-"`
	AccountID int64      `json:"account_id,omitempty"`
	Platform  string     `json:"platform,omitempty"`
	Model     string     `json:"model,omitempty"`
	TestMode  string     `json:"test_mode,omitempty"`
	Success   *bool      `json:"success,omitempty"`
}

// AccountTestUsagePlatformStats 是管理员账号测试的单个平台聚合结果。
type AccountTestUsagePlatformStats struct {
	Platform           string  `json:"platform"`
	TotalRequests      int64   `json:"total_requests"`
	SuccessfulRequests int64   `json:"successful_requests"`
	FailedRequests     int64   `json:"failed_requests"`
	InputTokens        int64   `json:"input_tokens"`
	OutputTokens       int64   `json:"output_tokens"`
	CacheTokens        int64   `json:"cache_tokens"`
	TotalTokens        int64   `json:"total_tokens"`
	AvgDurationMs      float64 `json:"avg_duration_ms"`
}

// AccountTestUsageModelStats 是管理员账号测试的单个模型聚合结果。
type AccountTestUsageModelStats struct {
	Model         string `json:"model"`
	TotalRequests int64  `json:"total_requests"`
}

// AccountTestUsageGlobalStats 是独立于正式账单的全局账号测试统计。
type AccountTestUsageGlobalStats struct {
	TotalRequests      int64                           `json:"total_requests"`
	SuccessfulRequests int64                           `json:"successful_requests"`
	FailedRequests     int64                           `json:"failed_requests"`
	InputTokens        int64                           `json:"input_tokens"`
	OutputTokens       int64                           `json:"output_tokens"`
	CacheTokens        int64                           `json:"cache_tokens"`
	TotalTokens        int64                           `json:"total_tokens"`
	AvgDurationMs      float64                         `json:"avg_duration_ms"`
	ByPlatform         []AccountTestUsagePlatformStats `json:"by_platform"`
	ByModel            []AccountTestUsageModelStats    `json:"by_model"`
}
