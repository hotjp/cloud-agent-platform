# CAP Worker — Agent execution container
#
# Receives task context via environment variables, clones the target repo,
# calls LLM API to produce code changes, commits and pushes results.
#
# Required env vars:
#   TASK_ID        - unique task identifier
#   TASK_GOAL      - what the agent should accomplish
#   REPO_URL       - git repository URL
#   BASE_BRANCH    - branch to checkout
#   RESULT_BRANCH  - branch to push results to
#   LLM_API_URL    - LLM API endpoint
#   LLM_API_KEY    - LLM API key
#   LLM_MODEL      - model name (e.g., "glm-4")
#
# Optional:
#   TASK_CONSTRAINTS    - JSON array of constraint strings
#   TASK_VERIFICATION   - JSON array of verification criteria
#   WORKING_DIR         - subdirectory to work in (default: repo root)

FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    git \
    curl \
    jq \
    bash \
    ca-certificates \
    openssh-client

# Create workspace
WORKDIR /workspace

# Copy entrypoint
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["sleep", "infinity"]
