[CmdletBinding()]
param(
    # 更新上游源码：要求当前工作区没有未提交改动，避免覆盖本地定制。
    [switch]$UpdateSource,

    # 发布服务器：仅在本地测试、构建和健康检查全部成功后执行。
    [switch]$DeployServer,

    # 服务器地址：默认使用当前已部署的阿里云 ECS。
    [string]$ServerHost = '118.31.186.169',

    # SSH 登录用户：服务器上的 Docker 部署由 root 维护。
    [string]$ServerUser = 'root',

    # SSH 私钥路径：与现有代理隧道共用同一把 ECS 密钥。
    [string]$SshKeyPath = (Join-Path $env:USERPROFILE '.ssh\sub2api_proxy_tunnel'),

    # Docker 镜像标签：本地和服务器必须始终使用同一个标签。
    [string]$ImageTag = 'sub2api:test-model-whitelist'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# 项目根目录：由脚本所在的 tools 目录向上定位，避免依赖终端当前路径。
$projectRoot = Split-Path -Parent $PSScriptRoot
# 本地 Compose 文件：仅重建应用容器，保留本机数据库、Redis 和配置数据。
$localComposeFile = Join-Path $projectRoot 'deploy\docker-compose.local.yml'
# 日志目录：保留每一次更新的完整输出，方便出现异常时定位。
$logDirectory = Join-Path $projectRoot '.codex\log'
# 本次更新日志：时间戳避免覆盖此前记录。
$logFile = Join-Path $logDirectory ("sub2api-update-{0}.log" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
# 前端包管理器版本：必须与 Dockerfile 固定的 pnpm@9 保持一致，避免改写锁文件。
$pnpmVersion = 'pnpm@9'
# 本机临时镜像文件：用文件和 scp 传输二进制数据，避免 PowerShell 管道损坏 tar 内容。
$localImageArchive = Join-Path $env:TEMP ("sub2api-{0}.tar" -f ([Guid]::NewGuid().ToString('N')))
# 服务器临时镜像文件：镜像导入成功后由远端脚本删除。
$remoteImageArchive = "/tmp/sub2api-$([Guid]::NewGuid().ToString('N')).tar"

function Write-UpdateStep {
    param([string]$Message)

    Write-Host "`n[Sub2API 更新] $Message" -ForegroundColor Cyan
}

function Invoke-NativeCommand {
    param(
        [string]$Description,
        [scriptblock]$Command
    )

    Write-UpdateStep $Description
    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "$Description 失败，退出码：$LASTEXITCODE"
    }
}

function Wait-LocalHealth {
    # 本地健康检查截止时间：应用重建后最多等待 90 秒。
    $deadline = (Get-Date).AddSeconds(90)
    do {
        try {
            $response = Invoke-WebRequest -UseBasicParsing -TimeoutSec 5 'http://127.0.0.1:8080/health'
            if ($response.StatusCode -eq 200 -and $response.Content -match '"status"\s*:\s*"ok"') {
                Write-Host "本地健康检查通过：$($response.Content)" -ForegroundColor Green
                return
            }
        } catch {
            Write-Host "等待本地服务就绪：$($_.Exception.Message)"
        }
        Start-Sleep -Seconds 3
    } while ((Get-Date) -lt $deadline)

    throw '本地服务在 90 秒内未通过健康检查。请查看本次日志和 docker compose logs sub2api。'
}

function Invoke-RemoteDeploy {
    param(
        [string[]]$SshBaseArguments,
        [string]$SshDestination,
        [string]$ImageArchive,
        [string]$RemoteArchive
    )

    # 远端部署脚本：先备份 Compose，再只替换第一个 image 字段并重建 sub2api 服务。
    $remoteScript = @'
set -eu
image_tag="$1"
image_archive="$2"
compose_file="/opt/sub2api/docker-compose.yml"
backup_file="${compose_file}.before-$(date +%Y%m%d-%H%M%S)"

test -f "$compose_file"
test -f "$image_archive"
docker load --input "$image_archive"
rm -f "$image_archive"
cp "$compose_file" "$backup_file"
sed -i -E "0,/^([[:space:]]*image:[[:space:]]*).*/s//\\1${image_tag}/" "$compose_file"
docker compose -f "$compose_file" up -d --no-deps --force-recreate sub2api

for attempt in $(seq 1 30); do
  if curl -fsS --max-time 5 http://127.0.0.1:8080/health; then
    echo
    echo "服务器健康检查通过，Compose 备份：$backup_file"
    exit 0
  fi
  sleep 3
done

docker compose -f "$compose_file" ps sub2api
docker compose -f "$compose_file" logs --tail=100 sub2api
exit 1
'@

    Write-UpdateStep '传输已在本地验证的 Docker 镜像到服务器'
    & scp.exe @SshBaseArguments $ImageArchive "${SshDestination}:$RemoteArchive"
    if ($LASTEXITCODE -ne 0) {
        throw "服务器镜像传输失败，退出码：$LASTEXITCODE"
    }

    Write-UpdateStep '服务器重建 Sub2API 并执行健康检查'
    $remoteScript | & ssh.exe @SshBaseArguments $SshDestination "sh -s -- '$ImageTag' '$RemoteArchive'"
    if ($LASTEXITCODE -ne 0) {
        throw "服务器部署或健康检查失败，退出码：$LASTEXITCODE"
    }
}

New-Item -ItemType Directory -Force -Path $logDirectory | Out-Null
Start-Transcript -Path $logFile | Out-Null

try {
    Push-Location $projectRoot

    if (-not (Test-Path $localComposeFile)) {
        throw "找不到本地 Compose 文件：$localComposeFile"
    }
    if (-not ($ImageTag -match '^[A-Za-z0-9][A-Za-z0-9._/-]*(?::[A-Za-z0-9_][A-Za-z0-9_.-]*)?$')) {
        throw "镜像标签格式不合法：$ImageTag"
    }

    if ($UpdateSource) {
        $workingTreeState = git status --porcelain
        if ($workingTreeState) {
            throw '工作区存在未提交改动。请先提交或暂存本地定制后，再使用 -UpdateSource 更新上游源码。'
        }
        Invoke-NativeCommand '获取上游最新提交' { git fetch origin }
        Invoke-NativeCommand '将本地定制提交变基到 origin/main' { git rebase origin/main }
    }

    Invoke-NativeCommand '运行账号测试弹窗的定向单元测试' {
        corepack $pnpmVersion --dir frontend exec vitest run src/components/admin/account/__tests__/AccountTestModal.spec.ts
    }
    Invoke-NativeCommand '执行前端类型检查' { corepack $pnpmVersion --dir frontend run typecheck }
    Invoke-NativeCommand "构建定制镜像 $ImageTag" {
        docker build --build-arg COMMIT=local-test-model-whitelist --tag $ImageTag .
    }

    $env:SUB2API_IMAGE = $ImageTag
    Invoke-NativeCommand '本地重建 Sub2API 应用容器' {
        docker compose -f $localComposeFile up -d --no-deps --force-recreate sub2api
    }
    Wait-LocalHealth

    if ($DeployServer) {
        if (-not (Test-Path $SshKeyPath)) {
            throw "找不到 SSH 私钥：$SshKeyPath"
        }
        # SSH 参数：禁用交互式密码输入，防止自动部署停在不可见的提示上。
        $sshBaseArguments = @('-i', $SshKeyPath, '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes')
        $sshDestination = "$ServerUser@$ServerHost"
        Invoke-NativeCommand '导出已验证的 Docker 镜像' {
            docker save --output $localImageArchive $ImageTag
        }
        Invoke-RemoteDeploy -SshBaseArguments $sshBaseArguments -SshDestination $sshDestination -ImageArchive $localImageArchive -RemoteArchive $remoteImageArchive
    } else {
        Write-Host "`n本地更新已完成。确认网页功能后，使用以下命令发布服务器：" -ForegroundColor Yellow
        Write-Host '.\tools\update-sub2api-local-first.ps1 -DeployServer' -ForegroundColor Yellow
    }
} finally {
    Pop-Location
    if (Test-Path $localImageArchive) {
        Remove-Item -Force $localImageArchive
    }
    Stop-Transcript | Out-Null
    Write-Host "更新日志：$logFile"
}
