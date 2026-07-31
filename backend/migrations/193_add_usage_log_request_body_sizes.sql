-- 193: 为 usage_logs 增加请求体大小快照字段。
-- 193: Add request body size snapshot columns to usage_logs.

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS request_body_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS max_request_body_bytes BIGINT;

COMMENT ON COLUMN usage_logs.request_body_bytes IS
    '请求体实际字节数；Request body size in bytes; NULL means unknown or not collected.';

COMMENT ON COLUMN usage_logs.max_request_body_bytes IS
    '请求实际生效的最大请求体字节数；Effective maximum request body size in bytes; NULL means unknown.';
