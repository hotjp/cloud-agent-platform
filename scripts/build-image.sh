#!/bin/bash
# =============================================================================
# Cloud Agent Platform - Docker Image Build Script
# =============================================================================
# Builds and optionally pushes Docker images for the CAP server and MCP.
#
# Usage:
#   ./scripts/build-image.sh              # Build both images
#   ./scripts/build-image.sh --push        # Build and push to registry
#   ./scripts/build-image.sh --target mcp  # Build only MCP image
#   ./scripts/build-image.sh --platform linux/amd64,linux/arm64  # Multi-platform
# =============================================================================

set -euo pipefail

# Default values
PUSH=false
TARGETS=("server" "mcp")
PLATFORM="linux/amd64"
REGISTRY="${DOCKER_REGISTRY:-}"
TAG="${IMAGE_TAG:-latest}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --push)
            PUSH=true
            shift
            ;;
        --target)
            TARGETS=("$2")
            shift 2
            ;;
        --platform)
            PLATFORM="$2"
            shift 2
            ;;
        --tag)
            TAG="$2"
            shift 2
            ;;
        --registry)
            REGISTRY="$2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [options]"
            echo "  --push                  Push images after building"
            echo "  --target <name>        Build specific target (server|mcp), can be specified multiple times"
            echo "  --platform <platforms> Comma-separated platforms (default: linux/amd64)"
            echo "  --tag <tag>            Image tag (default: latest)"
            echo "  --registry <url>      Docker registry URL"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Configuration
IMAGE_NAME="${REGISTRY}cap"
DOCKERFILE_PATH="$(dirname "$0")/../Dockerfile"

# Build each target
for target in "${TARGETS[@]}"; do
    image="${IMAGE_NAME}-${target}:${TAG}"
    echo "==> Building ${image} for ${PLATFORM}"

    # Build arguments
    build_args=(
        --file "${DOCKERFILE_PATH}"
        --target "${target}"
        --platform "${PLATFORM}"
        --tag "${image}"
    )

    # Add multi-platform support if multiple platforms specified
    if [[ "${PLATFORM}" == *,* ]]; then
        build_args+=(--push)
    fi

    docker build "${build_args[@]}"

    if [[ "${PUSH}" == true ]] && [[ "${PLATFORM}" != *,* ]]; then
        echo "==> Pushing ${image}"
        docker push "${image}"
    fi
done

echo "==> Build complete: ${IMAGE_NAME}-{server,mcp}:${TAG}"
