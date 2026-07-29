[CmdletBinding()]
param(
    # 发布版本使用不带 v 前缀的 SemVer，例如 0.1.163-local.1。
    [Parameter(Mandatory)]
    [ValidatePattern('^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$')]
    [string]$Version,

    # 镜像仓库使用完整 registry/repository，不包含 tag 或 digest。
    [Parameter(Mandatory)]
    [string]$RegistryImage,

    # GitRemote 必须指向个人 fork，默认 origin，绝不向 upstream 推送。
    [string]$GitRemote = 'origin',

    # Branch 留空时使用当前已检出的分支。
    [string]$Branch,

    # 平台默认与当前 Linux x86_64 部署一致。
    [string]$Platform = 'linux/amd64'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# 项目根目录由 tools 目录向上定位，避免从错误目录发布镜像。
$项目根目录 = Split-Path -Parent $PSScriptRoot
# 发布日志目录只记录镜像、标签和提交映射，不保存凭据。
$发布日志目录 = Join-Path $项目根目录 '.codex\log'
# 本次发布日志使用时间戳，便于追溯不可变镜像。
$发布日志 = Join-Path $发布日志目录 ("personal-image-publish-{0}.log" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))

# Write-PublishStep 输出可定位的发布阶段。
function Write-PublishStep {
    param([Parameter(Mandatory)][string]$Message)

    Write-Host "`n[个人镜像发布] $Message" -ForegroundColor Cyan
}

# Invoke-NativeChecked 将原生命令的失败转换为带阶段名称的异常。
function Invoke-NativeChecked {
    param(
        [Parameter(Mandatory)][string]$Description,
        [Parameter(Mandatory)][scriptblock]$Command
    )

    Write-PublishStep $Description
    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "$Description 失败，退出码：$LASTEXITCODE"
    }
}

# Resolve-GitHubRepository 从个人远端解析 OCI source 标签所需的 owner/repository。
function Resolve-GitHubRepository {
    param([Parameter(Mandatory)][string]$RemoteUrl)

    $匹配结果 = [regex]::Match($RemoteUrl.Trim(), 'github\.com[:/]([^/\s]+)/([^/\s]+?)(?:\.git)?$')
    if (-not $匹配结果.Success) {
        throw "Git 远端不是可识别的 GitHub 仓库：$GitRemote"
    }
    return "$($匹配结果.Groups[1].Value)/$($匹配结果.Groups[2].Value)"
}

# Normalize-RegistryImage 规范化 registry/repository，拒绝不稳定的 digest 或 tag 输入。
function Normalize-RegistryImage {
    param([Parameter(Mandatory)][string]$Image)

    $规范镜像 = $Image.Trim()
    if ([string]::IsNullOrWhiteSpace($规范镜像) -or $规范镜像.Contains('@') -or $规范镜像.Contains('://')) {
        throw 'RegistryImage 必须是 registry/repository，不能包含 digest 或 URL 协议。'
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
    return $规范镜像
}

New-Item -ItemType Directory -Force -Path $发布日志目录 | Out-Null
Start-Transcript -Path $发布日志 | Out-Null

try {
    Push-Location $项目根目录

    # 工作区必须干净，确保 Git tag、镜像和源码提交一一对应。
    if (git status --porcelain) {
        throw '工作区存在未提交改动。请先提交本次变更后再发布个人镜像。'
    }

    if ([string]::IsNullOrWhiteSpace($Branch)) {
        $Branch = (git branch --show-current).Trim()
    }
    if ([string]::IsNullOrWhiteSpace($Branch)) {
        throw '当前不是命名分支。请检出发布分支或显式指定 -Branch。'
    }

    # 个人远端校验：禁止向名为 upstream 的远端推送本地镜像发布标签。
    if ($GitRemote -eq 'upstream') {
        throw 'GitRemote 不能是 upstream；个人镜像只能发布到个人 fork。'
    }
    $远端地址 = (git remote get-url $GitRemote).Trim()
    $GitHub仓库 = Resolve-GitHubRepository -RemoteUrl $远端地址
    $规范镜像仓库 = Normalize-RegistryImage -Image $RegistryImage
    $版本标签 = "v$Version"
    $完整提交 = (git rev-parse HEAD).Trim()
    $短提交 = (git rev-parse --short=12 HEAD).Trim()

    # 发布前先推送当前个人分支，随后创建不可变版本 tag。
    Invoke-NativeChecked "推送个人分支 $GitRemote/$Branch" { git push $GitRemote "HEAD:refs/heads/$Branch" }
    $已有标签提交 = (git rev-list -n 1 $版本标签 2>$null).Trim()
    if ($已有标签提交 -and $已有标签提交 -ne $完整提交) {
        throw "标签 $版本标签 已指向其他提交：$已有标签提交"
    }
    if (-not $已有标签提交) {
        Invoke-NativeChecked "创建个人发布标签 $版本标签" { git tag -a $版本标签 -m "Personal image release $版本标签" $完整提交 }
        Invoke-NativeChecked "推送个人发布标签 $版本标签" { git push $GitRemote "refs/tags/$版本标签" }
    }

    # OCI 标签用于在运行镜像中追溯个人 fork 与准确源码提交。
    $版本镜像 = "${规范镜像仓库}:$版本标签"
    $提交镜像 = "${规范镜像仓库}:git-$短提交"
    $镜像来源标签 = "https://github.com/$GitHub仓库"
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

    $镜像摘要 = (docker buildx imagetools inspect --format '{{.Manifest.Digest}}' $版本镜像).Trim()
    if ($镜像摘要 -notmatch '^sha256:[a-f0-9]{64}$') {
        throw "无法读取已推送个人镜像摘要：$版本镜像"
    }
    [PSCustomObject]@{
        tag       = $版本标签
        commit    = $完整提交
        image     = $版本镜像
        digest    = $镜像摘要
        reference = "$规范镜像仓库@$镜像摘要"
    } | ConvertTo-Json -Compress | Write-Host
} finally {
    Pop-Location
    Stop-Transcript | Out-Null
    Write-Host "发布日志：$发布日志"
}
