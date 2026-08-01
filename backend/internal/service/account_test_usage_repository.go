package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// AccountTestUsageRepository 保存管理员账号测试的独立用量，不参与正式账单。
type AccountTestUsageRepository interface {
	Create(ctx context.Context, record *usagestats.AccountTestUsageRecord) error
	GetStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountTestUsageStatsResponse, error)
	// ListGlobal 查询全局管理员账号测试记录，不进入正式 usage_logs 账单链路。
	ListGlobal(ctx context.Context, params pagination.PaginationParams, filters usagestats.AccountTestUsageGlobalFilters) ([]usagestats.AccountTestUsageRecordView, *pagination.PaginationResult, error)
	// GetGlobalStats 聚合全局管理员账号测试统计，返回平台维度诊断数据。
	GetGlobalStats(ctx context.Context, filters usagestats.AccountTestUsageGlobalFilters) (*usagestats.AccountTestUsageGlobalStats, error)
}
