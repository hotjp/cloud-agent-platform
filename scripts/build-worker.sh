#!/bin/bash
# =============================================================================
# OpenClaw Agent Worker - Docker Image Build Script
# =============================================================================
# Builds and optionally pushes Docker images for the CAP worker agent.
#
# Usage:
#   ./scripts/build-worker.sh              # Build worker image
#   ./scripts/build-worker.sh --push        # Build and push to registry
#   ./scripts/build-worker.sh --platform linux/amd64,linux/arm64  # Multi-platform
#   ./scripts/build-worker.sh --tag v1.0.0  # Custom tag
#   ./scripts/build-worker.sh --registry ghcr.io/example  # Custom registry
# =============================================================================

set -euo pipefail

# Default values
PUSH=false
PLATFORM="linux/amd64"
REGISTRY="${DOCKER_REGISTRY:-}"
TAG="${IMAGE_TAG:-latest}"
BUILD_TAGS="${BUILD_TAGS:-}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --push)
            PUSH=true
            shift
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
        --build-tags)
            BUILD_TAGS="$2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [options]"
            echo "  --push                  Push images after building"
            echo "  --platform <platforms> Comma-separated platforms (default: linux/amd64)"
            echo "  --tag <tag>            Image tag (default: latest)"
            echo "  --registry <url>      Docker registry URL"
            echo "  --build-tags <tags>   Build tags for Go build"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Configuration
IMAGE_NAME="${REGISTRY}cap-worker"
DOCKERFILE_PATH="$(pwd)/docker/Dockerfile.worker"
ENTRYPOINT_PATH="$(pwd)/docker/entrypoint.sh"

# Build arguments
build_args=(
    --file "${DOCKERFILE_PATH}"
    --platform "${PLATFORM}"
    --tag "${IMAGE_NAME}:${TAG}"
)

# Add build tags if specified
if [ -n "${BUILD_TAGS}" ]; then
    build_args+=(--build-arg "BUILD_TAGS=${BUILD_TAGS}")
fi

echo "==> Building ${IMAGE_NAME}:${TAG} for ${PLATFORM}"

# Build the image
if [[ "${PLATFORM}" == *,* ]]; then
    # Multi-platform build (requires buildx)
    docker buildx build "${build_args[@]}" --push
else
    docker build "${build_args[@]}"

    if [[ "${PUSH}" == true ]]; then
        echo "==> Pushing ${IMAGE_NAME}:${TAG}"
        docker push "${IMAGE_NAME}:${TAG}"
    fi
fi

echo "==> Build complete: ${IMAGE_NAME}:${TAG}"