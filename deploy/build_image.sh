#!/usr/bin/env bash
# 本地构建镜像的快速脚本，避免在命令行反复输入构建参数。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
# 从 VERSION 文件解析并递增本地构建版本。
VERSION="$("${REPO_ROOT}/backend/scripts/resolve-version.sh")"
VERSION_FILE="${REPO_ROOT}/backend/cmd/server/VERSION"

echo "Building Sub2API version ${VERSION}"
docker build \
    -t "sub2api:${VERSION}" \
    -t sub2api:local-main \
    -t sub2api:latest \
    --build-arg VERSION="${VERSION}" \
    --build-arg GOPROXY=https://goproxy.cn,direct \
    --build-arg GOSUMDB=sum.golang.google.cn \
    -f "${REPO_ROOT}/Dockerfile" \
    "${REPO_ROOT}"

# 镜像构建成功后记录最新本地版本，确保下一次构建继续递增而不是复用旧标签。
printf '%s\n' "${VERSION}" > "${VERSION_FILE}"
echo "Updated ${VERSION_FILE} to ${VERSION}"
