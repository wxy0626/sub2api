-- 本地新加坡专用代理幂等登记脚本
-- 修复原登记偶发失败的根因：proxies 表中出现重复行或名称不匹配，导致校验不通过。
-- 策略：先软删除重复行（仅保留 id 最小的一行），再 upsert 该唯一行，最后校验恰好返回一行且名称/状态正确。

-- 1. 软删除重复的新加坡代理行，仅保留 id 最小的一行
UPDATE proxies
SET deleted_at = NOW()
WHERE deleted_at IS NULL
  AND protocol = 'http'
  AND host = 'host.docker.internal'
  AND port = 17998
  AND id NOT IN (
    SELECT id FROM proxies
    WHERE deleted_at IS NULL
      AND protocol = 'http'
      AND host = 'host.docker.internal'
      AND port = 17998
    ORDER BY id
    LIMIT 1
  );

-- 2. 更新保留行；若不存在则插入唯一一行
WITH existing_proxy AS (
    UPDATE proxies
    SET name = '新加坡家宽12-Mihomo',
        status = 'active',
        updated_at = NOW()
    WHERE deleted_at IS NULL
      AND protocol = 'http'
      AND host = 'host.docker.internal'
      AND port = 17998
    RETURNING id
)
INSERT INTO proxies (name, protocol, host, port, status, fallback_mode, expiry_warn_days)
SELECT '新加坡家宽12-Mihomo', 'http', 'host.docker.internal', 17998, 'active', 'none', 7
WHERE NOT EXISTS (SELECT 1 FROM existing_proxy);

-- 3. 校验：应恰好返回一行，且名称与状态符合预期
SELECT id, name, status
FROM proxies
WHERE deleted_at IS NULL
  AND protocol = 'http'
  AND host = 'host.docker.internal'
  AND port = 17998
ORDER BY id;
