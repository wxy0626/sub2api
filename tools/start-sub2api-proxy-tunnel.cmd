@echo off
REM Start the UTF-8 PowerShell tunnel launcher with PowerShell 7.
start "Sub2API Proxy Tunnel" /min pwsh.exe -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "E:\AI\sun2api\tools\start-sub2api-proxy-tunnel.ps1"
