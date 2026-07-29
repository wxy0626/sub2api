[CmdletBinding()]
param(
    # 命令模式：空参数负责启动，status 只读取当前状态。
    [Parameter(Position = 0)]
    [string]$命令 = 'start'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# 项目根目录：由 tools 目录向上解析，避免受当前工作目录影响。
$项目根目录 = Split-Path -Parent $PSScriptRoot
# 本地 Compose 文件：定义 Sub2API、PostgreSQL 和 Redis 服务。
$Compose文件 = Join-Path $项目根目录 'deploy\docker-compose.local.yml'
# 本地环境文件：保存本机端口和 Compose 所需配置。
$环境文件 = Join-Path $项目根目录 'deploy\.env'
# 远端快照 Compose 文件：不含密钥，定义与远端一致的应用、PostgreSQL 15 和 Redis 7 服务。
$远端快照Compose文件 = Join-Path $项目根目录 'deploy\docker-compose.remote-8080.yml'
# 远端快照运行时环境文件：位于受限目录，保存从远端容器提取的运行时变量。
$远端快照环境文件 = Join-Path $项目根目录 '.codex\runtime\remote-8080.env'
# 日本隧道守护脚本：把既有 Mihomo 7897 反向转发至远端 17897。
$日本隧道脚本 = Join-Path $PSScriptRoot 'start-sub2api-proxy-tunnel.ps1'
# 日本隧道状态文件：由守护脚本写入，不包含任何凭据。
$日本隧道状态文件 = Join-Path $项目根目录 '.codex\log\sub2api-proxy-tunnel-state.json'
# 新加坡隧道守护脚本：把专用代理 17998 反向转发至远端 17898。
$新加坡隧道脚本 = Join-Path $PSScriptRoot 'start-sub2api-singapore-proxy-tunnel.ps1'
# 新加坡专用代理启动脚本：从当前 Clash Verge 订阅启动固定节点的本机 Mihomo 实例。
$新加坡代理脚本 = Join-Path $PSScriptRoot 'start-sub2api-singapore-proxy.ps1'
# 新加坡隧道状态文件：由守护脚本写入，不包含任何凭据。
$新加坡隧道状态文件 = Join-Path $项目根目录 '.codex\log\sub2api-singapore-proxy-tunnel-state.json'
# 新加坡代理启动标准输出日志：保留启动器的已验证节点与端口摘要。
$新加坡代理启动标准输出日志 = Join-Path $项目根目录 '.codex\log\sub2api-singapore-proxy-launcher.out.log'
# 新加坡代理启动标准错误日志：保留启动器失败时的 PowerShell 原始错误。
$新加坡代理启动标准错误日志 = Join-Path $项目根目录 '.codex\log\sub2api-singapore-proxy-launcher.err.log'
# 本地管理界面的固定回环地址。
$本地服务地址 = '127.0.0.1'
# 本地管理界面的 Compose 映射端口。
$本地服务端口 = 8080
# 日本代理给隧道守护使用的既有本机监听端口。
$日本本地代理端口 = 7897
# 新加坡代理给隧道守护使用的专用本机监听端口。
$新加坡本地代理端口 = 17998

# 测试 TCP 端口：用短连接避免状态查询被网络接口阻塞。
function 测试Sub2Api端口 {
    param(
        [Parameter(Mandatory = $true)][string]$主机地址,
        [Parameter(Mandatory = $true)][int]$端口
    )

    $客户端 = [System.Net.Sockets.TcpClient]::new()
    try {
        $异步连接 = $客户端.BeginConnect($主机地址, $端口, $null, $null)
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

# 获取指定隧道守护进程：只匹配本项目脚本，不接管其他 SSH 或隧道进程。
function 获取Sub2Api隧道守护进程 {
    param([Parameter(Mandatory = $true)][string]$目标隧道脚本)

    # 隧道脚本文件名：用于限定后台守护进程的匹配范围。
    $隧道脚本文件名 = [System.IO.Path]::GetFileName($目标隧道脚本)
    return @(
        Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
            Where-Object {
                $_.Name -in @('pwsh.exe', 'powershell.exe') -and
                -not [string]::IsNullOrWhiteSpace($_.CommandLine) -and
                $_.CommandLine -like "*$隧道脚本文件名*"
            }
    )
}

# 启动指定隧道守护：已有守护进程时保持原状，避免产生重复隧道。
function 确保Sub2Api隧道守护 {
    param(
        [Parameter(Mandatory = $true)][string]$隧道名称,
        [Parameter(Mandatory = $true)][string]$目标隧道脚本
    )

    if (-not (Test-Path -LiteralPath $目标隧道脚本 -PathType Leaf)) {
        throw "找不到$隧道名称隧道守护脚本：$目标隧道脚本"
    }

    # 既有守护进程：存在时复用，以保持已建立的反向转发。
    $已有守护进程 = @(获取Sub2Api隧道守护进程 -目标隧道脚本 $目标隧道脚本)
    if ($已有守护进程.Count -gt 0) {
        Write-Host "${隧道名称}隧道守护已运行（PID：$($已有守护进程[0].ProcessId)）。" -ForegroundColor DarkGreen
        return
    }

    # PowerShell 7 可执行文件：与用户裸命令的宿主保持一致。
    $PowerShell可执行文件 = Join-Path $PSHOME 'pwsh.exe'
    if (-not (Test-Path -LiteralPath $PowerShell可执行文件 -PathType Leaf)) {
        throw "找不到 PowerShell 7 可执行文件：$PowerShell可执行文件"
    }

    # 守护启动参数：无 Profile 运行，避免交互配置影响后台进程。
    $守护启动参数 = @('-NoLogo', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $目标隧道脚本)
    $新守护进程 = Start-Process -FilePath $PowerShell可执行文件 -ArgumentList $守护启动参数 -WindowStyle Hidden -PassThru
    Start-Sleep -Milliseconds 500
    if ($新守护进程.HasExited) {
        throw "隧道守护启动后立即退出，退出码：$($新守护进程.ExitCode)"
    }

    Write-Host "${隧道名称}隧道守护已启动（PID：$($新守护进程.Id)）。" -ForegroundColor DarkGreen
}

# 确保新加坡专用代理：仅在 17998 未监听时运行历史启动器，避免重建已可用的固定出口。
function 确保Sub2Api新加坡代理 {
    if (测试Sub2Api端口 -主机地址 $本地服务地址 -端口 $新加坡本地代理端口) {
        Write-Host "新加坡专用代理已监听（127.0.0.1:$新加坡本地代理端口）。" -ForegroundColor DarkGreen
        return
    }

    if (-not (Test-Path -LiteralPath $新加坡代理脚本 -PathType Leaf)) {
        throw "找不到新加坡专用代理启动脚本：$新加坡代理脚本"
    }

    # PowerShell 7 可执行文件：与用户裸命令的宿主保持一致。
    $PowerShell可执行文件 = Join-Path $PSHOME 'pwsh.exe'
    if (-not (Test-Path -LiteralPath $PowerShell可执行文件 -PathType Leaf)) {
        throw "找不到 PowerShell 7 可执行文件：$PowerShell可执行文件"
    }

    # 启动参数：历史脚本会自行隐藏 Mihomo，并在完成节点 TLS 验证后退出。
    $代理启动参数 = @('-NoLogo', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $新加坡代理脚本)
    Remove-Item -LiteralPath $新加坡代理启动标准输出日志, $新加坡代理启动标准错误日志 -Force -ErrorAction SilentlyContinue
    Write-Host '正在启动并验证新加坡专用代理...' -ForegroundColor Cyan
    $代理启动进程 = Start-Process -FilePath $PowerShell可执行文件 -ArgumentList $代理启动参数 -WindowStyle Hidden -RedirectStandardOutput $新加坡代理启动标准输出日志 -RedirectStandardError $新加坡代理启动标准错误日志 -PassThru

    if (-not $代理启动进程.WaitForExit(45000)) {
        Stop-Process -Id $代理启动进程.Id -Force -ErrorAction SilentlyContinue
        throw '新加坡专用代理启动器在 45 秒内未完成。请查看 .codex\log\sub2api-singapore-proxy-launcher.err.log。'
    }
    if ($代理启动进程.ExitCode -ne 0) {
        throw "新加坡专用代理启动失败，退出码：$($代理启动进程.ExitCode)。请查看 .codex\log\sub2api-singapore-proxy-launcher.err.log。"
    }
    if (-not (测试Sub2Api端口 -主机地址 $本地服务地址 -端口 $新加坡本地代理端口)) {
        throw '新加坡专用代理启动后未监听 127.0.0.1:17998。请查看 .codex\log\sub2api-singapore-proxy-launcher.err.log。'
    }

    Write-Host "新加坡专用代理已启动（127.0.0.1:$新加坡本地代理端口）。" -ForegroundColor DarkGreen
}

# 读取指定隧道状态：状态文件不存在或格式异常时返回可读提示。
function 获取Sub2Api隧道状态 {
    param([Parameter(Mandatory = $true)][string]$目标隧道状态文件)

    if (-not (Test-Path -LiteralPath $目标隧道状态文件 -PathType Leaf)) {
        return '未生成状态文件'
    }

    try {
        # 状态对象：仅显示守护脚本的连接摘要。
        $状态对象 = Get-Content -LiteralPath $目标隧道状态文件 -Raw -Encoding UTF8 | ConvertFrom-Json
        return "$($状态对象.status)：$($状态对象.message)"
    } catch {
        return "状态文件无法读取：$($_.Exception.Message)"
    }
}

# 输出运行状态：集中显示服务、健康检查、代理和隧道的当前情况。
function 显示Sub2Api状态 {
    # 当前运行配置：远端恢复完成后优先显示实际生效的配置来源。
    $运行配置来源 = if ((Test-Path -LiteralPath $远端快照Compose文件 -PathType Leaf) -and (Test-Path -LiteralPath $远端快照环境文件 -PathType Leaf)) { '远端快照' } else { '本地默认' }
    $服务端口已监听 = 测试Sub2Api端口 -主机地址 $本地服务地址 -端口 $本地服务端口
    # 日本代理端口状态：对应远端 17897。
    $日本代理端口已监听 = 测试Sub2Api端口 -主机地址 $本地服务地址 -端口 $日本本地代理端口
    # 新加坡代理端口状态：对应远端 17898。
    $新加坡代理端口已监听 = 测试Sub2Api端口 -主机地址 $本地服务地址 -端口 $新加坡本地代理端口
    $健康检查结果 = '未响应'
    if ($服务端口已监听) {
        try {
            $健康响应 = Invoke-WebRequest -Uri "http://$本地服务地址`:$本地服务端口/health" -TimeoutSec 3
            $健康检查结果 = "HTTP $($健康响应.StatusCode) $($健康响应.Content.Trim())"
        } catch {
            $健康检查结果 = "请求失败：$($_.Exception.Message)"
        }
    }

    # 日本守护与状态：用于确认 7897 到远端 17897 的反向转发。
    $日本守护进程 = @(获取Sub2Api隧道守护进程 -目标隧道脚本 $日本隧道脚本)
    $日本守护摘要 = if ($日本守护进程.Count -gt 0) { "运行中（PID：$($日本守护进程[0].ProcessId)）" } else { '未运行' }
    $日本隧道摘要 = 获取Sub2Api隧道状态 -目标隧道状态文件 $日本隧道状态文件
    # 新加坡守护与状态：用于确认 17998 到远端 17898 的反向转发。
    $新加坡守护进程 = @(获取Sub2Api隧道守护进程 -目标隧道脚本 $新加坡隧道脚本)
    $新加坡守护摘要 = if ($新加坡守护进程.Count -gt 0) { "运行中（PID：$($新加坡守护进程[0].ProcessId)）" } else { '未运行' }
    $新加坡隧道摘要 = 获取Sub2Api隧道状态 -目标隧道状态文件 $新加坡隧道状态文件

    Write-Host ''
    Write-Host 'Sub2API 状态' -ForegroundColor Cyan
    Write-Host "运行配置：$运行配置来源"
    Write-Host "本地服务：$(if ($服务端口已监听) { '端口已监听' } else { '端口未监听' })（http://$本地服务地址`:$本地服务端口）"
    Write-Host "健康检查：$健康检查结果"
    Write-Host "日本代理：$(if ($日本代理端口已监听) { '127.0.0.1:7897 已监听' } else { '127.0.0.1:7897 未监听' })"
    Write-Host "日本隧道守护：$日本守护摘要"
    Write-Host "日本隧道状态：$日本隧道摘要"
    Write-Host "新加坡代理：$(if ($新加坡代理端口已监听) { '127.0.0.1:17998 已监听' } else { '127.0.0.1:17998 未监听' })"
    Write-Host "新加坡隧道守护：$新加坡守护摘要"
    Write-Host "新加坡隧道状态：$新加坡隧道摘要"
}

# 启动 Compose 服务：不停止或重建已有容器，只交由 Compose 补齐缺失服务。
function 启动Sub2Api服务 {
    # 活动配置：远端恢复配置存在时优先使用，确保裸命令 sub2api 不会重新启动旧上游镜像。
    $活动Compose文件 = if (Test-Path -LiteralPath $远端快照Compose文件 -PathType Leaf) { $远端快照Compose文件 } else { $Compose文件 }
    # 活动环境：远端恢复配置必须配合受限环境文件；本地默认配置保持原有环境文件。
    $活动环境文件 = if ($活动Compose文件 -eq $远端快照Compose文件) { $远端快照环境文件 } else { $环境文件 }

    if (-not (Test-Path -LiteralPath $活动Compose文件 -PathType Leaf)) {
        throw "找不到 Compose 文件：$活动Compose文件"
    }
    if (-not (Test-Path -LiteralPath $活动环境文件 -PathType Leaf)) {
        throw "找不到环境文件：$活动环境文件"
    }

    Write-Host '正在确保本地 Sub2API 服务运行...' -ForegroundColor Cyan
    & docker compose --env-file $活动环境文件 -f $活动Compose文件 up -d
    if ($LASTEXITCODE -ne 0) {
        throw "Docker Compose 启动失败，退出码：$LASTEXITCODE"
    }
}

switch ($命令.ToLowerInvariant()) {
    'start' {
        启动Sub2Api服务
        确保Sub2Api隧道守护 -隧道名称 '日本' -目标隧道脚本 $日本隧道脚本
        确保Sub2Api新加坡代理
        确保Sub2Api隧道守护 -隧道名称 '新加坡' -目标隧道脚本 $新加坡隧道脚本
        显示Sub2Api状态
    }
    'status' {
        显示Sub2Api状态
    }
    'help' {
        Write-Host '用法：sub2api [start|status|help]'
        Write-Host '  sub2api         确保本地 Compose 服务、新加坡专用代理与既有隧道守护已启动，并输出状态。'
        Write-Host '  sub2api status  只显示本地服务、代理与隧道状态。'
    }
    default {
        throw "不支持的命令：$命令。可用命令：start、status、help。"
    }
}
