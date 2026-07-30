package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDefaultScheduledTestPlanMigration 验证新增账号会初始化指定的定时测试默认值。
func TestDefaultScheduledTestPlanMigration(t *testing.T) {
	content, err := FS.ReadFile("183_create_default_scheduled_test_plan.sql")
	require.NoError(t, err)

	// 默认计划迁移 SQL，压缩空白后便于稳定断言。
	默认计划迁移SQL := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, 默认计划迁移SQL, "CREATE TRIGGER create_default_scheduled_test_plan_after_account_insert")
	require.Contains(t, 默认计划迁移SQL, "AFTER INSERT ON accounts")
	require.Contains(t, 默认计划迁移SQL, "'gpt-5.6-luna'")
	require.Contains(t, 默认计划迁移SQL, "'*/30 * * * *'")
	require.Contains(t, 默认计划迁移SQL, "true, 50, true")
	require.Contains(t, 默认计划迁移SQL, "EXTRACT(MINUTE FROM NOW()) / 30")
}
