FROM alpine:3.19

# If proxy is configured at build time, use it
ARG HTTP_PROXY
ARG HTTPS_PROXY

RUN apk add --no-cache git curl jq bash

# Worker scripts
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
COPY worker-git-mode.sh /usr/local/bin/worker-git-mode.sh
COPY worker-standalone.sh /usr/local/bin/worker-standalone.sh
RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/worker-git-mode.sh /usr/local/bin/worker-standalone.sh

WORKDIR /workspace

ENTRYPOINT ["sleep", "infinity"]
