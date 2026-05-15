#!/bin/bash
# CAP Worker Entrypoint
# Full chain: clone → LLM call → apply changes → commit → push
set -uo pipefail

# ─── Proxy setup ─────────────────────────────────────────────────────────────
# Override Docker Desktop's broken proxy (127.0.0.1 → host.docker.internal)
if [ -n "${HTTP_PROXY:-}" ] || [ -n "${HTTPS_PROXY:-}" ]; then
    echo "Proxy configured"
fi
# Ensure lowercase variants exist
[ -n "${HTTP_PROXY:-}" ] && [ -z "${http_proxy:-}" ] && export http_proxy="$HTTP_PROXY"
[ -n "${HTTPS_PROXY:-}" ] && [ -z "${https_proxy:-}" ] && export https_proxy="$HTTPS_PROXY"
# Git needs explicit proxy config
if [ -n "${https_proxy:-}${HTTPS_PROXY:-}" ]; then
    GIT_PROXY="${https_proxy:-${HTTPS_PROXY:-}}"
    git config --global http.proxy "$GIT_PROXY" 2>/dev/null || true
    git config --global https.proxy "$GIT_PROXY" 2>/dev/null || true
fi

# ─── Validate required env vars ──────────────────────────────────────────────
for var in TASK_ID TASK_GOAL REPO_URL BASE_BRANCH RESULT_BRANCH LLM_API_URL LLM_API_KEY LLM_MODEL; do
    if [ -z "${!var:-}" ]; then
        echo "ERROR: required env var $var is not set"
        exit 1
    fi
done

echo "=== CAP Worker Started ==="
echo "Task: $TASK_ID"
echo "Goal: $TASK_GOAL"
echo "Repo: $REPO_URL ($BASE_BRANCH → $RESULT_BRANCH)"

WORKING_DIR="${WORKING_DIR:-.}"
CLONE_DIR="/workspace/repo"

# ─── Step 1: Clone ───────────────────────────────────────────────────────────
echo ""
echo "=== Step 1: Cloning repository ==="
git clone --depth 1 --single-branch --branch "$BASE_BRANCH" "$REPO_URL" "$CLONE_DIR" 2>&1 || {
    echo "Clone failed with exit code $?"
    exit 1
}
cd "$CLONE_DIR"
git checkout -b "$RESULT_BRANCH" 2>&1 || {
    echo "Checkout branch failed: $?"
    exit 1
}
echo "Clone complete. HEAD: $(git rev-parse HEAD)"

# ─── Step 2: Call LLM ────────────────────────────────────────────────────────
echo ""
echo "=== Step 2: Calling LLM ($LLM_MODEL) ==="

# Collect file listing
FILE_LIST=$(find . -type f \
    -not -path './.git/*' \
    -not -path './vendor/*' \
    -not -path './node_modules/*' \
    -not -name '*.lock' \
    -not -name 'go.sum' \
    2>/dev/null | head -80)

# Build JSON request body with jq (avoids all escaping issues)
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
                "## Output Format\n" +
                "Respond with a JSON object:\n" +
                "{\"files\":[{\"path\":\"relative/path\",\"content\":\"file content here\"}],\"summary\":\"what you changed\"}\n" +
                "Create or modify files. Do NOT wrap in markdown fences."
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

# Extract content from response
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

# Strip markdown code fences if present
CLEAN_RESPONSE=$(echo "$RESPONSE_CONTENT" | sed 's/^```json//;s/^```//;s/```$//' | tr -d '\r')

# Try to parse as JSON
FILES_JSON=$(echo "$CLEAN_RESPONSE" | jq -r '.files // empty' 2>/dev/null)

if [ -z "$FILES_JSON" ]; then
    echo "No .files array found in LLM response. Attempting to extract JSON block..."
    # Try to find any JSON object in the response
    FILES_JSON=$(echo "$CLEAN_RESPONSE" | grep -o '{.*}' | head -1 | jq -r '.files // empty' 2>/dev/null)
fi

SUMMARY=$(echo "$CLEAN_RESPONSE" | jq -r '.summary // "No summary provided"' 2>/dev/null)

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

        mkdir -p "$(dirname "$FILE_PATH")"
        echo "$CONTENT" > "$FILE_PATH"
        echo "  [write] $FILE_PATH ($(echo "$CONTENT" | wc -c) bytes)"
    done
else
    echo "WARNING: Could not parse file operations from LLM response"
    echo "Saving raw response as result.txt"
    echo "$RESPONSE_CONTENT" > /workspace/result.txt
fi

echo ""
echo "Summary: $SUMMARY"

# ─── Step 4: Commit ──────────────────────────────────────────────────────────
echo ""
echo "=== Step 4: Committing ==="

git config user.email "agent@cloud-agent-platform.dev"
git config user.name "CAP Agent"

git add -A

if git diff --cached --quiet 2>/dev/null; then
    echo "No changes to commit"
else
    COMMIT_MSG="agent($TASK_ID): $TASK_GOAL"
    git commit -m "${COMMIT_MSG:0:200}" 2>&1
    echo "Changes committed"

    # Push (may fail if no push access — that's OK for testing)
    git push origin "$RESULT_BRANCH" 2>&1 && echo "Pushed to $RESULT_BRANCH" || echo "Push failed (expected for test repos)"
fi

echo ""
echo "=== CAP Worker Complete ==="
echo "Task: $TASK_ID"
echo "Summary: $SUMMARY"
