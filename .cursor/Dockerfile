# Base image for AgentBoard Personal cloud-agent dev environment.
# Node for the Vite/pnpm frontend; Go 1.26 for board-server/board-client;
# iproute2 provides `ss` for the client port collector.
FROM node:22-bookworm

ARG GO_VERSION=1.26.6

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        git curl ca-certificates iproute2 build-essential \
    && rm -rf /var/lib/apt/lists/* \
    && corepack enable

RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tgz \
    && tar -C /usr/local -xzf /tmp/go.tgz \
    && rm /tmp/go.tgz \
    && ln -sf /usr/local/go/bin/go /usr/local/bin/go \
    && ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

ENV PATH="/usr/local/go/bin:/root/go/bin:${PATH}"
ENV GOTOOLCHAIN=local
