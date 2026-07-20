# 隧道私钥路径：仅用于连接本人的 ECS，不会写入日志或状态文件。
$私钥路径 = Join-Path $env:USERPROFILE '.ssh\sub2api_proxy_tunnel'
# 项目日志目录：与日本隧道分离保存，避免状态混淆。
$日志目录 = 'E:\AI\sun2api\.codex\log'
# 新加坡专用本地 HTTP 代理监听地址。
$本地代理地址 = '127.0.0.1'
# 新加坡专用本地 HTTP 代理端口。
$本地代理端口 = 17998
# 新加坡专用远端反向转发端口。
$远端代理端口 = 17898
# 新加坡 SSH 隧道标准输出日志。
$标准输出日志 = Join-Path $日志目录 'sub2api-singapore-proxy-tunnel.out.log'
# 新加坡 SSH 隧道标准错误日志。
$标准错误日志 = Join-Path $日志目录 'sub2api-singapore-proxy-tunnel.err.log'
# 新加坡 SSH 隧道守护状态日志。
$状态日志 = Join-Path $日志目录 'sub2api-singapore-proxy-tunnel-launcher.log'
# 新加坡 SSH 隧道机器可读状态文件。
$状态文件 = Join-Path $日志目录 'sub2api-singapore-proxy-tunnel-state.json'

# SSH 参数：只建立 17898 到新加坡专用 17998 的反向隧道，不影响日本 17897 隧道。
$SSH参数 = @(
    '-i', $私钥路径,
    '-N',
    '-o', 'BatchMode=yes',
    '-o', 'ExitOnForwardFailure=yes',
    '-o', 'ServerAliveInterval=30',
    '-o', 'ServerAliveCountMax=3',
    '-o', 'TCPKeepAlive=yes',
    '-R', "172.17.0.1:$远端代理端口`:$本地代理地址`:$本地代理端口",
    'root@118.31.186.169'
)

# 写入隧道日志：保留守护生命周期和明确失败原因。
function 写入新加坡隧道日志 {
    param([Parameter(Mandatory = $true)][string]$消息)
    Add-Content -LiteralPath $状态日志 -Value "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') $消息" -Encoding UTF8
}

# 写入隧道状态：只记录进程和连接状态，不记录私钥及任何凭据。
function 写入新加坡隧道状态 {
    param(
        [Parameter(Mandatory = $true)][string]$状态,
        [Parameter(Mandatory = $true)][string]$消息,
        [Nullable[int]]$SSH进程ID = $null,
        [Nullable[int]]$SSH退出码 = $null
    )

    $隧道状态对象 = [ordered]@{
        status = $状态
        message = $消息
        local_proxy = "$本地代理地址`:$本地代理端口"
        remote_proxy = "172.17.0.1:$远端代理端口"
        updated_at = (Get-Date).ToString('o')
        guardian_pid = $PID
        ssh_pid = $SSH进程ID
        ssh_exit_code = $SSH退出码
    }
    $隧道状态对象 | ConvertTo-Json -Compress | Set-Content -LiteralPath $状态文件 -Encoding UTF8
}

# 获取同一端口的既有 SSH 进程：只识别新加坡端口，绝不接管日本隧道。
function 获取现有新加坡隧道进程 {
    Get-CimInstance Win32_Process -Filter "Name = 'ssh.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -match '172\.17\.0\.1:17898:127\.0\.0\.1:17998' }
}

# 测试本机新加坡专用代理监听状态。
function 测试新加坡本地代理已监听 {
    # TcpClient 短连接：避免 Get-NetTCPConnection 在部分 Windows 网络状态下阻塞隧道守护。
    $客户端 = [System.Net.Sockets.TcpClient]::new()
    try {
        $异步连接 = $客户端.BeginConnect($本地代理地址, $本地代理端口, $null, $null)
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

New-Item -ItemType Directory -Force -Path $日志目录 | Out-Null

# 独立互斥锁：与日本隧道守护并行运行，不共享生命周期。
$守护互斥锁名称 = 'Global\Sub2ApiSingaporeProxyTunnelGuard'
$已创建互斥锁 = $false
$守护互斥锁 = New-Object System.Threading.Mutex($false, $守护互斥锁名称, [ref]$已创建互斥锁)
if (-not $已创建互斥锁) {
    exit 0
}

$当前重试间隔秒 = 5
$最大重试间隔秒 = 60
写入新加坡隧道日志 '新加坡 SSH 隧道守护进程已启动。'
写入新加坡隧道状态 -状态 'starting' -消息 '正在检查本机新加坡专用代理与 SSH 反向隧道。'

try {
    while ($true) {
        if (-not (测试新加坡本地代理已监听)) {
            $等待消息 = "本机新加坡专用代理 $本地代理地址`:$本地代理端口 未监听，守护会在 $当前重试间隔秒 秒后复查。"
            写入新加坡隧道日志 $等待消息
            写入新加坡隧道状态 -状态 'waiting_for_local_proxy' -消息 $等待消息
            Start-Sleep -Seconds $当前重试间隔秒
            $当前重试间隔秒 = [Math]::Min($当前重试间隔秒 * 2, $最大重试间隔秒)
            continue
        }

        $已有隧道进程 = 获取现有新加坡隧道进程
        if ($已有隧道进程) {
            写入新加坡隧道状态 -状态 'connected' -消息 '已检测到现有新加坡 SSH 反向隧道进程。' -SSH进程ID $已有隧道进程[0].ProcessId
            Start-Sleep -Seconds 15
            $当前重试间隔秒 = 5
            continue
        }

        写入新加坡隧道日志 '新加坡专用代理已就绪，正在建立 SSH 反向隧道。'
        写入新加坡隧道状态 -状态 'connecting' -消息 '本机新加坡专用代理已就绪，正在建立 SSH 反向隧道。'
        $SSH进程 = Start-Process -FilePath 'ssh.exe' -ArgumentList $SSH参数 -WindowStyle Hidden -RedirectStandardOutput $标准输出日志 -RedirectStandardError $标准错误日志 -PassThru
        Start-Sleep -Milliseconds 500
        if (-not $SSH进程.HasExited) {
            写入新加坡隧道日志 "新加坡 SSH 反向隧道已建立，SSH 进程 ID $($SSH进程.Id)。"
            写入新加坡隧道状态 -状态 'connected' -消息 '新加坡 SSH 反向隧道已建立。' -SSH进程ID $SSH进程.Id
        }
        $SSH进程.WaitForExit()
        $退出消息 = "新加坡 SSH 反向隧道已退出，退出码 $($SSH进程.ExitCode)，守护会在 $当前重试间隔秒 秒后重连。"
        写入新加坡隧道日志 $退出消息
        写入新加坡隧道状态 -状态 'failed' -消息 $退出消息 -SSH进程ID $SSH进程.Id -SSH退出码 $SSH进程.ExitCode
        Start-Sleep -Seconds $当前重试间隔秒
        $当前重试间隔秒 = [Math]::Min($当前重试间隔秒 * 2, $最大重试间隔秒)
    }
} finally {
    写入新加坡隧道日志 '新加坡 SSH 隧道守护进程已停止。'
    写入新加坡隧道状态 -状态 'stopped' -消息 '新加坡 SSH 隧道守护进程已停止。'
    $守护互斥锁.ReleaseMutex()
    $守护互斥锁.Dispose()
}
