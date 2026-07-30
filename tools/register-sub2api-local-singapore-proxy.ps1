[CmdletBinding()]
param()

# Sub2API 应用容器名称：用于从实际运行容器验证新加坡代理可访问。
$应用容器名称 = 'sub2api'
# PostgreSQL 容器名称：只在本地 Compose 环境登记代理，不访问远端数据库。
$数据库容器名称 = 'sub2api-postgres'
# 新加坡代理展示名称：与固定的新加坡家宽节点保持一致，便于账号列表选择。
$新加坡代理名称 = '新加坡家宽12-Mihomo'
# 新加坡代理协议：Mihomo 专用实例暴露的是 HTTP 代理端口。
$新加坡代理协议 = 'http'
# 新加坡代理容器访问主机：Docker Desktop 容器通过该域名访问 Windows 主机监听端口。
$新加坡代理主机 = 'host.docker.internal'
# 新加坡代理端口：必须与 start-sub2api-singapore-proxy.ps1 的 mixed-port 一致。
$新加坡代理端口 = 17998
# 新加坡代理登记日志：仅记录连通性状态和代理 ID，不记录数据库配置或任何凭据。
$登记日志文件 = 'E:\AI\sun2api\.codex\log\sub2api-local-singapore-proxy-registration.log'

# 写入新加坡代理登记日志：为本地启动和故障排查保留不含敏感信息的结果。
function 写入新加坡代理登记日志 {
    param([Parameter(Mandatory = $true)][string]$消息)

    $日志目录 = Split-Path -Parent $登记日志文件
    New-Item -ItemType Directory -Force -Path $日志目录 | Out-Null
    Add-Content -LiteralPath $登记日志文件 -Value "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') $消息" -Encoding UTF8
}

# 获取容器环境变量值：不写入日志，避免管理员密码等敏感配置被输出。
function 获取容器环境变量值 {
    param(
        [Parameter(Mandatory = $true)][string]$容器名称,
        [Parameter(Mandatory = $true)][string]$变量名
    )

    $容器原始信息 = & docker.exe inspect $容器名称
    if ($LASTEXITCODE -ne 0) {
        throw "无法读取容器 $容器名称 的运行配置。请确认本地 Docker 容器已启动。"
    }

    $容器信息 = ($容器原始信息 -join "`n") | ConvertFrom-Json -ErrorAction Stop
    $变量前缀 = "$变量名="
    $变量项 = @($容器信息[0].Config.Env | Where-Object { $_.StartsWith($变量前缀, [System.StringComparison]::Ordinal) }) | Select-Object -First 1
    if ([string]::IsNullOrWhiteSpace($变量项)) {
        throw "容器 $容器名称 未提供 $变量名，无法安全登记本地新加坡代理。"
    }

    return $变量项.Substring($变量前缀.Length)
}

# 测试 Sub2API 容器经新加坡代理访问 ChatGPT：有 HTTP 响应即代表容器到代理和 TLS 出站链路可用。
function 测试Sub2API容器新加坡代理 {
    $代理地址 = "$新加坡代理协议`://$新加坡代理主机`:$新加坡代理端口"
    $探测命令 = "HTTPS_PROXY='$代理地址' https_proxy='$代理地址' wget -S --spider --timeout=12 https://chatgpt.com/ 2>&1"
    $探测输出 = & docker.exe exec $应用容器名称 sh -lc $探测命令
    $探测退出码 = $LASTEXITCODE
    $状态行 = @(
        $探测输出 |
            Select-String -Pattern 'HTTP/[0-9.]+\s+[0-9]{3}' |
            ForEach-Object { $_.Matches[0].Value }
    ) | Select-Object -Last 1

    if ([string]::IsNullOrWhiteSpace($状态行)) {
        throw "Sub2API 容器未能通过新加坡代理 $新加坡代理主机`:$新加坡代理端口 获得 ChatGPT HTTP 响应（wget 退出码 $探测退出码），不会登记不可用代理。"
    }

    $状态码匹配 = [regex]::Match($状态行, '\s(?<status>[0-9]{3})$')
    if (-not $状态码匹配.Success) {
        throw "Sub2API 容器的新加坡代理探测响应格式异常：$状态行。不会登记不可用代理。"
    }

    $状态码 = [int]$状态码匹配.Groups['status'].Value
    if ($状态码 -lt 200 -or $状态码 -gt 599) {
        throw "Sub2API 容器的新加坡代理探测返回无效 HTTP 状态码 $状态码，不会登记不可用代理。"
    }

    return $状态码
}

# 确保本地新加坡代理已登记：按协议、容器主机和端口幂等更新或插入，不改写日本代理、账号绑定或任何凭据。
function 确保本地新加坡代理已登记 {
    $数据库用户 = 获取容器环境变量值 -容器名称 $数据库容器名称 -变量名 'POSTGRES_USER'
    $数据库名称 = 获取容器环境变量值 -容器名称 $数据库容器名称 -变量名 'POSTGRES_DB'
    $登记SQL = @'
WITH existing_proxy AS (
    UPDATE proxies
    SET name = :'proxy_name',
        status = 'active',
        updated_at = NOW()
    WHERE deleted_at IS NULL
      AND protocol = :'proxy_protocol'
      AND host = :'proxy_host'
      AND port = :proxy_port
    RETURNING id
)
INSERT INTO proxies (name, protocol, host, port, status, fallback_mode, expiry_warn_days)
SELECT :'proxy_name', :'proxy_protocol', :'proxy_host', :proxy_port, 'active', 'none', 7
WHERE NOT EXISTS (SELECT 1 FROM existing_proxy);
'@

    # 使用标准输入执行 SQL：psql 的 -c 模式不会展开 :变量 占位符，标准输入可保留参数化边界。
    $null = $登记SQL | & docker.exe exec -i $数据库容器名称 psql -X -v ON_ERROR_STOP=1 -U $数据库用户 -d $数据库名称 -v "proxy_name=$新加坡代理名称" -v "proxy_protocol=$新加坡代理协议" -v "proxy_host=$新加坡代理主机" -v "proxy_port=$新加坡代理端口" -f -
    if ($LASTEXITCODE -ne 0) {
        throw '新加坡代理已通过连通性验证，但写入本地代理列表失败。请检查 sub2api-postgres 容器状态和数据库日志。'
    }

    $验证SQL = @'
SELECT id, name, status
FROM proxies
WHERE deleted_at IS NULL
  AND protocol = :'proxy_protocol'
  AND host = :'proxy_host'
  AND port = :proxy_port
ORDER BY id;
'@
    $登记结果 = @($验证SQL | & docker.exe exec -i $数据库容器名称 psql -X -v ON_ERROR_STOP=1 -U $数据库用户 -d $数据库名称 -At -F '|' -v "proxy_protocol=$新加坡代理协议" -v "proxy_host=$新加坡代理主机" -v "proxy_port=$新加坡代理端口" -f -)
    if ($LASTEXITCODE -ne 0 -or $登记结果.Count -ne 1) {
        throw '新加坡代理登记后校验失败：本地代理列表中未找到唯一的对应记录。'
    }

    $已登记字段 = $登记结果[0].Split('|')
    if ($已登记字段.Count -ne 3 -or $已登记字段[1] -ne $新加坡代理名称 -or $已登记字段[2] -ne 'active') {
        throw '新加坡代理登记后状态异常：名称或启用状态不符合预期。'
    }

    return [int64]$已登记字段[0]
}

try {
    $响应状态码 = 测试Sub2API容器新加坡代理
    $新加坡代理ID = 确保本地新加坡代理已登记
    $成功消息 = "本地新加坡代理已可选择：ID $新加坡代理ID，$新加坡代理名称（容器探测 HTTP $响应状态码）。"
    写入新加坡代理登记日志 $成功消息
    Write-Output "[sub2api] $成功消息"
} catch {
    写入新加坡代理登记日志 "登记失败：$($_.Exception.Message)"
    throw
}
