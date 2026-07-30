#!/bin/sh
set -e

# Fix data directory permissions when running as root.
# Docker named volumes / host bind-mounts may be owned by root,
# preventing the non-root sub2api user from writing files.
if [ "$(id -u)" = "0" ]; then
    mkdir -p /app/data
    # Use || true to avoid failure on read-only mounted files (e.g. config.yaml:ro)
    chown -R sub2api:sub2api /app/data 2>/dev/null || true
    # 本地镜像回退需要访问宿主 Docker socket；运行时按 socket GID 创建组，避免固定宿主机 GID。
    if [ -S /var/run/docker.sock ]; then
        docker_socket_gid="$(stat -c '%g' /var/run/docker.sock 2>/dev/null || true)"
        if [ -n "$docker_socket_gid" ]; then
            docker_socket_group="$(awk -F: -v gid="$docker_socket_gid" '$3 == gid { print $1; exit }' /etc/group)"
            if [ -z "$docker_socket_group" ]; then
                addgroup -g "$docker_socket_gid" sub2api-docker 2>/dev/null || true
                docker_socket_group="sub2api-docker"
            fi
            if [ -n "$docker_socket_group" ]; then
                addgroup sub2api "$docker_socket_group" 2>/dev/null || true
            fi
        fi
    fi
    # Re-invoke this script as sub2api so the flag-detection below
    # also runs under the correct user.
    exec su-exec sub2api "$0" "$@"
fi

# Compatibility: if the first arg looks like a flag (e.g. --help),
# prepend the default binary so it behaves the same as the old
# ENTRYPOINT ["/app/sub2api"] style.
if [ "${1#-}" != "$1" ]; then
    set -- /app/sub2api "$@"
fi

exec "$@"
