package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type accountTestUsageRepository struct {
	sql sqlExecutor
}

// NewAccountTestUsageRepository 创建管理员账号测试用量仓储。
func NewAccountTestUsageRepository(sqlDB *sql.DB) service.AccountTestUsageRepository {
	return &accountTestUsageRepository{sql: sqlDB}
}

// Create 保存一次账号测试结果；该表不含 user_id/api_key_id，因此不会进入用户计费。
func (r *accountTestUsageRepository) Create(ctx context.Context, record *usagestats.AccountTestUsageRecord) error {
	if r == nil || r.sql == nil {
		return fmt.Errorf("管理员账号测试记录数据库不可用，请检查数据库连接和 account_test_usage_logs 表")
	}
	if record == nil || record.AccountID <= 0 {
		return fmt.Errorf("管理员账号测试记录无效：account_id 必须大于 0")
	}
	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO account_test_usage_logs (
			account_id, platform, model, test_mode, endpoint,
			input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
			duration_ms, success, status_code, error_message, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, record.AccountID, record.Platform, record.Model, record.TestMode, record.Endpoint,
		record.InputTokens, record.OutputTokens, record.CacheCreationTokens, record.CacheReadTokens,
		record.DurationMs, record.Success, record.StatusCode, record.ErrorMessage, record.CreatedAt)
	return err
}

// GetStats 按日期聚合账号测试用量，失败次数也保留用于诊断。
func (r *accountTestUsageRepository) GetStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountTestUsageStatsResponse, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("管理员账号测试记录数据库不可用，请检查数据库连接和 account_test_usage_logs 表")
	}
	days := int(endTime.Sub(startTime).Hours() / 24)
	if days <= 0 {
		days = 1
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT
			TO_CHAR(created_at, 'YYYY-MM-DD'),
			COUNT(*),
			COUNT(*) FILTER (WHERE success),
			COUNT(*) FILTER (WHERE NOT success),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cache_creation_tokens + cache_read_tokens), 0)
		FROM account_test_usage_logs
		WHERE account_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY 1
		ORDER BY 1
	`, accountID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := make([]usagestats.AccountTestUsageHistory, 0)
	for rows.Next() {
		var item usagestats.AccountTestUsageHistory
		if err := rows.Scan(&item.Date, &item.Requests, &item.SuccessfulRequests, &item.FailedRequests, &item.InputTokens, &item.OutputTokens, &item.CacheTokens); err != nil {
			return nil, err
		}
		item.Label = item.Date
		if parsed, parseErr := time.Parse("2006-01-02", item.Date); parseErr == nil {
			item.Label = parsed.Format("01/02")
		}
		item.Tokens = item.InputTokens + item.OutputTokens + item.CacheTokens
		history = append(history, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var avgDuration float64
	if err := scanSingleRow(ctx, r.sql, `
		SELECT COALESCE(AVG(duration_ms), 0)
		FROM account_test_usage_logs
		WHERE account_id = $1 AND created_at >= $2 AND created_at < $3
	`, []any{accountID, startTime, endTime}, &avgDuration); err != nil {
		return nil, err
	}

	result := &usagestats.AccountTestUsageStatsResponse{
		History: history,
		Summary: usagestats.AccountTestUsageSummary{
			Days:           days,
			ActualDaysUsed: len(history),
			AvgDurationMs:  avgDuration,
		},
	}
	records, err := r.getRecentRecords(ctx, accountID, startTime, endTime, 50)
	if err != nil {
		return nil, err
	}
	result.Records = records
	for i := range history {
		item := &history[i]
		result.Summary.TotalRequests += item.Requests
		result.Summary.SuccessfulRequests += item.SuccessfulRequests
		result.Summary.FailedRequests += item.FailedRequests
		result.Summary.InputTokens += item.InputTokens
		result.Summary.OutputTokens += item.OutputTokens
		result.Summary.CacheTokens += item.CacheTokens
		result.Summary.TotalTokens += item.Tokens
	}
	if result.Summary.ActualDaysUsed == 0 {
		result.Summary.ActualDaysUsed = 1
	}
	today := timezone.Now().Format("2006-01-02")
	for i := range history {
		if history[i].Date == today {
			result.Summary.Today = &history[i]
			break
		}
	}
	return result, nil
}

// getRecentRecords 获取统计时间范围内最近的账号测试明细，供管理员排查测试结果。
func (r *accountTestUsageRepository) getRecentRecords(ctx context.Context, accountID int64, startTime, endTime time.Time, limit int) ([]usagestats.AccountTestUsageRecordView, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, model, test_mode, endpoint, input_tokens, output_tokens,
			cache_creation_tokens, cache_read_tokens, duration_ms, success,
			status_code, error_message, created_at
		FROM account_test_usage_logs
		WHERE account_id = $1 AND created_at >= $2 AND created_at < $3
		ORDER BY created_at DESC, id DESC
		LIMIT $4
	`, accountID, startTime, endTime, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]usagestats.AccountTestUsageRecordView, 0)
	for rows.Next() {
		var item usagestats.AccountTestUsageRecordView
		if err := rows.Scan(
			&item.ID, &item.Model, &item.TestMode, &item.Endpoint,
			&item.InputTokens, &item.OutputTokens, &item.CacheCreationTokens, &item.CacheReadTokens,
			&item.DurationMs, &item.Success, &item.StatusCode, &item.ErrorMessage, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.Tokens = item.InputTokens + item.OutputTokens + item.CacheCreationTokens + item.CacheReadTokens
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ListGlobal 查询全局账号测试记录，并通过 accounts 关联返回账号名称。
func (r *accountTestUsageRepository) ListGlobal(ctx context.Context, params pagination.PaginationParams, filters usagestats.AccountTestUsageGlobalFilters) ([]usagestats.AccountTestUsageRecordView, *pagination.PaginationResult, error) {
	if r == nil || r.sql == nil {
		return nil, nil, fmt.Errorf("管理员账号测试记录数据库不可用，请检查数据库连接和 account_test_usage_logs 表")
	}
	whereSQL, args := buildAccountTestUsageGlobalWhere(filters)
	var total int64
	if err := scanSingleRow(ctx, r.sql, `SELECT COUNT(*) FROM account_test_usage_logs l JOIN accounts a ON a.id = l.account_id`+whereSQL, args, &total); err != nil {
		return nil, nil, err
	}

	limit := params.Limit()
	queryArgs := append(append([]any{}, args...), limit, params.Offset())
	rows, err := r.sql.QueryContext(ctx, `
		SELECT l.id, l.account_id, a.name, l.platform, l.model, l.test_mode, l.endpoint,
			l.input_tokens, l.output_tokens, l.cache_creation_tokens, l.cache_read_tokens,
			l.duration_ms, l.success, l.status_code, l.error_message, l.created_at
		FROM account_test_usage_logs l
		JOIN accounts a ON a.id = l.account_id`+whereSQL+`
		ORDER BY l.created_at DESC, l.id DESC
		LIMIT $`+fmt.Sprint(len(args)+1)+` OFFSET $`+fmt.Sprint(len(args)+2), queryArgs...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	items := make([]usagestats.AccountTestUsageRecordView, 0, limit)
	for rows.Next() {
		var item usagestats.AccountTestUsageRecordView
		if err := rows.Scan(
			&item.ID, &item.AccountID, &item.AccountName, &item.Platform, &item.Model,
			&item.TestMode, &item.Endpoint, &item.InputTokens, &item.OutputTokens,
			&item.CacheCreationTokens, &item.CacheReadTokens, &item.DurationMs,
			&item.Success, &item.StatusCode, &item.ErrorMessage, &item.CreatedAt,
		); err != nil {
			return nil, nil, err
		}
		item.Tokens = item.InputTokens + item.OutputTokens + item.CacheCreationTokens + item.CacheReadTokens
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return items, paginationResultFromTotal(total, params), nil
}

// GetGlobalStats 聚合账号测试总量、平台和模型维度，所有字段均不带费用语义。
func (r *accountTestUsageRepository) GetGlobalStats(ctx context.Context, filters usagestats.AccountTestUsageGlobalFilters) (*usagestats.AccountTestUsageGlobalStats, error) {
	if r == nil || r.sql == nil {
		return nil, fmt.Errorf("管理员账号测试记录数据库不可用，请检查数据库连接和 account_test_usage_logs 表")
	}
	whereSQL, args := buildAccountTestUsageGlobalWhere(filters)
	var result usagestats.AccountTestUsageGlobalStats
	if err := scanSingleRow(ctx, r.sql, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE l.success), COUNT(*) FILTER (WHERE NOT l.success),
			COALESCE(SUM(l.input_tokens), 0), COALESCE(SUM(l.output_tokens), 0),
			COALESCE(SUM(l.cache_creation_tokens + l.cache_read_tokens), 0), COALESCE(AVG(l.duration_ms), 0)
		FROM account_test_usage_logs l
		JOIN accounts a ON a.id = l.account_id`+whereSQL, args, &result.TotalRequests,
		&result.SuccessfulRequests, &result.FailedRequests, &result.InputTokens,
		&result.OutputTokens, &result.CacheTokens, &result.AvgDurationMs); err != nil {
		return nil, err
	}
	result.TotalTokens = result.InputTokens + result.OutputTokens + result.CacheTokens

	platformRows, err := r.sql.QueryContext(ctx, `
		SELECT l.platform, COUNT(*), COUNT(*) FILTER (WHERE l.success), COUNT(*) FILTER (WHERE NOT l.success),
			COALESCE(SUM(l.input_tokens), 0), COALESCE(SUM(l.output_tokens), 0),
			COALESCE(SUM(l.cache_creation_tokens + l.cache_read_tokens), 0), COALESCE(AVG(l.duration_ms), 0)
		FROM account_test_usage_logs l
		JOIN accounts a ON a.id = l.account_id`+whereSQL+`
		GROUP BY l.platform ORDER BY l.platform`, args...)
	if err != nil {
		return nil, err
	}
	defer platformRows.Close()
	result.ByPlatform = make([]usagestats.AccountTestUsagePlatformStats, 0)
	for platformRows.Next() {
		var item usagestats.AccountTestUsagePlatformStats
		if err := platformRows.Scan(&item.Platform, &item.TotalRequests, &item.SuccessfulRequests, &item.FailedRequests,
			&item.InputTokens, &item.OutputTokens, &item.CacheTokens, &item.AvgDurationMs); err != nil {
			return nil, err
		}
		item.TotalTokens = item.InputTokens + item.OutputTokens + item.CacheTokens
		result.ByPlatform = append(result.ByPlatform, item)
	}
	if err := platformRows.Err(); err != nil {
		return nil, err
	}

	// 模型选项忽略当前模型筛选，确保选中模型后仍可切换到同范围内的其他已测试模型。
	modelFilters := filters
	modelFilters.Model = ""
	modelWhereSQL, modelArgs := buildAccountTestUsageGlobalWhere(modelFilters)
	modelRows, err := r.sql.QueryContext(ctx, `
		SELECT l.model, COUNT(*)
		FROM account_test_usage_logs l
		JOIN accounts a ON a.id = l.account_id`+modelWhereSQL+`
		 AND l.model <> ''
		GROUP BY l.model ORDER BY l.model`, modelArgs...)
	if err != nil {
		return nil, err
	}
	defer modelRows.Close()
	result.ByModel = make([]usagestats.AccountTestUsageModelStats, 0)
	for modelRows.Next() {
		var item usagestats.AccountTestUsageModelStats
		if err := modelRows.Scan(&item.Model, &item.TotalRequests); err != nil {
			return nil, err
		}
		result.ByModel = append(result.ByModel, item)
	}
	if err := modelRows.Err(); err != nil {
		return nil, err
	}
	return &result, nil
}

// buildAccountTestUsageGlobalWhere 构造参数化的半开时间范围和可选筛选条件。
func buildAccountTestUsageGlobalWhere(filters usagestats.AccountTestUsageGlobalFilters) (string, []any) {
	var builder strings.Builder
	args := make([]any, 0, 7)
	add := func(condition string, value any) {
		args = append(args, value)
		builder.WriteString(" AND ")
		builder.WriteString(strings.Replace(condition, "$N", fmt.Sprintf("$%d", len(args)), 1))
	}
	if filters.StartTime != nil {
		add("l.created_at >= $N", *filters.StartTime)
	}
	if filters.EndTime != nil {
		add("l.created_at < $N", *filters.EndTime)
	}
	if filters.AccountID > 0 {
		add("l.account_id = $N", filters.AccountID)
	}
	if value := strings.TrimSpace(filters.Platform); value != "" {
		add("l.platform = $N", value)
	}
	if value := strings.TrimSpace(filters.Model); value != "" {
		add("l.model = $N", value)
	}
	if value := strings.TrimSpace(filters.TestMode); value != "" {
		add("l.test_mode = $N", value)
	}
	if filters.Success != nil {
		add("l.success = $N", *filters.Success)
	}
	return builder.String(), args
}
