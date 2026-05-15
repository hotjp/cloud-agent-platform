FROM cap-worker:latest

# cap-worker already has git, curl, jq, bash

COPY git-api-server /usr/local/bin/git-api-server
RUN chmod +x /usr/local/bin/git-api-server

EXPOSE 9090

# Override worker's ENTRYPOINT — this container is a Git server
ENTRYPOINT ["/usr/local/bin/git-api-server"]
