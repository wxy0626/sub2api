# 隧道私钥路径：仅用于连接本人的 ECS。
$私钥路径 = Join-Path $env:USERPROFILE '.ssh\sub2api_proxy_tunnel'

# 日志目录：保留守护、断开和重连的排查信息。
$日志目录 = 'E:\AI\sun2api\.codex\log'
$标准输出日志 = Join-Path $日志目录 'sub2api-proxy-tunnel.out.log'
$标准错误日志 = Join-Path $日志目录 'sub2api-proxy-tunnel.err.log'
$状态日志 = Join-Path $日志目录 'sub2api-proxy-tunnel-launcher.log'
# 隧道状态文件：供 sub2api 启动命令确认守护和 SSH 转发的实际状态。
$状态文件 = Join-Path $日志目录 'sub2api-proxy-tunnel-state.json'

# 本机 Mihomo HTTP 代理监听地址。
$本地代理地址 = '127.0.0.1'
$本地代理端口 = 7897
# SSH 隧道掉线后的最小与最大重试间隔（秒）。
$最小重试间隔秒 = 5
$最大重试间隔秒 = 60

# SSH 参数：将本机 Mihomo 的 HTTP 代理安全地反向转发到 ECS Docker 私网网关。
$SSH参数 = @(
    '-i', $私钥路径,
    '-N',
    '-o', 'BatchMode=yes',
    '-o', 'ExitOnForwardFailure=yes',
    '-o', 'ServerAliveInterval=30',
    '-o', 'ServerAliveCountMax=3',
    '-o', 'TCPKeepAlive=yes',
    '-R', '172.17.0.1:17897:127.0.0.1:7897',
    'root@118.31.186.169'
)

# 写入隧道日志：统一记录守护进程状态，便于故障追踪。
function 写入隧道日志 {
    param([Parameter(Mandatory = $true)][string]$消息)

    Add-Content -LiteralPath $状态日志 -Value "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') $消息" -Encoding UTF8
}

# 写入隧道状态：状态文件只记录连接状态与可执行排查信息，不记录私钥或其他凭据。
function 写入隧道状态 {
    param(
        [Parameter(Mandatory = $true)][string]$状态,
        [Parameter(Mandatory = $true)][string]$消息,
        [Nullable[int]]$SSH进程ID = $null,
        [Nullable[int]]$SSH退出码 = $null
    )

    # 状态对象：主启动脚本以此判断隧道是否真正建立。
    $隧道状态对象 = [ordered]@{
        status        = $状态
        message       = $消息
        updated_at    = (Get-Date).ToString('o')
        guardian_pid  = $PID
        ssh_pid       = $SSH进程ID
        ssh_exit_code = $SSH退出码
    }
    $隧道状态对象 | ConvertTo-Json -Compress | Set-Content -LiteralPath $状态文件 -Encoding UTF8
}

# 获取现有隧道进程：防止重复创建同一反向端口。
function 获取现有隧道进程 {
    Get-CimInstance Win32_Process -Filter "Name = 'ssh.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -match '172\.17\.0\.1:17897:127\.0\.0\.1:7897' }
}

# 测试本地代理已监听：仅在 Mihomo 可用时建立 SSH 隧道。
function 测试本地代理已监听 {
    return [bool](Get-NetTCPConnection -LocalAddress $本地代理地址 -LocalPort $本地代理端口 -State Listen -ErrorAction SilentlyContinue)
}

New-Item -ItemType Directory -Force -Path $日志目录 | Out-Null

# 命名互斥锁：计划任务、手动启动和项目启动器同时触发时只允许一个守护进程工作。
$守护互斥锁名称 = 'Global\Sub2ApiProxyTunnelGuard'
# 互斥锁创建标记：PowerShell 7 传递 [ref] 前必须先初始化变量。
$已创建互斥锁 = $false
$守护互斥锁 = New-Object System.Threading.Mutex($false, $守护互斥锁名称, [ref]$已创建互斥锁)
if (-not $已创建互斥锁) {
    exit 0
}

$当前重试间隔秒 = $最小重试间隔秒
写入隧道日志 '隧道守护进程已启动。'
写入隧道状态 -状态 'starting' -消息 'SSH 隧道守护已启动，正在检查本机 Mihomo 与现有 SSH 转发。'

try {
    while ($true) {
        if (-not (测试本地代理已监听)) {
            $等待消息 = "本地 Mihomo $本地代理地址`:$本地代理端口 未监听，守护会在 $当前重试间隔秒 秒后复查。"
            写入隧道日志 $等待消息
            写入隧道状态 -状态 'waiting_for_local_proxy' -消息 $等待消息
            Start-Sleep -Seconds $当前重试间隔秒
            $当前重试间隔秒 = [Math]::Min($当前重试间隔秒 * 2, $最大重试间隔秒)
            continue
        }

        $已有隧道进程 = 获取现有隧道进程
        if ($已有隧道进程) {
            写入隧道状态 -状态 'connected' -消息 '已检测到现有 SSH 反向隧道进程。' -SSH进程ID $已有隧道进程[0].ProcessId
            Start-Sleep -Seconds 15
            $当前重试间隔秒 = $最小重试间隔秒
            continue
        }

        写入隧道日志 '本地代理已就绪，正在建立 SSH 反向隧道。'
        写入隧道状态 -状态 'connecting' -消息 '本地 Mihomo 已就绪，正在建立 SSH 反向隧道。'
        $SSH进程 = Start-Process -FilePath 'ssh.exe' -ArgumentList $SSH参数 -WindowStyle Hidden -RedirectStandardOutput $标准输出日志 -RedirectStandardError $标准错误日志 -PassThru
        Start-Sleep -Milliseconds 500
        if (-not $SSH进程.HasExited) {
            写入隧道日志 "SSH 反向隧道已建立，SSH 进程 ID $($SSH进程.Id)。"
            写入隧道状态 -状态 'connected' -消息 'SSH 反向隧道已建立。' -SSH进程ID $SSH进程.Id
        }
        $SSH进程.WaitForExit()
        $退出消息 = "SSH 反向隧道已退出，退出码 $($SSH进程.ExitCode)，守护会在 $当前重试间隔秒 秒后重连。"
        写入隧道日志 $退出消息
        写入隧道状态 -状态 'failed' -消息 $退出消息 -SSH进程ID $SSH进程.Id -SSH退出码 $SSH进程.ExitCode
        Start-Sleep -Seconds $当前重试间隔秒
        $当前重试间隔秒 = [Math]::Min($当前重试间隔秒 * 2, $最大重试间隔秒)
    }
} finally {
    写入隧道日志 '隧道守护进程已停止。'
    写入隧道状态 -状态 'stopped' -消息 'SSH 隧道守护进程已停止。'
    $守护互斥锁.ReleaseMutex()
    $守护互斥锁.Dispose()
}
