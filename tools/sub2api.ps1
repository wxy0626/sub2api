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
# SSH 隧道状态文件：由守护进程写入，供启动命令确认实际连接结果。
$SSH隧道状态文件 = 'E:\AI\sun2api\.codex\log\sub2api-proxy-tunnel-state.json'
# SSH 隧道详细日志：连接失败时直接提示该文件，避免只输出笼统错误。
$SSH隧道错误日志 = 'E:\AI\sun2api\.codex\log\sub2api-proxy-tunnel.err.log'
# SSH 隧道状态日志：记录守护重连和本地代理未监听等排查信息。
$SSH隧道状态日志 = 'E:\AI\sun2api\.codex\log\sub2api-proxy-tunnel-launcher.log'
# 本地 Mihomo HTTP 代理端口：仅用于区分等待代理与 SSH 建连失败。
$本地代理端口 = 7897
# 新加坡专用 Mihomo 启动器：独立实例固定选用已验证的新加坡节点。
$新加坡代理启动脚本 = 'E:\AI\sun2api\tools\start-sub2api-singapore-proxy.ps1'
# 新加坡 SSH 隧道守护脚本：仅建立远端 17898 到本机 17998 的专用转发。
$新加坡SSH隧道守护脚本 = 'E:\AI\sun2api\tools\start-sub2api-singapore-proxy-tunnel.ps1'
# 新加坡 SSH 隧道状态文件：用于确认第二条隧道已独立建立。
$新加坡SSH隧道状态文件 = 'E:\AI\sun2api\.codex\log\sub2api-singapore-proxy-tunnel-state.json'
# 新加坡 SSH 隧道错误日志：连接失败时保留 SSH 原始技术详情。
$新加坡SSH隧道错误日志 = 'E:\AI\sun2api\.codex\log\sub2api-singapore-proxy-tunnel.err.log'
# 新加坡 SSH 隧道状态日志：记录守护重连与本地专用代理状态。
$新加坡SSH隧道状态日志 = 'E:\AI\sun2api\.codex\log\sub2api-singapore-proxy-tunnel-launcher.log'
# 新加坡专用本地 HTTP 代理端口。
$新加坡本地代理端口 = 17998
# 新加坡代理登记脚本：只在专用代理和 SSH 隧道确认后写入本地 Sub2API 代理列表。
$新加坡代理登记脚本 = 'E:\AI\sun2api\tools\register-sub2api-local-singapore-proxy.ps1'

function 测试Docker引擎就绪 {
    docker info *> $null
    return $LASTEXITCODE -eq 0
}

# 获取 Compose 解析后的应用镜像：使用实际 .env 展开结果，避免脚本与 Compose 的变量规则不一致。
function 获取Sub2API应用镜像 {
    # Compose 配置原始输出：保留解析失败时的技术详情，便于定位 .env 或 Compose 配置问题。
    $Compose配置原始输出 = & docker compose --env-file .env -f docker-compose.local.yml config --format json 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "无法解析本地 Docker Compose 配置。请检查 $部署目录\.env 和 docker-compose.local.yml。原始技术详情：$($Compose配置原始输出 -join [Environment]::NewLine)"
    }

    try {
        # Compose 配置对象：仅读取 sub2api 服务最终生效的镜像名称。
        $Compose配置对象 = ($Compose配置原始输出 -join [Environment]::NewLine) | ConvertFrom-Json -ErrorAction Stop
        # 应用镜像名称：已经包含 SUB2API_IMAGE 的展开结果。
        $应用镜像名称 = [string]$Compose配置对象.services.sub2api.image
    } catch {
        throw "Docker Compose 配置无法解析为 JSON。请确认 Docker Compose 版本支持 'config --format json'。原始技术详情：$($_.Exception.Message)"
    }

    if ([string]::IsNullOrWhiteSpace($应用镜像名称)) {
        throw 'Docker Compose 未解析出 sub2api 服务的 image。请在 docker-compose.local.yml 或 .env 中设置 SUB2API_IMAGE。'
    }

    return $应用镜像名称
}

# 确认本地定制镜像存在：阻止 Compose 将无仓库路径的本地标签误当作远程仓库拉取。
function 确认本地Sub2API镜像可用 {
    # 应用镜像名称：从 Compose 最终配置读取，不自行覆盖用户的 .env 设置。
    $应用镜像名称 = 获取Sub2API应用镜像

    # 含路径分隔符的镜像属于仓库或注册表引用，保留 Compose 原有拉取行为。
    if ($应用镜像名称.Contains('/')) {
        return
    }

    docker image inspect $应用镜像名称 *> $null
    if ($LASTEXITCODE -eq 0) {
        return
    }

    # 本机可用的 Sub2API 标签：只列出诊断信息，不自动切换到可能包含不同代码的镜像。
    $本机Sub2API镜像标签 = @(docker image ls --format '{{.Repository}}:{{.Tag}}' | Where-Object { $_ -like 'sub2api:*' -and $_ -notlike '*:<none>' })
    # 本机镜像标签说明：为空时明确提示没有可供选择的本地定制镜像。
    $本机镜像标签说明 = if ($本机Sub2API镜像标签.Count -gt 0) { $本机Sub2API镜像标签 -join '、' } else { '未发现任何 sub2api 本地镜像标签' }
    throw "本地定制镜像不存在：$应用镜像名称。Docker Compose 会将无仓库路径的镜像名当作远程仓库拉取，因此出现 repository does not exist。当前可用的本机标签：$本机镜像标签说明。请二选一：1. 在 $部署目录\.env 中将 SUB2API_IMAGE 明确改为要使用的现有标签；2. 在项目根目录执行 'docker build -t $应用镜像名称 .' 生成该指定标签后重试。脚本不会自动替换镜像，以免启动未经确认的代码版本。"
}

# 启动 SSH 隧道守护：守护脚本通过互斥锁保证重复执行 sub2api 时不会创建重复进程。
function 启动SSH隧道守护 {
    if (-not (Test-Path -LiteralPath $SSH隧道守护脚本 -PathType Leaf)) {
        throw "未找到 SSH 隧道守护脚本：$SSH隧道守护脚本"
    }

    return Start-Process -FilePath 'pwsh.exe' -ArgumentList @(
        '-NoProfile',
        '-ExecutionPolicy', 'Bypass',
        '-WindowStyle', 'Hidden',
        '-File', $SSH隧道守护脚本
    ) -WindowStyle Hidden -PassThru -ErrorAction Stop
}

# 启动新加坡专用 Mihomo：此脚本不会改变 Clash Verge 全局实例或日本节点选择。
function 启动新加坡专用代理 {
    if (-not (Test-Path -LiteralPath $新加坡代理启动脚本 -PathType Leaf)) {
        throw "未找到新加坡专用代理启动脚本：$新加坡代理启动脚本"
    }

    & pwsh.exe -NoProfile -ExecutionPolicy Bypass -File $新加坡代理启动脚本
    if ($LASTEXITCODE -ne 0) {
        throw "新加坡专用代理启动或 ChatGPT TLS 验证失败。请查看 .codex\\log\\sub2api-singapore-proxy-probe.log 和 .codex\\log\\sub2api-singapore-mihomo.err.log。"
    }
}

# 启动新加坡 SSH 隧道守护：独立进程与独立端口确保日本隧道不受影响。
function 启动新加坡SSH隧道守护 {
    if (-not (Test-Path -LiteralPath $新加坡SSH隧道守护脚本 -PathType Leaf)) {
        throw "未找到新加坡 SSH 隧道守护脚本：$新加坡SSH隧道守护脚本"
    }

    return Start-Process -FilePath 'pwsh.exe' -ArgumentList @(
        '-NoProfile',
        '-ExecutionPolicy', 'Bypass',
        '-WindowStyle', 'Hidden',
        '-File', $新加坡SSH隧道守护脚本
    ) -WindowStyle Hidden -PassThru -ErrorAction Stop
}

# 登记本地新加坡代理：脚本会先从应用容器验证代理连通性，再幂等写入代理记录。
function 登记本地新加坡代理 {
    if (-not (Test-Path -LiteralPath $新加坡代理登记脚本 -PathType Leaf)) {
        throw "未找到新加坡代理登记脚本：$新加坡代理登记脚本"
    }

    & pwsh.exe -NoProfile -ExecutionPolicy Bypass -File $新加坡代理登记脚本
    if ($LASTEXITCODE -ne 0) {
        throw '新加坡专用代理和 SSH 隧道已建立，但未能安全登记到本地账号代理列表。请查看 .codex\\log 和 sub2api-postgres 容器日志。'
    }
}

# 测试本地代理端口：用于在隧道尚未建立时给出准确的中文状态说明。
function 测试本地代理已监听 {
    return [bool](Get-NetTCPConnection -LocalAddress '127.0.0.1' -LocalPort $本地代理端口 -State Listen -ErrorAction SilentlyContinue)
}

# 读取隧道状态：守护写入文件的瞬间可能被读取，解析失败时视为尚未产生可用状态。
function 获取SSH隧道状态 {
    if (-not (Test-Path -LiteralPath $SSH隧道状态文件 -PathType Leaf)) {
        return $null
    }

    try {
        return Get-Content -LiteralPath $SSH隧道状态文件 -Raw -ErrorAction Stop | ConvertFrom-Json -ErrorAction Stop
    } catch {
        return $null
    }
}

# 获取指定状态文件的隧道状态：供日本和新加坡两条隧道复用相同确认流程。
function 获取指定SSH隧道状态 {
    param([Parameter(Mandatory = $true)][string]$状态文件路径)

    if (-not (Test-Path -LiteralPath $状态文件路径 -PathType Leaf)) {
        return $null
    }

    try {
        return Get-Content -LiteralPath $状态文件路径 -Raw -ErrorAction Stop | ConvertFrom-Json -ErrorAction Stop
    } catch {
        return $null
    }
}

# 测试 SSH 隧道进程：只接受携带目标反向转发参数且仍存活的 ssh.exe 进程。
function 测试SSH隧道进程已连接 {
    return [bool](Get-CimInstance Win32_Process -Filter "Name = 'ssh.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -match '172\.17\.0\.1:17897:127\.0\.0\.1:7897' })
}

# 测试新加坡 SSH 隧道进程：只匹配 17898 到 17998 的专用转发。
function 测试新加坡SSH隧道进程已连接 {
    return [bool](Get-CimInstance Win32_Process -Filter "Name = 'ssh.exe'" -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -match '172\.17\.0\.1:17898:127\.0\.0\.1:17998' })
}

# 确认 SSH 隧道结果：本地代理可用时必须等到 SSH 进程和守护状态同时确认，否则以失败结束。
function 确认SSH隧道结果 {
    param([Parameter(Mandatory = $true)][System.Diagnostics.Process]$守护进程)

    # 隧道确认超时秒数：足够覆盖本机进程创建及 SSH 初始认证失败。
    $隧道确认超时秒数 = 15
    $截止时间 = (Get-Date).AddSeconds($隧道确认超时秒数)

    while ((Get-Date) -lt $截止时间) {
        $隧道状态 = 获取SSH隧道状态
        $本地代理已监听 = 测试本地代理已监听
        $SSH隧道已连接 = 测试SSH隧道进程已连接

        if ($隧道状态 -and $隧道状态.status -eq 'connected' -and $SSH隧道已连接) {
            Write-Output "[sub2api] SSH 反向隧道已连接：172.17.0.1:17897 -> 127.0.0.1:$本地代理端口"
            return
        }

        if (-not $本地代理已监听) {
            Write-Warning "[sub2api] 本机 Mihomo 127.0.0.1:$本地代理端口 未监听，SSH 隧道守护将持续等待；本次不宣称隧道已连接。状态日志：$SSH隧道状态日志"
            return
        }

        if ($隧道状态 -and $隧道状态.status -eq 'failed') {
            throw "SSH 反向隧道连接失败：$($隧道状态.message) 请检查 SSH 私钥、ECS 118.31.186.169 的 sshd 反向转发权限及端口占用。状态日志：$SSH隧道状态日志；SSH 错误日志：$SSH隧道错误日志"
        }

        if ($守护进程.HasExited) {
            throw "SSH 隧道守护进程已提前退出，未建立反向隧道。请检查守护启动权限和状态日志：$SSH隧道状态日志；SSH 错误日志：$SSH隧道错误日志"
        }

        Start-Sleep -Milliseconds 500
    }

    throw "本机 Mihomo 127.0.0.1:$本地代理端口 已监听，但 SSH 反向隧道在 $隧道确认超时秒数 秒内未建立。请检查 SSH 私钥、ECS 118.31.186.169 的 sshd 反向转发权限及端口占用。状态日志：$SSH隧道状态日志；SSH 错误日志：$SSH隧道错误日志"
}

# 确认新加坡 SSH 隧道结果：只有专用代理、守护状态和 SSH 进程均就绪才报告连接成功。
function 确认新加坡SSH隧道结果 {
    param([Parameter(Mandatory = $true)][System.Diagnostics.Process]$守护进程)

    $隧道确认超时秒数 = 15
    $截止时间 = (Get-Date).AddSeconds($隧道确认超时秒数)
    while ((Get-Date) -lt $截止时间) {
        $隧道状态 = 获取指定SSH隧道状态 -状态文件路径 $新加坡SSH隧道状态文件
        # 短 TCP 探针：避免 Windows 网络枚举在启动路径阻塞。
        $新加坡代理客户端 = [System.Net.Sockets.TcpClient]::new()
        try {
            $新加坡代理异步连接 = $新加坡代理客户端.BeginConnect('127.0.0.1', $新加坡本地代理端口, $null, $null)
            $本地代理已监听 = $新加坡代理异步连接.AsyncWaitHandle.WaitOne(1000)
            if ($本地代理已监听) {
                $新加坡代理客户端.EndConnect($新加坡代理异步连接)
            }
        } catch {
            $本地代理已监听 = $false
        } finally {
            $新加坡代理客户端.Dispose()
        }
        $SSH隧道已连接 = 测试新加坡SSH隧道进程已连接

        if ($隧道状态 -and $隧道状态.status -eq 'connected' -and $SSH隧道已连接) {
            Write-Output "[sub2api] 新加坡 SSH 反向隧道已连接：172.17.0.1:17898 -> 127.0.0.1:$新加坡本地代理端口"
            return
        }
        if (-not $本地代理已监听) {
            throw "新加坡专用代理 127.0.0.1:$新加坡本地代理端口 未监听。请查看 .codex\\log\\sub2api-singapore-proxy-probe.log。"
        }
        if ($隧道状态 -and $隧道状态.status -eq 'failed') {
            throw "新加坡 SSH 反向隧道连接失败：$($隧道状态.message) 请检查 $新加坡SSH隧道状态日志 和 $新加坡SSH隧道错误日志。"
        }
        if ($守护进程.HasExited) {
            throw "新加坡 SSH 隧道守护进程已提前退出。请检查 $新加坡SSH隧道状态日志 和 $新加坡SSH隧道错误日志。"
        }
        Start-Sleep -Milliseconds 500
    }

    throw "新加坡专用代理已监听，但 SSH 反向隧道在 $隧道确认超时秒数 秒内未建立。请检查 $新加坡SSH隧道状态日志 和 $新加坡SSH隧道错误日志。"
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
    确认本地Sub2API镜像可用
    Write-Output '[sub2api] Starting containers...'
    docker compose --env-file .env -f docker-compose.local.yml up -d
    if ($LASTEXITCODE -ne 0) {
        throw "容器启动失败，Docker Compose 退出码：$LASTEXITCODE。请执行 'docker compose --env-file .env -f docker-compose.local.yml ps' 和 'docker compose --env-file .env -f docker-compose.local.yml logs --tail=100 sub2api' 查看具体原因。"
    }
} finally {
    Pop-Location
}

Write-Output '[sub2api] Starting remote proxy tunnel guardian...'
$SSH隧道守护进程 = 启动SSH隧道守护
确认SSH隧道结果 -守护进程 $SSH隧道守护进程

Write-Output '[sub2api] Starting Singapore dedicated proxy...'
启动新加坡专用代理
Write-Output '[sub2api] Starting Singapore proxy tunnel guardian...'
$新加坡SSH隧道守护进程 = 启动新加坡SSH隧道守护
确认新加坡SSH隧道结果 -守护进程 $新加坡SSH隧道守护进程
Write-Output '[sub2api] Registering Singapore proxy in the local account list...'
登记本地新加坡代理

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
