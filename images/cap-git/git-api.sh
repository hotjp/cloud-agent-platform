#!/bin/bash
# git-api.sh — Lightweight Git HTTP API for cap-git containers
# Endpoints:
#   GET  /health                     → health check
#   POST /init                       → clone repo or create empty git
#   GET  /files                      → list files in repo
#   GET  /files?path=xxx             → read file content
#   POST /commit                     → git add -A && commit
#   GET  /diff                       → git diff HEAD
#   POST /push                       → git push
#   GET  /log                        → git log --oneline
#   POST /branch                     → create + checkout branch

set -euo pipefail

PORT="${GIT_API_PORT:-9090}"

echo "cap-git API server starting on :$PORT"
echo "Workspace: $(pwd)"

# Ensure git is configured
git config --global user.email "agent@cloud-agent-platform.dev" 2>/dev/null || true
git config --global user.name "CAP Git Container" 2>/dev/null || true

# Simple HTTP server using bash + nc
while true; do
  # Accept connection, read request
  coproc NC { nc -l -p "$PORT" -q 1 2>/dev/null || true; }

  # Read HTTP request line
  read -r METHOD PATH HTTP_VERSION <&${NC[0]} 2>/dev/null || true

  # Read headers (consume them)
  CONTENT_LENGTH=0
  while IFS= read -r -t 2 line <&${NC[0]} 2>/dev/null; do
    line=$(echo "$line" | tr -d '\r')
    [[ -z "$line" ]] && break
    if [[ "$line" =~ ^Content-Length:\ *([0-9]+) ]]; then
      CONTENT_LENGTH=${BASH_REMATCH[1]}
    fi
  done

  # Read body if present
  BODY=""
  if [[ "$CONTENT_LENGTH" -gt 0 ]] 2>/dev/null; then
    BODY=$(head -c "$CONTENT_LENGTH" <&${NC[0]} 2>/dev/null || true)
  fi

  # Route
  STATUS=200
  RESPONSE="{}"

  case "$METHOD $PATH" in
    "GET /health")
      RESPONSE='{"ok":true,"service":"cap-git"}'
      ;;

    "POST /init")
      REPO_URL=$(echo "$BODY" | jq -r '.repo_url // empty' 2>/dev/null || true)
      BRANCH=$(echo "$BODY" | jq -r '.branch // "main"' 2>/dev/null || true)

      if [[ -n "$REPO_URL" ]]; then
        echo "Cloning $REPO_URL (branch: $BRANCH)..."
        # Configure proxy for git
        if [[ -n "${HTTP_PROXY:-}" ]]; then
          git config --global http.proxy "$HTTP_PROXY" 2>/dev/null || true
        fi
        git clone --depth 1 --single-branch -b "$BRANCH" "$REPO_URL" /workspace/repo 2>&1 || true
        if [[ -d /workspace/repo ]]; then
          cp -a /workspace/repo/. /workspace/
          rm -rf /workspace/repo
          RESPONSE='{"ok":true,"action":"clone","branch":"'"$BRANCH"'"}'
        else
          # Clone failed, init empty
          git init /workspace 2>/dev/null || true
          RESPONSE='{"ok":true,"action":"init_empty","warning":"clone failed"}'
        fi
      else
        git init /workspace 2>/dev/null || true
        RESPONSE='{"ok":true,"action":"init_empty"}'
      fi
      ;;

    "GET /files")
      QUERY_PATH=$(echo "$PATH" | sed 's|/files||' | sed 's|^/||')
      if [[ -n "$QUERY_PATH" ]]; then
        # Read specific file
        if [[ -f "$QUERY_PATH" ]]; then
          CONTENT=$(cat "$QUERY_PATH" | base64)
          RESPONSE='{"ok":true,"path":"'"$QUERY_PATH"'","content":"'"$CONTENT"'","encoding":"base64"}'
        else
          STATUS=404
          RESPONSE='{"ok":false,"error":"file not found"}'
        fi
      else
        # List all files
        FILES=$(git ls-files 2>/dev/null || find . -type f -not -path './.git/*' | sed 's|^\./||')
        FILE_JSON=$(echo "$FILES" | jq -R -s 'split("\n") | map(select(length > 0))' 2>/dev/null || echo "[]")
        RESPONSE='{"ok":true,"files":'"$FILE_JSON"'}'
      fi
      ;;

    "POST /commit")
      MESSAGE=$(echo "$BODY" | jq -r '.message // "agent commit"' 2>/dev/null || echo "agent commit")
      BRANCH_NAME=$(echo "$BODY" | jq -r '.branch // empty' 2>/dev/null || true)

      if [[ -n "$BRANCH_NAME" ]]; then
        git checkout -b "$BRANCH_NAME" 2>/dev/null || git checkout "$BRANCH_NAME" 2>/dev/null || true
      fi

      git add -A 2>/dev/null || true
      CHANGES=$(git diff --cached --stat 2>/dev/null || echo "")

      if [[ -z "$CHANGES" ]]; then
        RESPONSE='{"ok":true,"action":"commit","message":"no changes"}'
      else
        git commit -m "$MESSAGE" 2>/dev/null || true
        COMMIT_SHA=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
        RESPONSE='{"ok":true,"action":"commit","sha":"'"$COMMIT_SHA"'","message":"'"$MESSAGE"'"}'
      fi
      ;;

    "GET /diff")
      DIFF=$(git diff HEAD~1 2>/dev/null || git diff --cached 2>/dev/null || git diff 2>/dev/null || echo "")
      DIFF_SIZE=${#DIFF}
      RESPONSE='{"ok":true,"diff":"'"$(echo "$DIFF" | base64 -w0)"'","diff_size":'"$DIFF_SIZE"'}'
      ;;

    "POST /push")
      REMOTE=$(echo "$BODY" | jq -r '.remote // "origin"' 2>/dev/null || echo "origin")
      PUSH_BRANCH=$(echo "$BODY" | jq -r '.branch // ""' 2>/dev/null || true)
      TOKEN=$(echo "$BODY" | jq -r '.token // empty' 2>/dev/null || true)

      if [[ -n "$TOKEN" ]] && [[ -n "$REMOTE" ]]; then
        # Inject token into remote URL
        CURRENT_URL=$(git remote get-url "$REMOTE" 2>/dev/null || echo "")
        if [[ "$CURRENT_URL" == https://* ]]; then
          AUTH_URL=$(echo "$CURRENT_URL" | sed "s|https://|https://${TOKEN}@|")
          git remote set-url "$REMOTE" "$AUTH_URL" 2>/dev/null || true
        fi
      fi

      if [[ -n "$PUSH_BRANCH" ]]; then
        PUSH_OUTPUT=$(git push "$REMOTE" "$PUSH_BRANCH" 2>&1) || PUSH_OUTPUT="push failed"
      else
        PUSH_OUTPUT=$(git push "$REMOTE" 2>&1) || PUSH_OUTPUT="push failed"
      fi

      RESPONSE='{"ok":true,"output":"'"$(echo "$PUSH_OUTPUT" | head -5)"'"}'
      ;;

    "GET /log")
      LOG=$(git log --oneline -20 2>/dev/null || echo "no commits yet")
      RESPONSE='{"ok":true,"log":"'"$(echo "$LOG" | base64 -w0)"'"}'
      ;;

    "POST /branch")
      BRANCH_NAME=$(echo "$BODY" | jq -r '.name // empty' 2>/dev/null || true)
      if [[ -z "$BRANCH_NAME" ]]; then
        STATUS=400
        RESPONSE='{"ok":false,"error":"branch name required"}'
      else
        git checkout -b "$BRANCH_NAME" 2>/dev/null || git checkout "$BRANCH_NAME" 2>/dev/null || true
        RESPONSE='{"ok":true,"branch":"'"$BRANCH_NAME"'"}'
      fi
      ;;

    *)
      STATUS=404
      RESPONSE='{"ok":false,"error":"not found","path":"'"$PATH"'"}'
      ;;
  esac

  # Send HTTP response
  echo -e "HTTP/1.1 $STATUS OK\r\nContent-Type: application/json\r\nContent-Length: ${#RESPONSE}\r\nConnection: close\r\n\r\n$RESPONSE" >&${NC[1]} 2>/dev/null || true

  # Cleanup
  wait $NC_PID 2>/dev/null || true
done
