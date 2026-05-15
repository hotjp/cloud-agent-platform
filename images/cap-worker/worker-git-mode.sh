#!/bin/bash
# worker-git-mode.sh — Worker runs with files pre-mounted from Git container
# Only does: read file list → LLM call → write files → notify Git API to commit
set -uo pipefail

GIT_API="${GIT_CONTAINER_API:?GIT_CONTAINER_API is required}"
CAP_API_URL="${CAP_API_URL:-http://host.docker.internal:18080}"
TASK_ID="${TASK_ID:?}"
TASK_GOAL="${TASK_GOAL:?}"
RESULT_BRANCH="${RESULT_BRANCH:-cap-agent/$TASK_ID}"
WORK_DIR="/workspace"

cd "$WORK_DIR"

# ─── Step 1: Get file list from Git API ──────────────────────────────────────
echo ""
echo "=== Step 1: Reading project files ==="
FILE_LIST=$(curl -sL --noproxy "*" "$GIT_API/files" --max-time 10 2>/dev/null || echo '{"files":[]}')

FILE_COUNT=$(echo "$FILE_LIST" | jq '.files | length' 2>/dev/null || echo 0)
echo "Project has $FILE_COUNT files"

# Build file tree for LLM context (first 200 files, truncated)
FILE_TREE=$(echo "$FILE_LIST" | jq -r '.files[:200][]' 2>/dev/null | head -200 | tr '\n' ',' | sed 's/,$//')

# ─── Step 2: Read key files for context ──────────────────────────────────────
echo ""
echo "=== Step 2: Calling LLM ($LLM_MODEL) ==="

# Build system prompt with file list
SYSTEM_PROMPT="You are a coding agent. You receive a task and a list of existing project files.
Respond with a JSON object containing your changes. Format:
{\"summary\": \"what you did\", \"files\": [{\"path\": \"filename\", \"content\": \"file content\"}]}
Rules:
- Paths must be relative, no leading ./
- Only include files you create or modify
- Return valid JSON only, no markdown fences"

USER_PROMPT="Task: $TASK_GOAL

Existing files in project: $FILE_TREE

Produce the file changes needed to complete this task."

# Build LLM request
LLM_BODY=$(jq -n \
    --arg sys "$SYSTEM_PROMPT" \
    --arg user "$USER_PROMPT" \
    --arg model "$LLM_MODEL" \
    '{
        model: $model,
        messages: [
            {role: "system", content: $sys},
            {role: "user", content: $user}
        ],
        temperature: 0.3,
        max_tokens: 4096
    }')

RESPONSE=$(curl -sL "$LLM_API_URL" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $LLM_API_KEY" \
    -d "$LLM_BODY" \
    --max-time 60 2>/dev/null)

if [ -z "$RESPONSE" ]; then
    echo "FATAL: LLM call returned empty response"
    exit 1
fi

RESPONSE_CONTENT=$(echo "$RESPONSE" | jq -r '.choices[0].message.content // empty' 2>/dev/null)
if [ -z "$RESPONSE_CONTENT" ]; then
    echo "FATAL: Could not extract LLM response content"
    echo "Raw response: $RESPONSE" | head -5
    exit 1
fi

# Strip markdown fences
CLEAN_RESPONSE=$(echo "$RESPONSE_CONTENT" | sed 's/^```json[[:space:]]*//;s/^```[[:space:]]*//;s/```$//' | tr -d '\r')

RESPONSE_LEN=${#RESPONSE_CONTENT}
echo "LLM response received ($RESPONSE_LEN chars)"

# ─── Step 3: Apply changes ───────────────────────────────────────────────────
echo ""
echo "=== Step 3: Applying changes ==="

SUMMARY=$(echo "$CLEAN_RESPONSE" | jq -r '.summary // "No summary"' 2>/dev/null)
FILES_JSON=$(echo "$CLEAN_RESPONSE" | jq -r '.files // empty' 2>/dev/null)

if [ -z "$FILES_JSON" ]; then
    echo "No file operations in response"
    SUMMARY="No changes needed: $SUMMARY"
else
    FILE_COUNT=$(echo "$FILES_JSON" | jq 'length' 2>/dev/null || echo 0)
    echo "Found $FILE_COUNT file operations"

    echo "$FILES_JSON" | jq -c '.[]' 2>/dev/null | while IFS= read -r op; do
        FILE_PATH=$(echo "$op" | jq -r '.path // empty')
        CONTENT=$(echo "$op" | jq -r '.content // empty')

        if [ -z "$FILE_PATH" ] || [ -z "$CONTENT" ]; then
            echo "  [skip] missing path or content"
            continue
        fi

        # Strip leading ./ if present
        FILE_PATH=$(echo "$FILE_PATH" | sed 's|^\./||')

        FULL_PATH="$WORK_DIR/$FILE_PATH"
        mkdir -p "$(dirname "$FULL_PATH")"
        echo "$CONTENT" > "$FULL_PATH"
        echo "  [write] $FILE_PATH ($(echo "$CONTENT" | wc -c | tr -d ' ') bytes)"
    done
fi

echo "Summary: $SUMMARY"

# ─── Step 4: Notify Git API to commit ────────────────────────────────────────
echo ""
echo "=== Step 4: Commit via Git API ==="

COMMIT_RESULT=$(curl -sL --noproxy "*" -X POST "$GIT_API/commit" \
    -H "Content-Type: application/json" \
    -d "{\"message\": \"agent($TASK_ID): $TASK_GOAL\", \"branch\": \"$RESULT_BRANCH\"}" \
    --max-time 10 2>/dev/null || echo '{"ok":false}')

COMMIT_SHA=$(echo "$COMMIT_RESULT" | jq -r '.sha // "unknown"' 2>/dev/null)
echo "Committed: $COMMIT_SHA"

# ─── Step 5: Get diff from Git API ───────────────────────────────────────────
echo ""
echo "=== Step 5: Collecting diff ==="

DIFF_RESULT=$(curl -sL --noproxy "*" "$GIT_API/diff" --max-time 10 2>/dev/null || echo '{"diff":"","diff_size":0}')
DIFF_SIZE=$(echo "$DIFF_RESULT" | jq -r '.diff_size // 0' 2>/dev/null)

echo "Diff size: $DIFF_SIZE bytes"

# ─── Step 6: Report artifacts to platform ────────────────────────────────────
echo ""
echo "=== Step 6: Reporting artifacts ==="

CHANGED_FILES=$(git diff --name-only 2>/dev/null | jq -R -s 'split("\n") | map(select(length > 0))' 2>/dev/null || echo "[]")

RESULT_PAYLOAD=$(jq -n \
    --arg task_id "$TASK_ID" \
    --arg summary "$SUMMARY" \
    --argjson changed_files "${CHANGED_FILES:-[]}" \
    --arg diff "$DIFF_RESULT" \
    --arg llm_response "$RESPONSE_CONTENT" \
    --arg commit_sha "$COMMIT_SHA" \
    '{
        task_id: $task_id,
        summary: $summary,
        changed_files: $changed_files,
        diff: $diff,
        llm_response: $llm_response,
        commit_sha: $commit_sha,
        diff_size: ($diff | length),
        file_count: ($changed_files | length)
    }')

curl -sL --noproxy "*" -X POST "$CAP_API_URL/api/v1/tasks/$TASK_ID/artifacts" \
    -H "Content-Type: application/json" \
    -d "$RESULT_PAYLOAD" \
    --max-time 10 2>/dev/null || echo "WARNING: artifact report failed"

echo "Artifact reported"

# ─── Step 7: Push (best effort, requires GIT_TOKEN) ──────────────────────────
echo ""
echo "=== Step 7: Push (if configured) ==="

if [ -n "${GIT_TOKEN:-}" ]; then
    PUSH_RESULT=$(curl -sL --noproxy "*" -X POST "$GIT_API/push" \
        -H "Content-Type: application/json" \
        -d "{\"branch\": \"$RESULT_BRANCH\", \"token\": \"$GIT_TOKEN\"}" \
        --max-time 30 2>/dev/null || echo '{"ok":false}')
    echo "Push: $(echo "$PUSH_RESULT" | jq -r '.ok // false')"
else
    echo "No GIT_TOKEN — skipping push"
fi

# ─── Done ─────────────────────────────────────────────────────────────────────
echo ""
echo "=== CAP Worker Complete ==="
echo "Task: $TASK_ID"
echo "Summary: $SUMMARY"
echo "Commit: $COMMIT_SHA"
