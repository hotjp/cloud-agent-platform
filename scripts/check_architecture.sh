#!/bin/bash
set -e

echo "Checking layer dependencies..."

# Allowed dependencies following the architecture rules:
# L5-Gateway → L3-Authz → L4-Service → L2-Domain → L1-Storage
ALLOWED_DEPS=(
  "gateway:authz"
  "gateway:service"
  "authz:service"
  "authz:domain"
  "service:domain"
  "service:storage"
  "domain:storage"
)

MODULE=$(go list -m 2>/dev/null || echo "github.com/cloud-agent-platform/cap")

check_layer() {
  local layer=$1
  local deps=$(go list -f '{{ join .Deps "\n" }}' ./internal/$layer 2>/dev/null || true)

  for dep in $deps; do
    if [[ "$dep" == "$MODULE/internal/"* ]]; then
      local dep_layer=${dep#$MODULE/internal/}
      local allowed=false

      for rule in "${ALLOWED_DEPS[@]}"; do
        local from=${rule%%:*}
        local to=${rule##*:}
        if [[ "$layer" == "$from" && "$dep_layer" == "$to"* ]]; then
          allowed=true
          break
        fi
      done

      # Check reverse dependencies (not allowed)
      if [[ "$dep_layer" == "gateway" && "$layer" != "gateway" ]]; then
        echo "ERROR: Layer '$layer' illegally depends on '$dep_layer' (reverse dependency)"
        exit 1
      fi

      if [[ "$allowed" == "false" && "$dep_layer" =~ ^(gateway|authz|service|domain|storage)$ ]]; then
        echo "ERROR: Layer '$layer' illegally depends on '$dep_layer'"
        exit 1
      fi
    fi
  done
}

for dir in gateway authz service domain storage; do
  if [[ -d "internal/$dir" ]]; then
    echo "Checking $dir..."
    check_layer "$dir"
  fi
done

echo "✓ Architecture check passed"
