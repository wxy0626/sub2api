# Sub2API Compose 部署目录。
$部署目录 = 'E:\AI\sun2api\deploy'
# Docker Desktop 程序路径。
$Docker桌面程序 = 'C:\Program Files\Docker\Docker\Docker Desktop.exe'
# 本地服务健康检查地址。
$健康检查地址 = 'http://127.0.0.1:8080/health'
# 本地管理页面地址。
$管理页面地址 = 'http://127.0.0.1:8080'
# SSH 隧道守护脚本：随本地服务启动，并持续维持远端反向代理隧道。
$SSH隧道守护脚本 = 'E:\AI\sun2api\tools\start-sub2api-proxy-tunnel.ps1'

function 测试Docker引擎就绪 {
    docker info *> $null
    return $LASTEXITCODE -eq 0
}

# 启动 SSH 隧道守护：守护脚本通过互斥锁保证重复执行 sub2api 时不会创建重复进程。
function 启动SSH隧道守护 {
    if (-not (Test-Path -LiteralPath $SSH隧道守护脚本 -PathType Leaf)) {
        throw "未找到 SSH 隧道守护脚本：$SSH隧道守护脚本"
    }

    Start-Process -FilePath 'pwsh.exe' -ArgumentList @(
        '-NoProfile',
        '-ExecutionPolicy', 'Bypass',
        '-WindowStyle', 'Hidden',
        '-File', $SSH隧道守护脚本
    ) -WindowStyle Hidden
}

Write-Output '[sub2api] Checking Docker Desktop...'
if (-not (测试Docker引擎就绪)) {
    Write-Output '[sub2api] Starting Docker Desktop...'
    Start-Process -FilePath $Docker桌面程序 -WindowStyle Hidden

    $Docker等待次数 = 45
    for ($序号 = 1; $序号 -le $Docker等待次数; $序号++) {
        if (测试Docker引擎就绪) {
            break
        }
        Write-Output "[sub2api] Waiting for Docker engine ($序号/$Docker等待次数)..."
        Start-Sleep -Seconds 2
    }
    if (-not (测试Docker引擎就绪)) {
        Write-Error '[sub2api] Docker Desktop did not become ready within 90 seconds.'
        exit 1
    }
}

Push-Location $部署目录
try {
    Write-Output '[sub2api] Starting containers...'
    docker compose --env-file .env -f docker-compose.local.yml up -d
    if ($LASTEXITCODE -ne 0) {
        throw 'Failed to start containers.'
    }
} finally {
    Pop-Location
}

Write-Output '[sub2api] Starting remote proxy tunnel guardian...'
启动SSH隧道守护

# 端口健康检查最大轮次。
$健康检查次数 = 30
for ($序号 = 1; $序号 -le $健康检查次数; $序号++) {
    try {
        $响应 = Invoke-WebRequest -Uri $健康检查地址 -UseBasicParsing -TimeoutSec 3
                             if ($响应.StatusCode -eq 200) {
                                 Write-Output "[sub2api] Ready: $管理页面地址"
                                 Start-Process $管理页面地址
                                 exit 0
        }
    } catch {
        # 服务正在启动，继续等待。
    }
    Write-Output "[sub2api] Waiting for port 8080 ($序号/$健康检查次数)..."
    Start-Sleep -Seconds 2
}

Write-Error '[sub2api] Port 8080 did not pass the health check.'
exit 1
