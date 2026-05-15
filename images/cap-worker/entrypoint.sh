#!/bin/bash
# CAP Worker Entrypoint v3
# Two modes:
#   1. Git Container mode (GIT_CONTAINER_API set): files pre-mounted, just LLM + write + notify git API
#   2. Standalone mode (no GIT_CONTAINER_API): full clone + LLM + commit (legacy)
set -uo pipefail

# ─── Proxy setup ─────────────────────────────────────────────────────────────
if [ -n "${HTTP_PROXY:-}" ] || [ -n "${HTTPS_PROXY:-}" ]; then
    echo "Proxy configured"
fi
[ -n "${HTTP_PROXY:-}" ] && [ -z "${http_proxy:-}" ] && export http_proxy="$HTTP_PROXY"
[ -n "${HTTPS_PROXY:-}" ] && [ -z "${https_proxy:-}" ] && export https_proxy="$HTTPS_PROXY"
if [ -n "${https_proxy:-}${HTTPS_PROXY:-}" ]; then
    GIT_PROXY="${https_proxy:-${HTTPS_PROXY:-}}"
    git config --global http.proxy "$GIT_PROXY" 2>/dev/null || true
fi

# ─── Validate required env vars ──────────────────────────────────────────────
for var in TASK_ID TASK_GOAL LLM_API_URL LLM_API_KEY LLM_MODEL; do
    if [ -z "${!var:-}" ]; then
        echo "FATAL: required env var $var is not set"
        exit 1
    fi
done

# Mode detection
GIT_MODE="${GIT_CONTAINER_API:-}"
WORK_DIR="/workspace"

echo "=== CAP Worker Started ==="
echo "Task: $TASK_ID"
echo "Goal: $TASK_GOAL"

if [ -n "$GIT_MODE" ]; then
    echo "Mode: Git Container (API: $GIT_MODE)"
    echo "Project: ${PROJECT_ID:-unknown}"
    # Working dir is the mounted volume from the Git container
    cd "$WORK_DIR" 2>/dev/null || true
    exec /usr/local/bin/worker-git-mode.sh
else
    echo "Mode: Standalone (legacy)"
    for var in REPO_URL BASE_BRANCH RESULT_BRANCH; do
        if [ -z "${!var:-}" ]; then
            echo "FATAL: required env var $var is not set for standalone mode"
            exit 1
        fi
    done
    REPO_URL="${REPO_URL:-}"
    BASE_BRANCH="${BASE_BRANCH:-main}"
    RESULT_BRANCH="${RESULT_BRANCH:-cap-agent/$TASK_ID}"
    echo "Repo: $REPO_URL ($BASE_BRANCH → $RESULT_BRANCH)"
    exec /usr/local/bin/worker-standalone.sh
fi
