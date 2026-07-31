package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestScheduledTestIntervalMigration 验证历史计划、数据库默认值和新账号触发器都统一为 60 分钟。
func TestScheduledTestIntervalMigration(t *testing.T) {
	content, err := FS.ReadFile("192_update_scheduled_test_interval_to_60_minutes.sql")
	require.NoError(t, err)

	// 迁移 SQL 压缩空白后断言关键契约，避免换行格式影响测试稳定性。
	定时测试周期迁移SQL := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, 定时测试周期迁移SQL, "ALTER COLUMN cron_expression SET DEFAULT '0 * * * *'")
	require.Contains(t, 定时测试周期迁移SQL, "SET cron_expression = '0 * * * *'")
	require.Contains(t, 定时测试周期迁移SQL, "date_trunc('hour', NOW()) + INTERVAL '1 hour'")
	require.Contains(t, 定时测试周期迁移SQL, "CREATE OR REPLACE FUNCTION create_default_scheduled_test_plan_for_account()")
	require.NotContains(t, 定时测试周期迁移SQL, "*/30 * * * *")
}
