-- 192: 将所有账号定时测试统一调整为每 60 分钟执行一次。
-- 同步更新数据库默认值和新账号触发器，保证历史账号与新账号使用同一默认周期。

ALTER TABLE scheduled_test_plans
    ALTER COLUMN cron_expression SET DEFAULT '0 * * * *';

UPDATE scheduled_test_plans
SET cron_expression = '0 * * * *',
    next_run_at = CASE
        WHEN enabled THEN date_trunc('hour', NOW()) + INTERVAL '1 hour'
        ELSE next_run_at
    END,
    updated_at = NOW();

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
        '0 * * * *',
        true,
        50,
        true,
        date_trunc('hour', NOW()) + INTERVAL '1 hour',
        NOW(),
        NOW()
    );

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
