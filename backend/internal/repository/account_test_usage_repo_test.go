package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

const (
	accountTestUsageHistoryQuery         = "(?s)SELECT.*FROM account_test_usage_logs\\s+WHERE account_id = \\$1 AND created_at >= \\$2 AND created_at < \\$3\\s+GROUP BY 1\\s+ORDER BY 1"
	accountTestUsageAverageQuery         = "(?s)SELECT COALESCE\\(AVG\\(duration_ms\\), 0\\).*FROM account_test_usage_logs\\s+WHERE account_id = \\$1 AND created_at >= \\$2 AND created_at < \\$3"
	accountTestUsageRecordsQuery         = "(?s)SELECT id, model, test_mode, endpoint, input_tokens, output_tokens,.*FROM account_test_usage_logs\\s+WHERE account_id = \\$1 AND created_at >= \\$2 AND created_at < \\$3\\s+ORDER BY created_at DESC, id DESC\\s+LIMIT \\$4"
	accountTestUsageInsertQuery          = "(?s)INSERT INTO account_test_usage_logs \\(.*account_id, platform, model, test_mode, endpoint,.*duration_ms, success, status_code, error_message, created_at.*\\) VALUES \\(\\$1, \\$2, \\$3, \\$4, \\$5, \\$6, \\$7, \\$8, \\$9, \\$10, \\$11, \\$12, \\$13, \\$14\\)"
	accountTestUsageGlobalCountQuery     = "(?s)SELECT COUNT\\(\\*\\) FROM account_test_usage_logs l JOIN accounts a ON a.id = l.account_id.*"
	accountTestUsageGlobalListQuery      = "(?s)SELECT l.id, l.account_id, a.name, l.platform, l.model, l.test_mode, l.endpoint.*FROM account_test_usage_logs l.*JOIN accounts a ON a.id = l.account_id.*ORDER BY l.created_at DESC, l.id DESC.*LIMIT \\$8 OFFSET \\$9"
	accountTestUsageGlobalStatsQuery     = "(?s)SELECT COUNT\\(\\*\\), COUNT\\(\\*\\) FILTER.*FROM account_test_usage_logs l.*JOIN accounts a ON a.id = l.account_id.*"
	accountTestUsageGlobalPlatformsQuery = "(?s)SELECT l.platform, COUNT\\(\\*\\).*FROM account_test_usage_logs l.*JOIN accounts a ON a.id = l.account_id.*GROUP BY l.platform ORDER BY l.platform"
	accountTestUsageGlobalModelsQuery    = "(?s)SELECT l.model, COUNT\\(\\*\\).*FROM account_test_usage_logs l.*JOIN accounts a ON a.id = l.account_id.*l.model <> ''.*GROUP BY l.model ORDER BY l.model"
)

// TestAccountTestUsageRepositoryCreatePersistsAllFields 验证 INSERT 字段顺序及边界值不会错位。
func TestAccountTestUsageRepositoryCreatePersistsAllFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	createdAt := time.Date(2026, 7, 31, 23, 59, 58, 123456789, time.UTC)
	record := &usagestats.AccountTestUsageRecord{
		AccountID: 42, Platform: "grok", Model: "grok-4", TestMode: "stream", Endpoint: "/v1/chat/completions",
		InputTokens: 0, OutputTokens: 1, CacheCreationTokens: 2, CacheReadTokens: 3,
		DurationMs: 0, Success: false, StatusCode: 0, ErrorMessage: "upstream timeout", CreatedAt: createdAt,
	}
	mock.ExpectExec(accountTestUsageInsertQuery).WithArgs(
		record.AccountID, record.Platform, record.Model, record.TestMode, record.Endpoint,
		record.InputTokens, record.OutputTokens, record.CacheCreationTokens, record.CacheReadTokens,
		record.DurationMs, record.Success, record.StatusCode, record.ErrorMessage, record.CreatedAt,
	).WillReturnResult(sqlmock.NewResult(1, 1))

	repo := &accountTestUsageRepository{sql: db}
	require.NoError(t, repo.Create(context.Background(), record))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAccountTestUsageRepositoryCreateRejectsInvalidRecords 验证无效记录在触碰数据库前被拒绝。
func TestAccountTestUsageRepositoryCreateRejectsInvalidRecords(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &accountTestUsageRepository{sql: db}
	require.Error(t, repo.Create(context.Background(), nil))
	require.Error(t, repo.Create(context.Background(), &usagestats.AccountTestUsageRecord{AccountID: 0}))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAccountTestUsageRepositoryGetStatsAggregatesHistorySummaryAndRecords 验证统计聚合、汇总和最近明细均使用独立测试表。
func TestAccountTestUsageRepositoryGetStatsAggregatesHistorySummaryAndRecords(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	accountID := int64(42)
	startTime := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 7, 31, 12, 34, 56, 0, time.UTC)
	historyRows := sqlmock.NewRows([]string{"date", "requests", "successful_requests", "failed_requests", "input_tokens", "output_tokens", "cache_tokens"}).
		AddRow("2026-07-29", int64(2), int64(1), int64(1), int64(10), int64(20), int64(3)).
		AddRow("2026-07-31", int64(1), int64(1), int64(0), int64(4), int64(5), int64(6))
	mock.ExpectQuery(accountTestUsageHistoryQuery).WithArgs(accountID, startTime, endTime).WillReturnRows(historyRows)
	mock.ExpectQuery(accountTestUsageAverageQuery).WithArgs(accountID, startTime, endTime).
		WillReturnRows(sqlmock.NewRows([]string{"avg"}).AddRow(275.5))
	recordRows := sqlmock.NewRows([]string{"id", "model", "test_mode", "endpoint", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens", "duration_ms", "success", "status_code", "error_message", "created_at"})
	recordRows.AddRow(int64(7), "grok-4", "stream", "/v1/chat/completions", 4, 5, 2, 4, int64(300), true, 200, "", createdAt)
	mock.ExpectQuery(accountTestUsageRecordsQuery).WithArgs(accountID, startTime, endTime, 50).WillReturnRows(recordRows)

	result, err := (&accountTestUsageRepository{sql: db}).GetStats(context.Background(), accountID, startTime, endTime)
	require.NoError(t, err)
	require.Len(t, result.History, 2)
	require.Equal(t, "07/29", result.History[0].Label)
	require.Equal(t, int64(33), result.History[0].Tokens)
	require.Equal(t, "07/31", result.History[1].Label)
	require.Equal(t, int64(15), result.History[1].Tokens)
	require.Equal(t, usagestats.AccountTestUsageSummary{Days: 3, ActualDaysUsed: 2, TotalRequests: 3, SuccessfulRequests: 2, FailedRequests: 1, InputTokens: 14, OutputTokens: 25, CacheTokens: 9, TotalTokens: 48, AvgDurationMs: 275.5}, result.Summary)
	require.Len(t, result.Records, 1)
	require.Equal(t, int64(7), result.Records[0].ID)
	require.Equal(t, int64(300), result.Records[0].DurationMs)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAccountTestUsageRepositoryGetStatsReturnsEmptyResult 验证无记录时仍返回稳定的空数组和零汇总。
func TestAccountTestUsageRepositoryGetStatsReturnsEmptyResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	accountID := int64(42)
	startTime := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(accountTestUsageHistoryQuery).WithArgs(accountID, startTime, endTime).
		WillReturnRows(sqlmock.NewRows([]string{"date", "requests", "successful_requests", "failed_requests", "input_tokens", "output_tokens", "cache_tokens"}))
	mock.ExpectQuery(accountTestUsageAverageQuery).WithArgs(accountID, startTime, endTime).
		WillReturnRows(sqlmock.NewRows([]string{"avg"}).AddRow(0.0))
	mock.ExpectQuery(accountTestUsageRecordsQuery).WithArgs(accountID, startTime, endTime, 50).
		WillReturnRows(sqlmock.NewRows([]string{"id", "model", "test_mode", "endpoint", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens", "duration_ms", "success", "status_code", "error_message", "created_at"}))

	result, err := (&accountTestUsageRepository{sql: db}).GetStats(context.Background(), accountID, startTime, endTime)
	require.NoError(t, err)
	require.Empty(t, result.History)
	require.Empty(t, result.Records)
	require.Equal(t, 3, result.Summary.Days)
	require.Equal(t, 1, result.Summary.ActualDaysUsed)
	require.Zero(t, result.Summary.TotalRequests)
	require.Zero(t, result.Summary.TotalTokens)
	require.Zero(t, result.Summary.AvgDurationMs)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAccountTestUsageRepositoryGetStatsReturnsHistoryQueryError 验证按日聚合查询错误会原样返回。
func TestAccountTestUsageRepositoryGetStatsReturnsHistoryQueryError(t *testing.T) {
	testAccountTestUsageStatsDatabaseError(t, func(mock sqlmock.Sqlmock, accountID int64, startTime, endTime time.Time) {
		mock.ExpectQuery(accountTestUsageHistoryQuery).WithArgs(accountID, startTime, endTime).WillReturnError(errors.New("history query failed"))
	})
}

// TestAccountTestUsageRepositoryGetStatsReturnsAverageQueryError 验证平均时长查询错误会原样返回。
func TestAccountTestUsageRepositoryGetStatsReturnsAverageQueryError(t *testing.T) {
	testAccountTestUsageStatsDatabaseError(t, func(mock sqlmock.Sqlmock, accountID int64, startTime, endTime time.Time) {
		mock.ExpectQuery(accountTestUsageHistoryQuery).WithArgs(accountID, startTime, endTime).
			WillReturnRows(sqlmock.NewRows([]string{"date", "requests", "successful_requests", "failed_requests", "input_tokens", "output_tokens", "cache_tokens"}))
		mock.ExpectQuery(accountTestUsageAverageQuery).WithArgs(accountID, startTime, endTime).WillReturnError(errors.New("average query failed"))
	})
}

// TestAccountTestUsageRepositoryGetStatsReturnsRecordsQueryError 验证最近明细查询错误会原样返回。
func TestAccountTestUsageRepositoryGetStatsReturnsRecordsQueryError(t *testing.T) {
	testAccountTestUsageStatsDatabaseError(t, func(mock sqlmock.Sqlmock, accountID int64, startTime, endTime time.Time) {
		mock.ExpectQuery(accountTestUsageHistoryQuery).WithArgs(accountID, startTime, endTime).
			WillReturnRows(sqlmock.NewRows([]string{"date", "requests", "successful_requests", "failed_requests", "input_tokens", "output_tokens", "cache_tokens"}))
		mock.ExpectQuery(accountTestUsageAverageQuery).WithArgs(accountID, startTime, endTime).
			WillReturnRows(sqlmock.NewRows([]string{"avg"}).AddRow(0.0))
		mock.ExpectQuery(accountTestUsageRecordsQuery).WithArgs(accountID, startTime, endTime, 50).WillReturnError(errors.New("records query failed"))
	})
}

// TestAccountTestUsageRepositoryCreateReturnsDatabaseError 验证 INSERT 数据库错误会原样返回。
func TestAccountTestUsageRepositoryCreateReturnsDatabaseError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	record := &usagestats.AccountTestUsageRecord{AccountID: 42, CreatedAt: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)}
	mock.ExpectExec(accountTestUsageInsertQuery).WithArgs(record.AccountID, record.Platform, record.Model, record.TestMode, record.Endpoint, record.InputTokens, record.OutputTokens, record.CacheCreationTokens, record.CacheReadTokens, record.DurationMs, record.Success, record.StatusCode, record.ErrorMessage, record.CreatedAt).WillReturnError(errors.New("insert failed"))
	require.EqualError(t, (&accountTestUsageRepository{sql: db}).Create(context.Background(), record), "insert failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAccountTestUsageRepositoryListGlobalJoinsAccountsAndFilters 验证全局列表的关联、筛选、半开时间范围和排序。
func TestAccountTestUsageRepositoryListGlobalJoinsAccountsAndFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	end := time.Date(2026, 8, 4, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	success := false
	filters := usagestats.AccountTestUsageGlobalFilters{StartTime: &start, EndTime: &end, AccountID: 42, Platform: "deepseek", Model: "deepseek-chat", TestMode: "responses", Success: &success}
	args := []driver.Value{start, end, int64(42), "deepseek", "deepseek-chat", "responses", false}
	mock.ExpectQuery(accountTestUsageGlobalCountQuery).WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	rows := sqlmock.NewRows([]string{"id", "account_id", "name", "platform", "model", "test_mode", "endpoint", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens", "duration_ms", "success", "status_code", "error_message", "created_at"}).
		AddRow(int64(9), int64(42), "DeepSeek 主账号", "deepseek", "deepseek-chat", "responses", "/v1/responses", 12, 8, 1, 2, int64(345), false, 500, "上游错误", start.Add(time.Hour))
	mock.ExpectQuery(accountTestUsageGlobalListQuery).WithArgs(append(args, 20, 0)...).WillReturnRows(rows)
	items, result, err := (&accountTestUsageRepository{sql: db}).ListGlobal(context.Background(), pagination.PaginationParams{Page: 1, PageSize: 20}, filters)
	require.NoError(t, err)
	require.Equal(t, int64(1), result.Total)
	require.Len(t, items, 1)
	require.Equal(t, "DeepSeek 主账号", items[0].AccountName)
	require.Equal(t, int64(42), items[0].AccountID)
	require.Equal(t, int64(23), items[0].Tokens)
	require.False(t, items[0].Success)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestBuildAccountTestUsageGlobalWhereUsesPostgresPlaceholders 验证日期和筛选条件不会丢失 PostgreSQL 参数占位符前缀。
func TestBuildAccountTestUsageGlobalWhereUsesPostgresPlaceholders(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	success := true
	whereSQL, args := buildAccountTestUsageGlobalWhere(usagestats.AccountTestUsageGlobalFilters{
		StartTime: &start,
		EndTime:   &end,
		AccountID: 42,
		Platform:  "deepseek",
		Success:   &success,
	})

	require.Equal(t, " AND l.created_at >= $1 AND l.created_at < $2 AND l.account_id = $3 AND l.platform = $4 AND l.success = $5", whereSQL)
	require.Equal(t, []any{start, end, int64(42), "deepseek", true}, args)
}

// TestAccountTestUsageRepositoryGetGlobalStatsAggregatesTotalsPlatformsAndModels 验证所有统计均来自独立测试表。
func TestAccountTestUsageRepositoryGetGlobalStatsAggregatesTotalsPlatformsAndModels(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	filters := usagestats.AccountTestUsageGlobalFilters{StartTime: &start, EndTime: &end}
	mock.ExpectQuery(accountTestUsageGlobalStatsQuery).WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"requests", "success", "failed", "input", "output", "cache", "avg"}).AddRow(int64(3), int64(2), int64(1), int64(30), int64(20), int64(5), 125.5))
	mock.ExpectQuery(accountTestUsageGlobalPlatformsQuery).WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"platform", "requests", "success", "failed", "input", "output", "cache", "avg"}).
			AddRow("deepseek", int64(2), int64(2), int64(0), int64(20), int64(10), int64(2), 100.0).
			AddRow("grok", int64(1), int64(0), int64(1), int64(10), int64(10), int64(3), 176.5))
	mock.ExpectQuery(accountTestUsageGlobalModelsQuery).WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"model", "requests"}).
			AddRow("deepseek-chat", int64(2)).
			AddRow("grok-4", int64(1)))
	stats, err := (&accountTestUsageRepository{sql: db}).GetGlobalStats(context.Background(), filters)
	require.NoError(t, err)
	require.Equal(t, int64(3), stats.TotalRequests)
	require.Equal(t, int64(55), stats.TotalTokens)
	require.Equal(t, 125.5, stats.AvgDurationMs)
	require.Len(t, stats.ByPlatform, 2)
	require.Equal(t, int64(32), stats.ByPlatform[0].TotalTokens)
	require.Equal(t, int64(23), stats.ByPlatform[1].TotalTokens)
	require.Equal(t, []usagestats.AccountTestUsageModelStats{{Model: "deepseek-chat", TotalRequests: 2}, {Model: "grok-4", TotalRequests: 1}}, stats.ByModel)
	require.NoError(t, mock.ExpectationsWereMet())
}

// testAccountTestUsageStatsDatabaseError 统一执行统计查询错误场景并检查账号和时间范围参数。
func testAccountTestUsageStatsDatabaseError(t *testing.T, setup func(sqlmock.Sqlmock, int64, time.Time, time.Time)) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	accountID := int64(42)
	startTime := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	setup(mock, accountID, startTime, endTime)
	_, err = (&accountTestUsageRepository{sql: db}).GetStats(context.Background(), accountID, startTime, endTime)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
