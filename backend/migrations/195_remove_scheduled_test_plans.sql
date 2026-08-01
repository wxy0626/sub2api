-- 195: 关闭并清除所有账号的定时测试，同时移除新账号的默认定时测试计划。
-- 删除计划会通过 scheduled_test_results.plan_id 的 ON DELETE CASCADE 一并清理历史结果。

DROP TRIGGER IF EXISTS create_default_scheduled_test_plan_after_account_insert ON accounts;
DROP FUNCTION IF EXISTS create_default_scheduled_test_plan_for_account();

DELETE FROM scheduled_test_plans;
