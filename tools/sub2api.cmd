@echo off
setlocal EnableExtensions EnableDelayedExpansion
chcp 65001 >nul

:: ===== 基础路径与配置（项目内固定，按需修改）=====
set "ROOT=E:\AI\sun2api"
set "TOOLS=%ROOT%\tools"
set "DEPLOY=%ROOT%\deploy"
set "LOGDIR=%ROOT%\.codex\log"
set "HEALTH=http://127.0.0.1:8080/health"
set "ADMIN=http://127.0.0.1:8080"
set "DOCKERAPP=C:\Users\Administrator\AppData\Local\Programs\DockerDesktop\Docker Desktop.exe"
set "SGCONF=%ROOT%\.codex\runtime\sub2api-singapore-mihomo.yaml"
set "CVHOME=%APPDATA%\io.github.clash-verge-rev.clash-verge-rev"
set "MIHOMO=D:\VPN\Clash Verge\verge-mihomo.exe"
set "SGPORT=17998"

:: 确保日志目录存在
if not exist "%LOGDIR%" mkdir "%LOGDIR%"

echo [sub2api] 检查 Docker Desktop 是否已就绪...
docker info >nul 2>&1
if errorlevel 1 (
  echo [sub2api] 未检测到 Docker 引擎，正在启动 Docker Desktop...
  if exist "%DOCKERAPP%" ( start "" "%DOCKERAPP%" ) else ( echo [sub2api] 错误：未找到 Docker Desktop 程序：%DOCKERAPP% & exit /b 1 )
  set "N=0"
  :dockerwait
  set /a N+=1
  if %N% gtr 45 (
    echo [sub2api] 错误：Docker Desktop 在 90 秒内仍未就绪。请手动启动 Docker Desktop 后重试。
    exit /b 1
  )
  echo [sub2api] 等待 Docker 引擎就绪 (%N%/45)...
  ping -n 1 -w 2000 127.0.0.1 >nul
  docker info >nul 2>&1
  if errorlevel 1 goto dockerwait
)

echo [sub2api] 检查本地定制镜像是否存在...
call :getenv SUB2API_IMAGE IMG
if not defined IMG (
  echo [sub2api] 错误：未在 %DEPLOY%\.env 中找到 SUB2API_IMAGE，无法继续。
  exit /b 1
)
echo !IMG! | findstr /R "/" >nul
if errorlevel 1 (
  docker image inspect !IMG! >nul 2>&1
  if errorlevel 1 (
    echo [sub2api] 错误：本地定制镜像不存在：!IMG!。请先在项目根目录执行 "docker build -t !IMG! ." 生成该标签，或在 .env 中将 SUB2API_IMAGE 改为已存在的镜像标签。脚本不会自动替换镜像，以免启动未经确认的代码版本。
    exit /b 1
  )
)

echo [sub2api] 使用 Docker Compose 启动容器...
pushd "%DEPLOY%"
docker compose --env-file .env -f docker-compose.local.yml up -d
if errorlevel 1 (
  echo [sub2api] 错误：容器启动失败，Docker Compose 退出码 %ERRORLEVEL%。请执行 "docker compose --env-file .env -f docker-compose.local.yml ps" 与 "docker compose --env-file .env -f docker-compose.local.yml logs --tail=100 sub2api" 查看具体原因。
  popd
  exit /b 1
)
popd

echo [sub2api] 启动日本 SSH 反向隧道守护（后台）...
call :startguard jp
echo [sub2api] 启动新加坡专用代理 Mihomo（后台）...
call :startsgproxy
if errorlevel 1 exit /b 1
echo [sub2api] 启动新加坡 SSH 反向隧道守护（后台）...
call :startguard sg

echo [sub2api] 登记本地新加坡代理到账号列表...
call :register_sg_proxy
if errorlevel 1 (
  echo [sub2api] 错误：新加坡专用代理与 SSH 隧道已建立，但未能安全登记到本地账号代理列表。请查看 %LOGDIR%\sub2api-local-singapore-proxy-registration.log 与容器代理探测日志。
  exit /b 1
)

echo [sub2api] 等待服务端口 8080 健康检查...
set "N=0"
:health
set /a N+=1
if %N% gtr 30 (
  echo [sub2api] 错误：端口 8080 在 60 秒内未通过健康检查，服务可能未正常启动。
  exit /b 1
)
curl.exe -s -o NUL -w "%%{http_code}" %HEALTH% 2>nul | findstr /R "^200$" >nul
if not errorlevel 1 (
  echo [sub2api] 就绪：%ADMIN%
  start "" "%ADMIN%"
  exit /b 0
)
echo [sub2api] 等待端口 8080 (%N%/30)...
ping -n 1 -w 2000 127.0.0.1 >nul
goto health

:: ===== 子程序：从 .env 读取变量 =====
:getenv
set "%~2="
for /f "usebackq tokens=1,* delims==" %%a in ("%DEPLOY%\.env") do (
  if /i "%%a"=="%~1" set "%~2=%%b"
)
goto :eof

:: ===== 子程序：启动指定隧道守护（jp/sg）=====
:startguard
set "G=%~1"
if "!G!"=="jp" goto guard_jp
goto guard_sg

:guard_jp
set "LOCK=%LOGDIR%\sub2api-proxy-tunnel-jp.lock"
set "FORWARD=172.17.0.1:17897:127.0.0.1:7897"
goto guard_run

:guard_sg
set "LOCK=%LOGDIR%\sub2api-proxy-tunnel-sg.lock"
set "FORWARD=172.17.0.1:17898:127.0.0.1:17998"

:guard_run
:: 清理上次运行的锁文件与残留 ssh 隧道，保证单实例
if exist "!LOCK!" del /f /q "!LOCK!" >nul 2>&1
wmic process where "name='ssh.exe' and commandline like '%%!FORWARD!%%'" call terminate >nul 2>&1
ping -n 1 -w 1000 127.0.0.1 >nul
start "sub2api-tunnel-!G!" /min cmd /c "!TOOLS!\sub2api-tunnel-!G!.cmd"
goto :eof

:: ===== 子程序：启动新加坡专用 Mihomo 代理（若未运行则启动）=====
:startsgproxy
netstat -an | findstr /R "127.0.0.1:%SGPORT% " >nul
if not errorlevel 1 (
  echo [sub2api] 新加坡专用代理已在监听 127.0.0.1:%SGPORT%，跳过启动。
  goto :eof
)
if not exist "%SGCONF%" (
  echo [sub2api] 错误：未找到新加坡专用代理配置 %SGCONF%。如需更换节点，请先运行 start-sub2api-singapore-proxy.ps1 -重新选择 生成配置。
  exit /b 1
)
if not exist "%MIHOMO%" (
  echo [sub2api] 错误：未找到 Mihomo 程序 %MIHOMO%。请确认 Clash Verge 安装目录。
  exit /b 1
)
echo [sub2api] 启动新加坡专用 Mihomo 实例...
start "" /min "%MIHOMO%" -d "%CVHOME%" -f "%SGCONF%"
set "N=0"
:sgwait
set /a N+=1
if %N% gtr 30 (
  echo [sub2api] 错误：新加坡专用 Mihomo 在 15 秒内未监听 127.0.0.1:%SGPORT%。请查看 %LOGDIR%\sub2api-singapore-mihomo.err.log。
  exit /b 1
)
netstat -an | findstr /R "127.0.0.1:%SGPORT% " >nul
if not errorlevel 1 (
  ping -n 1 -w 2000 127.0.0.1 >nul
  goto :eof
)
ping -n 1 -w 500 127.0.0.1 >nul
goto sgwait

:: ===== 子程序：从容器验证新加坡代理连通性并幂等登记 =====
:register_sg_proxy
set "PROXY=http://host.docker.internal:%SGPORT%"
set "REGLOG=%LOGDIR%\sub2api-local-singapore-proxy-registration.log"
set "PROBE=%LOGDIR%\sub2api-singapore-proxy-container-probe.tmp"

echo [sub2api] 从 sub2api 容器内验证新加坡代理到 ChatGPT 的连通性...
docker exec sub2api sh -lc "HTTPS_PROXY='%PROXY%' https_proxy='%PROXY%' wget -S --spider --timeout=12 https://chatgpt.com/ 2>&1" > "%PROBE%" 2>&1
findstr /R "HTTP/[0-9.]* [0-9][0-9][0-9]" "%PROBE%" >nul
if errorlevel 1 (
  echo [sub2api] 错误：容器内未能通过新加坡代理获得 ChatGPT HTTP 响应，不登记不可用代理。详情见 %PROBE%
  exit /b 1
)

echo [sub2api] 读取 PostgreSQL 连接信息...
docker inspect -f "{{range .Config.Env}}{{.}}{{println}}{{end}}" sub2api-postgres > "%LOGDIR%\sub2api-pg-env.tmp" 2>&1
if errorlevel 1 (
  echo [sub2api] 错误：无法读取 sub2api-postgres 容器配置，请确认本地 Docker 容器已启动。
  exit /b 1
)
set "PGUSER="
set "PGDB="
for /f "tokens=1,* delims==" %%a in (%LOGDIR%\sub2api-pg-env.tmp) do (
  if /i "%%a"=="POSTGRES_USER" set "PGUSER=%%b"
  if /i "%%a"=="POSTGRES_DB" set "PGDB=%%b"
)
if not defined PGUSER (
  echo [sub2api] 错误：未从容器环境变量中获取 POSTGRES_USER，无法登记代理。
  exit /b 1
)
if not defined PGDB (
  echo [sub2api] 错误：未从容器环境变量中获取 POSTGRES_DB，无法登记代理。
  exit /b 1
)

echo [sub2api] 幂等登记本地新加坡代理（清理重复行，确保唯一活动行）...
docker exec -i sub2api-postgres psql -X -v ON_ERROR_STOP=1 -U !PGUSER! -d !PGDB! -At -F "|" -f - < "%TOOLS%\sub2api-singapore-register.sql" > "%LOGDIR%\sub2api-singapore-register-result.tmp" 2>&1
if errorlevel 1 (
  echo [sub2api] 错误：新加坡代理已通过连通性验证，但写入本地代理列表失败。请查看 %LOGDIR%\sub2api-singapore-register-result.tmp 与 sub2api-postgres 容器日志。
  exit /b 1
)

set "OK=0"
for /f "tokens=1,2,3 delims=|" %%a in (%LOGDIR%\sub2api-singapore-register-result.tmp) do (
  if "%%b"=="新加坡家宽12-Mihomo" if "%%c"=="active" set "OK=1"
)
if "%OK%"=="0" (
  echo [sub2api] 错误：新加坡代理登记后校验失败，本地代理列表未出现唯一正确记录。实际返回：
  type "%LOGDIR%\sub2api-singapore-register-result.tmp"
  exit /b 1
)

echo %DATE% %TIME% 本地新加坡代理已可选择：新加坡家宽12-Mihomo（容器探测通过）。 >> "%REGLOG%"
echo [sub2api] 本地新加坡代理已登记成功。
goto :eof
