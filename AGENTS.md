# 项目开发规则

## 错误提示

- 所有面向用户和管理员的错误提示必须给出中文的具体原因、排查方向或可执行处理方式；禁止只显示“操作失败”“账号异常”“请查看日志”等笼统文案。
- 标准业务说明使用中文；`HTTP 401`、`EOF`、`deactivated_workspace`、模型 ID、上游错误码、字段名等技术字段保留原始英文，便于准确排查。
- 当上游返回英文错误时，界面须显示“中文说明 + 后端返回的原始技术详情”，不能直接吞掉、替换或在客户端前端自行脱敏。
- 管理员后端负责在写库、写日志或返回响应前按需脱敏 `Authorization`、Cookie、API Key、access_token、refresh_token、密码及其他凭据；客户端前端直接展示后端提供的详情。
- 新增或修改错误路径时，必须补充覆盖中文说明、技术详情保留，以及管理员后端脱敏边界的测试。

## 批量账号测试

- 批量测试中，每个账号开始时只标记该账号为刷新中；该账号成功、失败或异常结束时，必须立即清除该账号的刷新状态，不得等待全部账号完成。
- 单个账号失败只记录并展示该账号的详细错误，不能中断其他账号的测试。

## Git 与远端发布

- 推送或更新远端服务器前，必须先将本次变更提交到本地 Git，并成功推送到对应 Git 远端。
- 禁止将未提交或未推送的工作区直接构建、部署或发布到远端服务器。

## 本地 Docker 构建与容器生效

- 代码版本更新后，网页版本不会仅因修改 `VERSION` 文件而生效；必须完成“构建版本镜像 -> 仅重建 `sub2api` 应用容器 -> 验证健康状态和网页版本”的完整流程。
- 构建前确认 Docker Desktop 引擎已运行，并确认 `deploy/.env` 中的 `SUB2API_IMAGE`、`SUB2API_VERSION`、`SUB2API_COMMIT` 与本次版本和提交一致。
- Windows 下 Docker CLI 可能不在当前 `PATH`。优先使用 Docker Desktop 安装目录中的绝对路径：
  ```powershell
  $dockerBin = 'C:\Users\Administrator\AppData\Local\Programs\DockerDesktop\resources\bin'
  $buildx = 'C:\Users\Administrator\AppData\Local\Programs\DockerDesktop\resources\cli-plugins\docker-buildx.exe'
  $compose = "$dockerBin\docker-compose.exe"
  ```
- 使用 Buildx 构建并载入本地镜像，必须传入版本和提交参数：
  ```powershell
  & $buildx build --progress plain --load `
    --build-arg VERSION=<版本> --build-arg COMMIT=<提交> `
    -t 'sub2api:<版本>' 'E:\AI\sun2api'
  ```
- 构建后仅重建应用容器，禁止因版本更新删除或重建 PostgreSQL、Redis 容器及其数据卷：
  ```powershell
  & $compose --env-file 'E:\AI\sun2api\deploy\.env' `
    -f 'E:\AI\sun2api\deploy\docker-compose.local.yml' `
    up -d --no-deps --force-recreate sub2api
  ```
- 若 Docker 凭据助手导致构建失败，可为当前命令设置临时 `DOCKER_CONFIG`，不得修改或删除用户原有 Docker 登录配置。
- 最终验收至少检查：`sub2api` 使用目标镜像、容器为 `healthy`、PostgreSQL/Redis 仍在运行，并通过网页或版本接口确认已显示目标版本。
