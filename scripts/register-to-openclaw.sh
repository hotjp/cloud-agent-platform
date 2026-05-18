#!/bin/bash
# register-to-openclaw.sh - Register MCP server to OpenClaw
# Usage: ./scripts/register-to-openclaw.sh [--unregister]

set -e

# Configuration
OPENCLAW_API_URL="${OPENCLAW_API_URL:-http://localhost:8081}"
CAP_SERVER_PORT="${CAP_SERVER_PORT:-18080}"
OPENCLAW_AUTH_TOKEN="${OPENCLAW_AUTH_TOKEN:-}"
MCP_ENDPOINT="http://localhost:${CAP_SERVER_PORT}/mcp"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if OpenClaw is reachable
check_openclaw() {
    log_info "Checking OpenClaw availability at ${OPENCLAW_API_URL}..."
    if curl -s --fail "${OPENCLAW_API_URL}/api/v1/health" > /dev/null 2>&1; then
        log_info "OpenClaw is reachable"
        return 0
    else
        log_warn "OpenClaw is not reachable at ${OPENCLAW_API_URL}"
        return 1
    fi
}

# Register MCP server to OpenClaw
do_register() {
    log_info "Registering MCP server to OpenClaw..."

    # Build registration payload
    local payload=$(cat <<EOF
{
    "name": "cloud-agent-platform",
    "type": "mcp",
    "version": "1.0.0",
    "endpoint": "${MCP_ENDPOINT}",
    "transport": "http"
}
EOF
)

    # Build curl command
    local curl_cmd="curl -s -X POST ${OPENCLAW_API_URL}/api/v1/mcp/servers"
    curl_cmd="${curl_cmd} -H 'Content-Type: application/json'"

    if [ -n "${OPENCLAW_AUTH_TOKEN}" ]; then
        curl_cmd="${curl_cmd} -H 'Authorization: Bearer ${OPENCLAW_AUTH_TOKEN}'"
    fi
    curl_cmd="${curl_cmd} -d '${payload}'"

    # Execute registration
    if response=$(eval "$curl_cmd"); then
        log_info "Successfully registered MCP server"
        log_info "Response: ${response}"
        return 0
    else
        log_error "Failed to register MCP server"
        return 1
    fi
}

# Unregister MCP server from OpenClaw
do_unregister() {
    log_info "Unregistering MCP server from OpenClaw..."

    local curl_cmd="curl -s -X DELETE ${OPENCLAW_API_URL}/api/v1/mcp/servers/cloud-agent-platform"

    if [ -n "${OPENCLAW_AUTH_TOKEN}" ]; then
        curl_cmd="${curl_cmd} -H 'Authorization: Bearer ${OPENCLAW_AUTH_TOKEN}'"
    fi

    # Execute unregistration
    if response=$(eval "$curl_cmd"); then
        log_info "Successfully unregistered MCP server"
        log_info "Response: ${response}"
        return 0
    else
        log_error "Failed to unregister MCP server"
        return 1
    fi
}

# Get registration status
do_status() {
    log_info "Checking MCP server registration status..."

    local curl_cmd="curl -s -X GET ${OPENCLAW_API_URL}/api/v1/mcp/servers/cloud-agent-platform"

    if [ -n "${OPENCLAW_AUTH_TOKEN}" ]; then
        curl_cmd="${curl_cmd} -H 'Authorization: Bearer ${OPENCLAW_AUTH_TOKEN}'"
    fi

    if response=$(eval "$curl_cmd"); then
        log_info "Registration status:"
        echo "${response}" | jq '.' 2>/dev/null || echo "${response}"
        return 0
    else
        log_warn "MCP server is not registered with OpenClaw"
        return 1
    fi
}

# Main
main() {
    case "${1:-}" in
        --unregister|-u)
            do_unregister
            ;;
        --status|-s)
            check_openclaw || exit 1
            do_status
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --unregister, -u    Unregister MCP server from OpenClaw"
            echo "  --status, -s        Check registration status"
            echo "  --help, -h          Show this help message"
            echo ""
            echo "Environment variables:"
            echo "  OPENCLAW_API_URL    OpenClaw API URL (default: http://localhost:8081)"
            echo "  CAP_SERVER_PORT     CAP server port (default: 18080)"
            echo "  OPENCLAW_AUTH_TOKEN OpenClaw auth token (optional)"
            exit 0
            ;;
        *)
            check_openclaw || exit 0
            do_register
            ;;
    esac
}

main "$@"