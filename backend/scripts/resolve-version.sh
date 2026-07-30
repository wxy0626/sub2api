#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
BACKEND_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
REPO_DIR="$(CDPATH= cd -- "$BACKEND_DIR/.." && pwd)"
VERSION_FILE="$BACKEND_DIR/cmd/server/VERSION"

# 本地版本文件带 local 标识时优先保留，避免可达的上游 vX.Y.Z 标签覆盖本地版本标识。
VERSION_FILE_VALUE="$(tr -d '\r\n' < "$VERSION_FILE")"
case "$VERSION_FILE_VALUE" in
  v[0-9]*-local-[0-9]*|[0-9]*-local-[0-9]*|v[0-9]*-local|[0-9]*-local)
    printf '%s\n' "$VERSION_FILE_VALUE"
    exit 0
    ;;
esac

# Prefer the exact release tag when building from a tagged checkout so
# source builds from vX.Y.Z don't inherit the previous VERSION file value.
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

printf '%s\n' "$VERSION_FILE_VALUE"
