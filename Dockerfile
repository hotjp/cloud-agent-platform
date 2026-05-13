# =============================================================================
# Cloud Agent Platform - Multi-stage Dockerfile
# =============================================================================
# Build stage
FROM --platform=linux/amd64 golang:1.25-bookworm AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy go mod files first for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build server binary (CGO_ENABLED=0 for pure Go)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" \
    -o server \
    ./cmd/server

# Build MCP binary (CGO_ENABLED=0 for pure Go)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" \
    -o mcp \
    ./cmd/mcp

# =============================================================================
# Server image (distroless base)
# =============================================================================
FROM gcr.io/distroless/static-debian12 AS server

# Create non-root user for security
USER nonroot

COPY --from=builder /build/server /server
COPY --from=builder /build/config.example.yaml /config.example.yaml

EXPOSE 8080 9090 6060

ENTRYPOINT ["/server"]
CMD ["--config", "/config/config.yaml"]

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

# =============================================================================
# MCP image (distroless base)
# =============================================================================
FROM gcr.io/distroless/static-debian12 AS mcp

USER nonroot

COPY --from=builder /build/mcp /mcp

ENTRYPOINT ["/mcp"]
