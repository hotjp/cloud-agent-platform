#!/bin/sh
# =============================================================================
# OpenClaw Agent Worker - Entrypoint Script
# =============================================================================
# Handles environment variable configuration for the worker agent
# - Processes SSH keys if provided
# - Sets up working directory
# - Starts the worker process

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

# Execute worker with any passed arguments
exec /usr/local/bin/worker "$@"