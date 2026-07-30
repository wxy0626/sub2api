#!/usr/bin/env bash
# 本地构建镜像的快速脚本，避免在命令行反复输入构建参数。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
# 从 VERSION 文件解析并递增本地构建版本。
VERSION="$("${REPO_ROOT}/backend/scripts/resolve-version.sh")"
VERSION_FILE="${REPO_ROOT}/backend/cmd/server/VERSION"
# 优先使用 Docker Desktop 客户端，避免 WSL 内置旧版 docker 缺少 Buildx。
if command -v docker.exe >/dev/null 2>&1; then
    DOCKER_CLI="docker.exe"
else
    DOCKER_CLI="docker"
fi
# Windows Docker CLI 需要 Windows 格式的构建上下文和 Dockerfile 路径。
DOCKER_CONTEXT="${REPO_ROOT}"
DOCKERFILE="${REPO_ROOT}/Dockerfile"
if [ "${DOCKER_CLI}" = "docker.exe" ] && command -v wslpath >/dev/null 2>&1; then
    DOCKER_CONTEXT="$(wslpath -w "${REPO_ROOT}")"
    DOCKERFILE="$(wslpath -w "${REPO_ROOT}/Dockerfile")"
fi

echo "Building Sub2API version ${VERSION}"
"${DOCKER_CLI}" buildx build --load \
    -t "sub2api:${VERSION}" \
    -t sub2api:local-main \
    -t sub2api:latest \
    --build-arg VERSION="${VERSION}" \
    --build-arg GOPROXY=https://goproxy.cn,direct \
    --build-arg GOSUMDB=sum.golang.google.cn \
    -f "${DOCKERFILE}" \
    "${DOCKER_CONTEXT}"

# 镜像构建成功后记录最新本地版本，确保下一次构建继续递增而不是复用旧标签。
printf '%s\n' "${VERSION}" > "${VERSION_FILE}"
echo "Updated ${VERSION_FILE} to ${VERSION}"
