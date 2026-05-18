#!/bin/bash
# =============================================================================
# OpenClaw Worker - Entrypoint Script
# =============================================================================
# Handles environment variable configuration for the OpenClaw agent worker
# - Processes SSH keys if provided
# - Sets up working directory
# - Starts the OpenClaw CLI

set -euo pipefail

# Configure SSH directory if SSH keys are provided
if [ -n "${SSH_PRIVATE_KEY:-}" ]; then
    mkdir -p /home/worker/.ssh
    echo "${SSH_PRIVATE_KEY}" > /home/worker/.ssh/id_rsa
    chmod 600 /home/worker/.ssh/id_rsa

    # Optionally add known hosts
    if [ -n "${SSH_KNOWN_HOSTS:-}" ]; then
        echo "${SSH_KNOWN_HOSTS}" > /home/worker/.ssh/known_hosts
        chmod 644 /home/worker/.ssh/known_hosts
    fi

    # Disable strict host key checking for known hosts
    if [ -f /home/worker/.ssh/known_hosts ]; then
        export GIT_SSH_COMMAND="ssh -o UserKnownHostsFile=/home/worker/.ssh/known_hosts -o StrictHostKeyChecking=accept-new"
    fi
fi

# Set working directory
WORKDIR="${WORKDIR:-/workspace}"
mkdir -p "${WORKDIR}"
cd "${WORKDIR}"

# Check if OpenClaw CLI is available
if ! command -v openclaw &> /dev/null; then
    echo "ERROR: OpenClaw CLI not found in PATH"
    exit 1
fi

# Execute OpenClaw CLI with any passed arguments
# Default to agent mode if no arguments provided
if [ $# -eq 0 ]; then
    exec openclaw agent start
else
    exec openclaw "$@"
fi