package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRemoveScheduledTestPlansMigration 验证历史定时测试会被清除且新账号不再自动创建计划。
func TestRemoveScheduledTestPlansMigration(t *testing.T) {
	content, err := FS.ReadFile("195_remove_scheduled_test_plans.sql")
	require.NoError(t, err)

	// 清理迁移 SQL，压缩空白后稳定断言关键数据库契约。
	清理定时测试迁移SQL := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, 清理定时测试迁移SQL, "DROP TRIGGER IF EXISTS create_default_scheduled_test_plan_after_account_insert ON accounts")
	require.Contains(t, 清理定时测试迁移SQL, "DROP FUNCTION IF EXISTS create_default_scheduled_test_plan_for_account()")
	require.Contains(t, 清理定时测试迁移SQL, "DELETE FROM scheduled_test_plans")
}
