#!/bin/bash
# CAP Worker Entrypoint
# clone → LLM call → apply changes → generate diff → report artifacts via API
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
    git config --global https.proxy "$GIT_PROXY" 2>/dev/null || true
fi

# ─── Validate required env vars ──────────────────────────────────────────────
for var in TASK_ID TASK_GOAL REPO_URL BASE_BRANCH RESULT_BRANCH LLM_API_URL LLM_API_KEY LLM_MODEL; do
    if [ -z "${!var:-}" ]; then
        echo "FATAL: required env var $var is not set"
        exit 1
    fi
done

# CAP_API_URL is the platform's own API (for artifact reporting)
CAP_API_URL="${CAP_API_URL:-http://host.docker.internal:18080}"

echo "=== CAP Worker Started ==="
echo "Task: $TASK_ID"
echo "Goal: $TASK_GOAL"
echo "Repo: $REPO_URL ($BASE_BRANCH → $RESULT_BRANCH)"

CLONE_DIR="/workspace/repo"

# ─── Helper: report artifact back to platform ────────────────────────────────
report_artifact() {
    local artifact_type="$1"  # "diff" or "files"
    local content="$2"

    if [ -z "$CAP_API_URL" ]; then
        echo "Skip artifact report (no CAP_API_URL)"
        return
    fi

    # Build artifact JSON
    local payload
    payload=$(jq -n \
        --arg task_id "$TASK_ID" \
        --arg type "$artifact_type" \
        --arg content "$content" \
        '{task_id: $task_id, type: $type, content: $content}')

    curl -sL -X POST "$CAP_API_URL/api/v1/tasks/$TASK_ID/artifacts" \
        -H "Content-Type: application/json" \
        -d "$payload" \
        --max-time 10 > /dev/null 2>&1 || {
        echo "WARNING: failed to report artifact to platform"
    }
}

# ─── Step 1: Clone ───────────────────────────────────────────────────────────
echo ""
echo "=== Step 1: Cloning repository ==="
git clone --depth 1 --single-branch --branch "$BASE_BRANCH" "$REPO_URL" "$CLONE_DIR" 2>&1 || {
    echo "Clone failed: $?"
    exit 1
}
cd "$CLONE_DIR"
git checkout -b "$RESULT_BRANCH" 2>&1 || {
    echo "Checkout failed: $?"
    exit 1
}
echo "Clone complete. HEAD: $(git rev-parse HEAD)"

# Save the initial tree hash for diff later
INITIAL_TREE=$(git write-tree 2>/dev/null || echo "")

# ─── Step 2: Call LLM ────────────────────────────────────────────────────────
echo ""
echo "=== Step 2: Calling LLM ($LLM_MODEL) ==="

FILE_LIST=$(find . -type f \
    -not -path './.git/*' \
    -not -path './vendor/*' \
    -not -path './node_modules/*' \
    -not -name '*.lock' \
    -not -name 'go.sum' \
    2>/dev/null | head -80)

REQUEST_BODY=$(jq -n \
    --arg model "$LLM_MODEL" \
    --arg goal "$TASK_GOAL" \
    --arg files "$FILE_LIST" \
    --arg constraints "${TASK_CONSTRAINTS:-}" \
    --arg verification "${TASK_VERIFICATION:-}" \
    '{
        model: $model,
        messages: [{
            role: "user",
            content: (
                "You are a software engineer. Complete this task:\n\n" +
                "## Task\n" + $goal + "\n\n" +
                "## Repository Files\n" + $files + "\n\n" +
                (if $constraints != "" then "## Constraints\n" + $constraints + "\n\n" else "" end) +
                (if $verification != "" then "## Verification\n" + $verification + "\n\n" else "" end) +
                "## Output Format (CRITICAL)\n" +
                "You MUST respond with ONLY a JSON object, no markdown fences, no explanation before or after:\n" +
                "{\"files\":[{\"path\":\"relative/path/to/file\",\"content\":\"complete file content here\"}],\"summary\":\"brief description of changes\"}\n" +
                "- Each file MUST have the complete content, not diffs or patches.\n" +
                "- path must be relative to repo root (e.g. src/main.go, NOT ./src/main.go).\n" +
                "- Include ALL files that need to be created or modified.\n" +
                "- Do NOT include unchanged files."
            )
        }],
        temperature: 0.3,
        max_tokens: 4096
    }')

LLM_RESPONSE=$(curl -sL -X POST "$LLM_API_URL" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $LLM_API_KEY" \
    -d "$REQUEST_BODY" \
    --max-time 120)

RESPONSE_CONTENT=$(echo "$LLM_RESPONSE" | jq -r '.choices[0].message.content // empty' 2>/dev/null)

if [ -z "$RESPONSE_CONTENT" ]; then
    echo "ERROR: LLM returned empty response"
    echo "Raw: $(echo "$LLM_RESPONSE" | head -c 500)"
    exit 1
fi

RESPONSE_LEN=${#RESPONSE_CONTENT}
echo "LLM response received ($RESPONSE_LEN chars)"

# ─── Step 3: Parse and apply changes ─────────────────────────────────────────
echo ""
echo "=== Step 3: Applying changes ==="

# Strip markdown fences
CLEAN_RESPONSE=$(echo "$RESPONSE_CONTENT" | sed 's/^```json[[:space:]]*//;s/^```[[:space:]]*//;s/```$//' | tr -d '\r')

# Parse JSON
FILES_JSON=$(echo "$CLEAN_RESPONSE" | jq -r '.files // empty' 2>/dev/null)
if [ -z "$FILES_JSON" ]; then
    FILES_JSON=$(echo "$CLEAN_RESPONSE" | grep -o '{.*}' | head -1 | jq -r '.files // empty' 2>/dev/null)
fi

SUMMARY=$(echo "$CLEAN_RESPONSE" | jq -r '.summary // "No summary"' 2>/dev/null)
CHANGED_FILES="[]"

if [ -n "$FILES_JSON" ]; then
    FILE_COUNT=$(echo "$FILES_JSON" | jq 'length' 2>/dev/null)
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

        mkdir -p "$(dirname "$FILE_PATH")"
        printf '%s' "$CONTENT" > "$FILE_PATH"
        echo "  [write] $FILE_PATH ($(printf '%s' "$CONTENT" | wc -c) bytes)"
    done
else
    echo "WARNING: Could not parse file operations"
    exit 1
fi

echo "Summary: $SUMMARY"

# ─── Step 4: Generate diff ───────────────────────────────────────────────────
echo ""
echo "=== Step 4: Generating diff ==="

git add -A
DIFF_OUTPUT=$(git diff --cached 2>/dev/null)
DIFF_LEN=${#DIFF_OUTPUT}

if [ "$DIFF_LEN" -eq 0 ]; then
    echo "No changes detected"
    DIFF_OUTPUT=""
else
    echo "Diff: $DIFF_LEN bytes"
fi

# Build changed files list
CHANGED_FILES=$(git diff --cached --name-only 2>/dev/null | jq -R -s 'split("\n") | map(select(length > 0))' 2>/dev/null || echo "[]")
CHANGED_COUNT=$(echo "$CHANGED_FILES" | jq 'length' 2>/dev/null || echo 0)
echo "Changed files: $CHANGED_COUNT"

# ─── Step 5: Report artifacts ────────────────────────────────────────────────
echo ""
echo "=== Step 5: Reporting artifacts ==="

# Report the full result as a single artifact payload
RESULT_PAYLOAD=$(jq -n \
    --arg task_id "$TASK_ID" \
    --arg summary "$SUMMARY" \
    --argjson changed_files "${CHANGED_FILES:-[]}" \
    --arg diff "$DIFF_OUTPUT" \
    --arg llm_response "$RESPONSE_CONTENT" \
    '{
        task_id: $task_id,
        summary: $summary,
        changed_files: $changed_files,
        diff: $diff,
        llm_response: $llm_response,
        diff_size: ($diff | length),
        file_count: ($changed_files | length)
    }')

curl -sL -X POST "$CAP_API_URL/api/v1/tasks/$TASK_ID/artifacts" \
    -H "Content-Type: application/json" \
    -d "$RESULT_PAYLOAD" \
    --max-time 10 2>&1 || echo "WARNING: artifact report failed"

echo "Artifact reported"

# ─── Step 6: Git commit (best effort, for audit trail) ───────────────────────
echo ""
echo "=== Step 6: Commit (audit trail) ==="

git config user.email "agent@cloud-agent-platform.dev"
git config user.name "CAP Agent"

git add -A
if ! git diff --cached --quiet 2>/dev/null; then
    COMMIT_MSG="agent($TASK_ID): $TASK_GOAL"
    git commit -m "${COMMIT_MSG:0:200}" 2>&1
    echo "Committed"

    # Push if credentials available
    if [ -n "${GIT_TOKEN:-}" ]; then
        PUSH_URL=$(echo "$REPO_URL" | sed "s|https://|https://${GIT_TOKEN}@|")
        git push "$PUSH_URL" "$RESULT_BRANCH" 2>&1 && echo "Pushed" || echo "Push failed"
    else
        echo "No GIT_TOKEN — skipping push"
    fi
fi

echo ""
echo "=== CAP Worker Complete ==="
echo "Task: $TASK_ID"
echo "Files changed: $CHANGED_COUNT"
echo "Diff size: $DIFF_LEN bytes"
echo "Summary: $SUMMARY"
