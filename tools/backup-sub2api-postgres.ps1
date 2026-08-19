# =============================================================================
# sub2api-postgres 定时逻辑备份脚本 (pg_dump)
# 用途: 每日备份 sub2api 数据库, 防止 WAL/数据损坏后无法恢复。
# 产物: E:\AI\sun2api\.workbuddy\backups\sub2api_YYYYMMDD_HHMM.dump (保留最近 30 份)
# 用法: pwsh -NoProfile -ExecutionPolicy Bypass -File tools\backup-sub2api-postgres.ps1
# =============================================================================
$ErrorActionPreference = 'Stop'

# Docker CLI 完整路径 (本机 Docker Desktop 未加入 PATH)
$DockerCli = 'C:\Users\Administrator\AppData\Local\Programs\DockerDesktop\resources\bin\docker.exe'
# 备份输出目录 (项目内 .workbuddy, 按内容分类)
$BackupDir = 'E:\AI\sun2api\.workbuddy\backups'
# 保留份数
$KeepCount = 30

# Postgres 容器与凭据
$Container = 'sub2api-postgres'
$DbUser = 'sub2api'
$DbName = 'sub2api'

if (-not (Test-Path -LiteralPath $BackupDir -PathType Container)) {
    New-Item -ItemType Directory -Path $BackupDir -Force | Out-Null
}

$Timestamp = Get-Date -Format 'yyyyMMdd_HHmm'
$TmpFile = "/tmp/sub2api_$Timestamp.dump"
$TargetFile = Join-Path $BackupDir "sub2api_$Timestamp.dump"

# 1. 容器内生成备份
& $DockerCli exec $Container pg_dump -U $DbUser -d $DbName -Fc -f $TmpFile
if ($LASTEXITCODE -ne 0) {
    throw "pg_dump failed, exit code $LASTEXITCODE. Check container $Container is running."
}

# 2. 复制到宿主机备份目录
& $DockerCli cp "${Container}:${TmpFile}" $TargetFile
if ($LASTEXITCODE -ne 0) {
    throw "docker cp failed, exit code $LASTEXITCODE."
}

# 3. 清理容器内临时文件
& $DockerCli exec $Container rm -f $TmpFile 2>$null

# 4. 验证备份文件头 (PGDMP 魔数)
$HeaderBytes = [System.IO.File]::ReadAllBytes($TargetFile)[0..4]
$Magic = [System.Text.Encoding]::ASCII.GetString($HeaderBytes)
if ($Magic -ne 'PGDMP') {
    throw "Backup file invalid: $TargetFile is not a valid pg_dump file (magic $Magic)."
}

# 5. 清理过期备份 (按文件名时间戳保留最近 N 份)
Get-ChildItem -LiteralPath $BackupDir -Filter 'sub2api_*.dump' |
    Sort-Object Name -Descending |
    Select-Object -Skip $KeepCount |
    ForEach-Object {
        Write-Warning "[backup] removing old backup: $($_.Name)"
        Remove-Item -LiteralPath $_.FullName -Force
    }

$SizeMB = [math]::Round((Get-Item -LiteralPath $TargetFile).Length / 1MB, 1)
Write-Output "[backup] done: $TargetFile ($SizeMB MB)"
