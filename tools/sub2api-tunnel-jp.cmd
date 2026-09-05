@echo off
setlocal EnableExtensions
chcp 65001 >nul

:: 日本 SSH 反向隧道守护：把本机 Mihomo(7897) 反向转发到 ECS Docker 私网网关。
set "LOGDIR=E:\AI\sun2api\.codex\log"
set "STATE=%LOGDIR%\sub2api-proxy-tunnel-state.json"
set "LOCK=%LOGDIR%\sub2api-proxy-tunnel-jp.lock"
set "KEY=%USERPROFILE%\.ssh\sub2api_proxy_tunnel"
set "LOCALPORT=7897"
set "FORWARD=172.17.0.1:17897:127.0.0.1:7897"
set "HOST=root@118.31.186.169"
set "ERR=%LOGDIR%\sub2api-proxy-tunnel.err.log"

:: 单实例：锁文件存在说明已有守护在跑
if exist "%LOCK%" exit /b 0
echo %RANDOM% > "%LOCK%"

:loop
:: 本机 Mihomo 未监听则等待（不影响日本隧道建立）
netstat -an | findstr /R "127.0.0.1:%LOCALPORT% " >nul
if errorlevel 1 (
  call :state waiting "本地 Mihomo 127.0.0.1:%LOCALPORT% 未监听，守护将在 5 秒后复查"
  ping -n 1 -w 5000 127.0.0.1 >nul
  goto loop
)
:: 已有同转发串的 ssh 进程则视为已连接
wmic process where "name='ssh.exe' and commandline like '%%%FORWARD%%%'" get ProcessId 2>nul | findstr /r "[0-9]" >nul
if not errorlevel 1 (
  call :state connected "已检测到现有 SSH 反向隧道进程"
  ping -n 1 -w 15000 127.0.0.1 >nul
  goto loop
)
call :state connecting "本地 Mihomo 已就绪，正在建立 SSH 反向隧道"
start "" /min ssh.exe -i "%KEY%" -N -o BatchMode=yes -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 -o ServerAliveCountMax=3 -o TCPKeepAlive=yes -R %FORWARD% %HOST%
ping -n 1 -w 1500 127.0.0.1 >nul
wmic process where "name='ssh.exe' and commandline like '%%%FORWARD%%%'" get ProcessId 2>nul | findstr /r "[0-9]" >nul
if not errorlevel 1 call :state connected "SSH 反向隧道已建立"
:wait
wmic process where "name='ssh.exe' and commandline like '%%%FORWARD%%%'" get ProcessId 2>nul | findstr /r "[0-9]" >nul
if not errorlevel 1 (
  ping -n 1 -w 5000 127.0.0.1 >nul
  goto wait
)
call :state failed "SSH 反向隧道已退出，守护将在 5 秒后重连"
ping -n 1 -w 5000 127.0.0.1 >nul
goto loop

:state
echo {"status":"%~1","message":"%~2","updated_at":"%DATE% %TIME%"} > "%STATE%"
goto :eof
