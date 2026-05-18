#!/bin/bash
# OpenClaw Agent Worker Entrypoint
set -e

echo "[entrypoint] CAP_WORKER_MODE=${CAP_WORKER_MODE:-agent}"
echo "[entrypoint] CAP_WORKSPACE=${CAP_WORKSPACE:-/workspace}"
echo "[entrypoint] CAP_HTTP_PORT=${CAP_HTTP_PORT:-8080}"

# Configure git
if [ -n "${GIT_USER_NAME:-}" ]; then
    git config --global user.name "${GIT_USER_NAME}"
fi
if [ -n "${GIT_USER_EMAIL:-}" ]; then
    git config --global user.email "${GIT_USER_EMAIL}"
fi

# Setup SSH known_hosts
if [ -n "${GIT_SSH_KEY:-}" ]; then
    mkdir -p ~/.ssh
    echo "${GIT_SSH_KEY}" > ~/.ssh/id_rsa
    chmod 600 ~/.ssh/id_rsa
    ssh-keyscan github.com gitlab.com bitbucket.org 2>/dev/null > ~/.ssh/known_hosts 2>/dev/null || true
fi

# Setup workspace
mkdir -p "${CAP_WORKSPACE:-/workspace}"

# Execute
exec "$@"
