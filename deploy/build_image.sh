#!/usr/bin/env bash
# 本地构建镜像的快速脚本，避免在命令行反复输入构建参数。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
# 从 VERSION 文件解析并递增本地构建版本。
VERSION="$("${REPO_ROOT}/backend/scripts/resolve-version.sh")"

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
