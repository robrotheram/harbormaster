FROM node:24-bookworm-slim AS frontend-build
WORKDIR /ui
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.27.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG SERVICE=controlplane
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/${SERVICE} && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/workspace-mcp ./cmd/workspace-mcp && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/workbench-tools ./cmd/workbench-tools

FROM alpine:3.23 AS platform
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 10001 workbench && adduser -D -u 10001 -G workbench workbench && \
    mkdir -p /data/files && chown -R 10001:10001 /data
COPY --from=build /out/service /usr/local/bin/service
COPY --from=build /out/healthcheck /usr/local/bin/healthcheck
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/service"]
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s CMD ["/usr/local/bin/healthcheck"]

FROM platform AS dashboard
COPY --from=frontend-build /ui/dist /opt/workbench/ui
ENV FRONTEND_DIR=/opt/workbench/ui

# Pin the native T3 server/client bundle and provider CLIs at build time.
# T3's standalone executable needs glibc, libatomic and its adjacent assets.
FROM node:24-bookworm-slim AS workspace-tools
ARG CODEX_VERSION=0.164.0
ARG CLAUDE_VERSION=2.1.238
ARG T3_VERSION=0.1.3
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git python3 ripgrep libatomic1 && \
    rm -rf /var/lib/apt/lists/* && \
    npm install -g @openai/codex@${CODEX_VERSION} @anthropic-ai/claude-code@${CLAUDE_VERSION} t3@${T3_VERSION} && \
    t3 --version && cp -a /root/.t3/runtime/versions/${T3_VERSION} /opt/t3 && chmod 755 /opt/t3 && \
    ln -sf /opt/t3/t3 /usr/local/bin/t3 && \
    CODEX_NATIVE=$(rg --files /usr/local/lib/node_modules/@openai | rg '/codex$' | head -1) && \
    install -m 755 "$CODEX_NATIVE" /usr/local/bin/codex-native && \
    npm cache clean --force && codex --version && claude --version && \
    useradd -m -u 10001 workbench && mkdir -p /data/files && chown -R 10001:10001 /data
RUN CODEX_NATIVE=$(rg --files /usr/local/lib/node_modules/@openai | rg '/codex$' | head -1) && \
    for helper in "$(dirname "$CODEX_NATIVE")"/*; do \
      if [ -f "$helper" ] && [ "$(basename "$helper")" != codex ]; then ln -sf "$helper" "/usr/local/bin/$(basename "$helper")"; fi; \
    done
FROM workspace-tools AS workspace
RUN apt-get update && apt-get install -y --no-install-recommends xvfb fluxbox x11vnc novnc websockify xterm x11-utils fonts-dejavu-core && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/workbench-tools /usr/local/bin/workbench-tools
RUN ln -s workbench-tools /usr/local/bin/workbench-desktop && ln -s workbench-tools /usr/local/bin/workbench-preview
COPY --from=build /out/service /usr/local/bin/service
COPY --from=build /out/healthcheck /usr/local/bin/healthcheck
COPY --from=build /out/workspace-mcp /usr/local/bin/workspace-mcp
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/service"]
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s CMD ["/usr/local/bin/healthcheck"]
