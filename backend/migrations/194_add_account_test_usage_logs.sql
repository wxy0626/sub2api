-- 194: 管理员账号测试独立用量记录，不参与正式用户计费。

CREATE TABLE IF NOT EXISTS account_test_usage_logs (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    platform VARCHAR(32) NOT NULL DEFAULT '',
    model VARCHAR(255) NOT NULL DEFAULT '',
    test_mode VARCHAR(32) NOT NULL DEFAULT 'default',
    endpoint VARCHAR(255) NOT NULL DEFAULT '',
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    success BOOLEAN NOT NULL DEFAULT FALSE,
    status_code INTEGER NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_account_test_usage_logs_account_created
    ON account_test_usage_logs (account_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_account_test_usage_logs_account_model
    ON account_test_usage_logs (account_id, model, created_at DESC);
