[CmdletBinding()]
param(
    # 发布版本 使用不带 v 前缀的 SemVer，例如 0.1.162-custom.1。
    [Parameter(Mandatory)]
    [ValidatePattern('^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$')]
    [string]$Version,

    # 镜像仓库 使用完整 registry/repository，不包含 tag 或 digest。
    [Parameter(Mandatory)]
    [string]$RegistryImage,

    # 用户 Git 远端 默认为 origin，必须是 GitHub HTTPS 或 SSH 地址。
    [string]$GitRemote = 'origin',

    # 源码分支 默认使用当前分支；发布前会将当前 HEAD 推送到此远端分支。
    [string]$Branch,

    # 目标平台 默认适配当前远端 Linux x86_64 部署。
    [string]$Platform = 'linux/amd64'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# 项目根目录 固定从 tools 目录向上定位，避免用户从其他目录执行时发布错误仓库。
$项目根目录 = Split-Path -Parent $PSScriptRoot
# 发布日志目录 保留 Git tag、commit、镜像引用和 digest，不记录任何 token 或 registry 密码。
$发布日志目录 = Join-Path $项目根目录 '.codex\log'
# 本次发布日志 使用时间戳避免覆盖此前的发布证据。
$发布日志 = Join-Path $发布日志目录 ("personal-image-publish-{0}.log" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))

# Write-PublishStep 统一输出和日志中的发布阶段名称。
function Write-PublishStep {
    param([string]$Message)

    Write-Host "`n[个人镜像发布] $Message" -ForegroundColor Cyan
}

# Invoke-NativeChecked 执行原生命令并把非零退出码转为可定位的 PowerShell 异常。
function Invoke-NativeChecked {
    param(
        [string]$Description,
        [scriptblock]$Command
    )

    Write-PublishStep $Description
    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "$Description 失败，退出码：$LASTEXITCODE"
    }
}

# Resolve-GitHubRepository 从 Git remote 解析 owner/repository，用于 OCI source 标签与发布记录。
function Resolve-GitHubRepository {
    param([string]$RemoteUrl)

    $匹配结果 = [regex]::Match($RemoteUrl.Trim(), 'github\.com[:/]([^/\s]+)/([^/\s]+?)(?:\.git)?$')
    if (-not $匹配结果.Success) {
        throw "Git 远端不是可识别的 GitHub 仓库：$RemoteUrl。请确认 $GitRemote 指向你的 GitHub fork。"
    }
    return "$($匹配结果.Groups[1].Value)/$($匹配结果.Groups[2].Value)"
}

# Normalize-RegistryImage 去除输入中的可变 tag，确保发布工具只管理仓库名称和本次明确 tag。
function Normalize-RegistryImage {
    param([string]$Image)

    $规范镜像 = $Image.Trim()
    if ([string]::IsNullOrWhiteSpace($规范镜像) -or $规范镜像.Contains('@') -or $规范镜像.Contains('://')) {
        throw 'RegistryImage 必须是 registry/repository，不能含 digest 或 URL 协议。'
    }
    $片段 = $规范镜像.Split('/')
    if ($片段.Length -lt 2 -or -not $片段[0].Contains('.')) {
        throw 'RegistryImage 必须包含 registry host，例如 ghcr.io/example/sub2api。'
    }
    $最后片段 = $片段[$片段.Length - 1]
    $冒号位置 = $最后片段.LastIndexOf(':')
    if ($冒号位置 -gt 0) {
        $片段[$片段.Length - 1] = $最后片段.Substring(0, $冒号位置)
        $规范镜像 = [string]::Join('/', $片段)
    }
    if ($规范镜像.Contains(' ') -or $规范镜像.EndsWith('/')) {
        throw 'RegistryImage 格式无效。'
    }
    return $规范镜像
}

New-Item -ItemType Directory -Force -Path $发布日志目录 | Out-Null
Start-Transcript -Path $发布日志 | Out-Null

try {
    Push-Location $项目根目录

    # 当前工作区状态 发布前禁止把未提交文件混入可回退的 Git tag 和镜像 digest。
    $当前工作区状态 = git status --porcelain
    if ($当前工作区状态) {
        throw '工作区存在未提交改动。请先提交全部本次变更后再发布，避免 Git tag 与镜像内容不一致。'
    }

    # 当前分支 未显式指定时以检出的分支为准，避免发布到错误分支。
    if ([string]::IsNullOrWhiteSpace($Branch)) {
        $Branch = (git branch --show-current).Trim()
    }
    if ([string]::IsNullOrWhiteSpace($Branch)) {
        throw '当前不是命名分支。请检出你的发布分支，或显式传入 -Branch。'
    }

    # 用户远端地址 用于验证该发布链路属于自己的 Git 仓库，而不是 upstream。
    $远端地址 = (git remote get-url $GitRemote).Trim()
    $GitHub仓库 = Resolve-GitHubRepository -RemoteUrl $远端地址
    $规范镜像仓库 = Normalize-RegistryImage -Image $RegistryImage
    $版本标签 = "v$Version"
    $完整提交 = (git rev-parse HEAD).Trim()
    $短提交 = (git rev-parse --short=12 HEAD).Trim()

    # 已有本地 tag 必须严格指向当前提交，避免覆盖历史回退点。
    $本地标签提交 = git rev-list -n 1 $版本标签 2>$null
    if ($LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace($本地标签提交)) {
        if ($本地标签提交.Trim() -ne $完整提交) {
            throw "本地标签 $版本标签 已指向 $($本地标签提交.Trim())，与当前提交 $完整提交 不一致。请使用新的 Version。"
        }
    } else {
        Invoke-NativeChecked "创建用户 Git 发布标签 $版本标签" { git tag -a $版本标签 -m "Personal image release $版本标签" $完整提交 }
    }

    # 远端 tag 同样必须指向当前提交；不使用 force，确保所有回退点永久可追溯。
    $远端标签对象 = git ls-remote --tags $GitRemote "refs/tags/$版本标签" "refs/tags/$版本标签^{}"
    if ($LASTEXITCODE -ne 0) {
        throw "读取远端标签失败。请检查 Git 远端权限：$GitRemote"
    }
    if ($远端标签对象) {
        $远端提交 = ($远端标签对象 | Select-String -Pattern "refs/tags/$([regex]::Escape($版本标签))\^\{\}$" | ForEach-Object { ($_ -split '\s+')[0] } | Select-Object -First 1)
        if ($远端提交 -and $远端提交.Trim() -ne $完整提交) {
            throw "远端标签 $版本标签 已指向 $($远端提交.Trim())，与当前提交不一致。发布已停止，历史回退点未被覆盖。"
        }
    }

    # 先推送源码提交，再推送 tag，满足远端服务器依据用户 Git tag 查找镜像的前提。
    Invoke-NativeChecked "推送当前提交到 $GitRemote/$Branch" { git push $GitRemote "HEAD:refs/heads/$Branch" }
    if (-not $远端标签对象) {
        Invoke-NativeChecked "推送用户 Git 发布标签 $版本标签" { git push $GitRemote "refs/tags/$版本标签" }
    }

    $版本镜像 = "${规范镜像仓库}:$版本标签"
    $提交镜像 = "${规范镜像仓库}:git-$短提交"
    $镜像来源标签 = "https://github.com/$GitHub仓库"

    # Buildx 直接推送不可变 tag；OCI revision/source 标签供服务端再次核验 Git commit。
    Invoke-NativeChecked "构建并推送个人镜像 $版本镜像" {
        docker buildx build `
            --platform $Platform `
            --build-arg "VERSION=$Version" `
            --build-arg "COMMIT=$完整提交" `
            --label "org.opencontainers.image.revision=$完整提交" `
            --label "org.opencontainers.image.source=$镜像来源标签" `
            --tag $版本镜像 `
            --tag $提交镜像 `
            --push `
            .
    }

    # 不可变镜像摘要 作为远端服务真实部署和回退的唯一镜像凭据。
    $镜像摘要 = (docker buildx imagetools inspect --format '{{.Manifest.Digest}}' $版本镜像).Trim()
    if ($LASTEXITCODE -ne 0 -or $镜像摘要 -notmatch '^sha256:[a-f0-9]{64}$') {
        throw "无法读取已推送镜像的 digest：$版本镜像。请确认 registry 返回 OCI manifest 后重试。"
    }

    # 发布证据 仅记录版本映射，不写入 GitHub/registry 凭据。
    $发布记录 = [ordered]@{
        tag       = $版本标签
        commit    = $完整提交
        image     = $版本镜像
        digest    = $镜像摘要
        reference = "$规范镜像仓库@$镜像摘要"
        published_at = (Get-Date).ToUniversalTime().ToString('o')
    } | ConvertTo-Json -Compress
    Write-Host "发布完成：$发布记录" -ForegroundColor Green
} finally {
    Pop-Location
    Stop-Transcript | Out-Null
    Write-Host "发布日志：$发布日志"
}
