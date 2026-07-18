-- 183: 新建账号时在同一事务内初始化默认定时测试计划。
-- 不回填既有账号，避免覆盖管理员已经配置的计划。

CREATE OR REPLACE FUNCTION create_default_scheduled_test_plan_for_account()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO scheduled_test_plans (
        account_id,
        model_id,
        cron_expression,
        enabled,
        max_results,
        auto_recover,
        next_run_at,
        created_at,
        updated_at
    )
    VALUES (
        NEW.id,
        'gpt-5.6-luna',
        '*/30 * * * *',
        true,
        50,
        true,
        date_trunc('hour', NOW()) +
            ((FLOOR(EXTRACT(MINUTE FROM NOW()) / 30)::int + 1) * INTERVAL '30 minutes'),
        NOW(),
        NOW()
    );

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS create_default_scheduled_test_plan_after_account_insert ON accounts;

CREATE TRIGGER create_default_scheduled_test_plan_after_account_insert
AFTER INSERT ON accounts
FOR EACH ROW
EXECUTE FUNCTION create_default_scheduled_test_plan_for_account();
