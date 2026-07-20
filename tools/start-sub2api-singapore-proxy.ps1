[CmdletBinding()]
param(
    # 重新选择节点：仅在现有固定节点不可用或管理员明确要求切换时使用。
    [switch]$重新选择,
    # 只验证：不启动新进程，仅检查已运行的新加坡专用代理。
    [switch]$只验证
)

# Clash Verge 数据目录：读取当前订阅和节点定义，不改动其全局配置或节点选择。
$ClashVerge数据目录 = Join-Path $env:APPDATA 'io.github.clash-verge-rev.clash-verge-rev'
# Mihomo 可执行文件：专用实例与 Clash Verge 全局实例使用同一版本，避免协议兼容差异。
$Mihomo程序 = 'D:\VPN\Clash Verge\verge-mihomo.exe'
# 项目日志目录：保存专用实例的启动、探测和运行日志。
$日志目录 = 'E:\AI\sun2api\.codex\log'
# 项目运行目录：生成的最小节点配置含订阅凭据，已被 Git 忽略。
$运行目录 = 'E:\AI\sun2api\.codex\runtime'
# 新加坡专用 HTTP 代理端口：不与 Clash Verge 全局 mixed-port 7897 冲突。
$本地代理端口 = 17998
# 新加坡专用 SOCKS 端口：保留给本机诊断，Sub2API 仅使用 HTTP 端口。
$本地SOCKS端口 = 17999
# 新加坡专用纯 HTTP 端口：避免与 mixed-port 产生歧义。
$本地HTTP端口 = 18000
# 专用配置文件：仅包含一个固定的新加坡节点和 GLOBAL 选择组。
$运行配置文件 = Join-Path $运行目录 'sub2api-singapore-mihomo.yaml'
# 专用状态文件：记录已验证的固定节点和进程信息，不写入订阅凭据。
$状态文件 = Join-Path $日志目录 'sub2api-singapore-proxy-state.json'
# 专用实例标准输出日志。
$标准输出日志 = Join-Path $日志目录 'sub2api-singapore-mihomo.out.log'
# 专用实例标准错误日志。
$标准错误日志 = Join-Path $日志目录 'sub2api-singapore-mihomo.err.log'
# 专用节点探测日志：保存每个候选节点是否通过 ChatGPT TLS 验证。
$探测日志 = Join-Path $日志目录 'sub2api-singapore-proxy-probe.log'

# 新加坡候选节点优先级：优先家宽节点，首次部署只会选出一个通过 TLS 探测的节点并固定保存。
$新加坡候选节点 = @(
    '🇸🇬 新加坡 01 [0.3X]',
    '🇸🇬 新加坡 12 家宽',
    '🇸🇬 新加坡 02',
    '🇸🇬 新加坡 03',
    '🇸🇬 新加坡 04',
    '🇸🇬 新加坡 05',
    '🇸🇬 新加坡 06',
    '🇸🇬 新加坡 07',
    '🇸🇬 新加坡 08',
    '🇸🇬 新加坡 09',
    '🇸🇬 新加坡 10',
    '🇸🇬 新加坡 11'
)
# 默认固定节点：已在本机完成 chatgpt.com TLS 验证，普通启动绝不自动切换到其他出口。
$默认固定新加坡节点 = '🇸🇬 新加坡 12 家宽'

# 写入运行日志：将节点名称和验证结果保留在项目日志中，方便后续准确排查。
function 写入新加坡代理日志 {
    param([Parameter(Mandatory = $true)][string]$消息)

    Add-Content -LiteralPath $探测日志 -Value "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') $消息" -Encoding UTF8
}

# 获取活动订阅文件：依据 Clash Verge 当前 profile UID 读取实际生效的远程订阅。
function 获取活动订阅文件 {
    $Profile索引文件 = Join-Path $ClashVerge数据目录 'profiles.yaml'
    if (-not (Test-Path -LiteralPath $Profile索引文件 -PathType Leaf)) {
        throw "未找到 Clash Verge 配置索引：$Profile索引文件。请先启动 Clash Verge 并加载订阅。"
    }

    $索引内容 = Get-Content -LiteralPath $Profile索引文件 -Raw -Encoding UTF8
    $当前Profile匹配 = [regex]::Match($索引内容, '(?m)^current:\s*(?<uid>\S+)\s*$')
    if (-not $当前Profile匹配.Success) {
        throw 'Clash Verge profiles.yaml 未包含 current Profile，无法确定新加坡节点来源。'
    }

    # 当前 Profile UID：转义后再拼入正则，避免随机 UID 中的特殊字符影响匹配。
    $当前ProfileUID = [regex]::Escape($当前Profile匹配.Groups['uid'].Value)
    $Profile区块匹配 = [regex]::Match(
        $索引内容,
        "(?ms)^-\s+uid:\s*$当前ProfileUID\s*$.*?(?=^-\s+uid:|\z)"
    )
    if (-not $Profile区块匹配.Success) {
        throw '无法在 Clash Verge profiles.yaml 中找到当前 Profile 的配置块。'
    }

    $订阅文件匹配 = [regex]::Match($Profile区块匹配.Value, '(?m)^\s*file:\s*(?<file>\S+)\s*$')
    if (-not $订阅文件匹配.Success) {
        throw '当前 Clash Verge Profile 未配置本地订阅文件。'
    }

    $订阅文件 = Join-Path (Join-Path $ClashVerge数据目录 'profiles') $订阅文件匹配.Groups['file'].Value
    if (-not (Test-Path -LiteralPath $订阅文件 -PathType Leaf)) {
        throw "当前 Clash Verge 订阅文件不存在：$订阅文件。请在 Clash Verge 中刷新订阅后重试。"
    }

    return $订阅文件
}

# 获取节点原始 YAML 行：保留订阅提供的全部协议字段，不在脚本中复制或暴露凭据。
function 获取节点原始配置行 {
    param(
        [Parameter(Mandatory = $true)][string]$订阅文件,
        [Parameter(Mandatory = $true)][string]$节点名称
    )

    foreach ($行 in Get-Content -LiteralPath $订阅文件 -Encoding UTF8) {
        $节点匹配 = [regex]::Match($行, "^\s*-\s*\{\s*name:\s*'(?<name>[^']+)'")
        if ($节点匹配.Success -and $节点匹配.Groups['name'].Value -eq $节点名称) {
            return $行.Trim()
        }
    }

    throw "当前订阅中不存在新加坡节点：$节点名称。请刷新 Clash Verge 订阅或使用 -重新选择。"
}

# 写入最小专用配置：只有目标节点和一个 GLOBAL 组，确保 17998 的流量固定经过该节点。
function 写入新加坡专用配置 {
    param(
        [Parameter(Mandatory = $true)][string]$节点名称,
        [Parameter(Mandatory = $true)][string]$节点配置行
    )

    New-Item -ItemType Directory -Force -Path $运行目录, $日志目录 | Out-Null
    # 节点名称经 YAML 单引号转义后写入选择组，配置本身仅位于 Git 忽略目录。
    $YAML节点名称 = $节点名称.Replace("'", "''")
    $配置内容 = @(
        "mixed-port: $本地代理端口",
        "socks-port: $本地SOCKS端口",
        "port: $本地HTTP端口",
        'bind-address: 127.0.0.1',
        'allow-lan: false',
        'mode: global',
        'ipv6: true',
        'log-level: info',
        'unified-delay: true',
        'proxies:',
        "  $节点配置行",
        'proxy-groups:',
        '  - name: GLOBAL',
        '    type: select',
        "    proxies: ['$YAML节点名称']",
        'rules:',
        '  - MATCH,GLOBAL'
    )
    Set-Content -LiteralPath $运行配置文件 -Value $配置内容 -Encoding UTF8
}

# 停止指定进程：仅清理由本脚本创建、并记录在状态文件中的旧专用实例。
function 停止旧新加坡专用实例 {
    $状态 = 获取新加坡代理状态
    if (-not $状态 -or -not $状态.mihomo_pid) {
        return
    }

    $旧进程 = Get-Process -Id ([int]$状态.mihomo_pid) -ErrorAction SilentlyContinue
    if ($旧进程 -and $旧进程.ProcessName -eq 'verge-mihomo') {
        Stop-Process -Id $旧进程.Id -ErrorAction Stop
        Wait-Process -Id $旧进程.Id -Timeout 5 -ErrorAction SilentlyContinue
        写入新加坡代理日志 "已停止旧的新加坡专用 Mihomo 进程：$($旧进程.Id)。"
        Start-Sleep -Milliseconds 500
    }
}

# 获取状态文件：写入瞬间可能不完整，解析异常时按不存在处理。
function 获取新加坡代理状态 {
    if (-not (Test-Path -LiteralPath $状态文件 -PathType Leaf)) {
        return $null
    }

    try {
        return Get-Content -LiteralPath $状态文件 -Raw -Encoding UTF8 | ConvertFrom-Json -ErrorAction Stop
    } catch {
        return $null
    }
}

# 测试专用代理端口：只接受 loopback 监听，避免意外将代理暴露到局域网。
function 测试新加坡代理已监听 {
    # TcpClient 短连接：避免 Get-NetTCPConnection 在部分 Windows 网络状态下阻塞脚本。
    $客户端 = [System.Net.Sockets.TcpClient]::new()
    try {
        $异步连接 = $客户端.BeginConnect('127.0.0.1', $本地代理端口, $null, $null)
        if (-not $异步连接.AsyncWaitHandle.WaitOne(1000)) {
            return $false
        }
        $客户端.EndConnect($异步连接)
        return $true
    } catch {
        return $false
    } finally {
        $客户端.Dispose()
    }
}

# 测试 ChatGPT TLS：使用显式 HTTP 代理并要求 curl 无 EOF/超时，HTTP 状态非 2xx 仍可证明 TLS 出站成功。
function 测试新加坡ChatGPTTLS {
    $响应头文件 = Join-Path $运行目录 'sub2api-singapore-chatgpt.headers.txt'
    $错误文件 = Join-Path $运行目录 'sub2api-singapore-chatgpt.stderr.txt'
    Remove-Item -LiteralPath $响应头文件, $错误文件 -Force -ErrorAction SilentlyContinue

    & curl.exe --proxy "http://127.0.0.1:$本地代理端口" --connect-timeout 8 --max-time 12 --silent --show-error --output NUL --dump-header $响应头文件 'https://chatgpt.com/' 2> $错误文件
    $退出码 = $LASTEXITCODE
    # curl 成功时可能创建空错误文件；空内容必须按正常路径处理，不能对空值调用 Trim。
    $原始错误详情 = if (Test-Path -LiteralPath $错误文件) { Get-Content -LiteralPath $错误文件 -Raw -Encoding UTF8 } else { '' }
    $错误详情 = if ($null -eq $原始错误详情) { '' } else { $原始错误详情.Trim() }
    $首个状态行 = if (Test-Path -LiteralPath $响应头文件) {
        (Get-Content -LiteralPath $响应头文件 -Encoding UTF8 | Where-Object { $_ -match '^HTTP/' } | Select-Object -Last 1)
    } else {
        $null
    }

    if ($退出码 -ne 0 -or [string]::IsNullOrWhiteSpace($首个状态行) -or $错误详情 -match '(?i)EOF|timed out|connection reset|unexpected eof') {
        $原因 = if ($错误详情) { $错误详情 } elseif ($首个状态行) { "curl 退出码 $退出码；$首个状态行" } else { "curl 退出码 $退出码，未收到 HTTP 响应头" }
        写入新加坡代理日志 "ChatGPT TLS 验证失败：$原因"
        return $false
    }

    写入新加坡代理日志 "ChatGPT TLS 验证成功：$首个状态行"
    return $true
}

# 启动专用 Mihomo：先做配置语法校验，成功后在后台运行并等待本地端口就绪。
function 启动新加坡专用Mihomo {
    if (-not (Test-Path -LiteralPath $Mihomo程序 -PathType Leaf)) {
        throw "未找到 Mihomo 程序：$Mihomo程序。请确认 Clash Verge 安装目录。"
    }

    & $Mihomo程序 -t -d $ClashVerge数据目录 -f $运行配置文件 *> $标准错误日志
    if ($LASTEXITCODE -ne 0) {
        throw "新加坡专用 Mihomo 配置校验失败。请查看日志：$标准错误日志"
    }

    $进程 = Start-Process -FilePath $Mihomo程序 -ArgumentList @('-d', $ClashVerge数据目录, '-f', $运行配置文件) -WindowStyle Hidden -RedirectStandardOutput $标准输出日志 -RedirectStandardError $标准错误日志 -PassThru
    $截止时间 = (Get-Date).AddSeconds(15)
    while ((Get-Date) -lt $截止时间) {
        if (测试新加坡代理已监听) {
            # AnyTLS 节点在端口就绪后仍需要短暂初始化；等待完成后再发起 ChatGPT TLS 探针。
            Start-Sleep -Seconds 2
            return $进程
        }
        if ($进程.HasExited) {
            throw "新加坡专用 Mihomo 已提前退出，退出码 $($进程.ExitCode)。请查看日志：$标准错误日志"
        }
        Start-Sleep -Milliseconds 250
    }

    throw "新加坡专用 Mihomo 在 15 秒内未监听 127.0.0.1:$本地代理端口。请查看日志：$标准错误日志"
}

# 写入成功状态：状态不包含代理节点的服务器、密码或订阅 URL。
function 写入新加坡代理状态 {
    param(
        [Parameter(Mandatory = $true)][string]$节点名称,
        [Parameter(Mandatory = $true)][System.Diagnostics.Process]$Mihomo进程
    )

    $状态对象 = [ordered]@{
        status = 'connected'
        selected_node = $节点名称
        local_http_proxy = "127.0.0.1:$本地代理端口"
        mihomo_pid = $Mihomo进程.Id
        updated_at = (Get-Date).ToString('o')
    }
    $状态对象 | ConvertTo-Json -Compress | Set-Content -LiteralPath $状态文件 -Encoding UTF8
}

New-Item -ItemType Directory -Force -Path $日志目录, $运行目录 | Out-Null

if ($只验证) {
    if (-not (测试新加坡代理已监听)) {
        throw "新加坡专用代理未监听 127.0.0.1:$本地代理端口。请先运行 $PSCommandPath。"
    }
    if (-not (测试新加坡ChatGPTTLS)) {
        throw "新加坡专用代理未通过 chatgpt.com TLS 验证。请检查 $探测日志。"
    }
    Write-Output "[sub2api] 新加坡专用代理已通过 ChatGPT TLS 验证：127.0.0.1:$本地代理端口"
    exit 0
}

$现有状态 = 获取新加坡代理状态
# 普通启动只使用上次已验证节点或默认固定节点；只有显式 -重新选择 才允许管理员重新挑选出口。
$候选节点 = if ($重新选择) {
    $新加坡候选节点
} elseif ($现有状态 -and $现有状态.selected_node) {
    @([string]$现有状态.selected_node)
} else {
    @($默认固定新加坡节点)
}
$订阅文件 = 获取活动订阅文件
$最后错误 = $null

foreach ($节点名称 in $候选节点) {
    try {
        停止旧新加坡专用实例
        $节点配置行 = 获取节点原始配置行 -订阅文件 $订阅文件 -节点名称 $节点名称
        写入新加坡专用配置 -节点名称 $节点名称 -节点配置行 $节点配置行
        $Mihomo进程 = 启动新加坡专用Mihomo
        if (-not (测试新加坡ChatGPTTLS)) {
            Stop-Process -Id $Mihomo进程.Id -ErrorAction SilentlyContinue
            Wait-Process -Id $Mihomo进程.Id -Timeout 5 -ErrorAction SilentlyContinue
            Start-Sleep -Milliseconds 500
            throw 'ChatGPT TLS 验证未通过。'
        }
        写入新加坡代理状态 -节点名称 $节点名称 -Mihomo进程 $Mihomo进程
        Write-Output "[sub2api] 新加坡专用代理已启动：节点 $节点名称，127.0.0.1:$本地代理端口"
        exit 0
    } catch {
        $最后错误 = $_.Exception.Message
        写入新加坡代理日志 "节点 $节点名称 未通过验证：$最后错误"
        if (-not $重新选择) {
            break
        }
    }
}

throw "未找到可用的新加坡节点。最后错误：$最后错误。请查看 $探测日志。"
