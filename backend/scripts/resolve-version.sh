#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
BACKEND_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
REPO_DIR="$(CDPATH= cd -- "$BACKEND_DIR/.." && pwd)"
# 可通过 VERSION_FILE 注入临时版本文件，便于构建验证；默认使用项目版本文件。
VERSION_FILE="${VERSION_FILE:-$BACKEND_DIR/cmd/server/VERSION}"

# 正式标签构建保持发布版本；本地非标签构建基于 VERSION 文件递增。
if command -v git >/dev/null 2>&1; then
  TAG="$(
    git -C "$REPO_DIR" describe --tags --exact-match --match 'v[0-9]*' 2>/dev/null || \
    git -C "$REPO_DIR" describe --tags --exact-match --match '[0-9]*' 2>/dev/null || \
    true
  )"
  if [ -n "$TAG" ]; then
    printf '%s\n' "${TAG#v}"
    exit 0
  fi
fi

# 从 VERSION 文件取得当前本地版本，并只递增 local 序号。
CURRENT_VERSION="$(tr -d '\r\n' < "$VERSION_FILE")"
case "$CURRENT_VERSION" in
  *-local.[0-9]*)
    BASE_VERSION="${CURRENT_VERSION%-local.*}"
    LOCAL_NUMBER="${CURRENT_VERSION##*.}"
    case "$LOCAL_NUMBER" in
      ''|*[!0-9]*) printf '%s\n' "$CURRENT_VERSION"; exit 0 ;;
    esac
    printf '%s\n' "${BASE_VERSION}-local.$((LOCAL_NUMBER + 1))"
    ;;
  *-local)
    printf '%s\n' "${CURRENT_VERSION}.1"
    ;;
  *)
    printf '%s\n' "${CURRENT_VERSION}-local.1"
    ;;
esac
