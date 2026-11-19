FROM node:24-bookworm-slim@sha256:dc26ae92c9fb15cf6b1d63e20f0944b2efe0f115b20e00be48fb23fceb061c54 AS frontend-build
WORKDIR /ui
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS go-source
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/

FROM go-source AS platform-build
ARG SERVICE=controlplane
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/${SERVICE} && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck && \
    mkdir -p /out/data/files

FROM go-source AS workspace-build
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/workspace && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/workspace-mcp ./cmd/workspace-mcp && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/workbench-tools ./cmd/workbench-tools

FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931 AS platform
COPY --from=platform-build --chown=10001:10001 /out/data/ /data/
COPY --from=platform-build /out/service /usr/local/bin/service
COPY --from=platform-build /out/healthcheck /usr/local/bin/healthcheck
WORKDIR /
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/service"]
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s CMD ["/usr/local/bin/healthcheck"]

FROM platform AS dashboard
COPY --from=frontend-build /ui/dist /opt/workbench/ui
ENV FRONTEND_DIR=/opt/workbench/ui

# T3's standalone executable needs glibc, libatomic and its adjacent assets.
FROM node:24-bookworm-slim@sha256:dc26ae92c9fb15cf6b1d63e20f0944b2efe0f115b20e00be48fb23fceb061c54 AS workspace-tools
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git python3 ripgrep libatomic1 && \
    rm -rf /var/lib/apt/lists/*
COPY tools/package.json tools/package-lock.json /opt/workspace-tools/
RUN --mount=type=cache,target=/root/.npm npm ci --prefix /opt/workspace-tools --omit=dev --no-audit --no-fund && \
    for cli in codex claude; do ln -s "/opt/workspace-tools/node_modules/.bin/$cli" "/usr/local/bin/$cli"; done && \
    T3_VERSION=$(node -p 'require("/opt/workspace-tools/node_modules/t3/package.json").version') && \
    T3CODE_HOME=/tmp/t3 /opt/workspace-tools/node_modules/.bin/t3 --version && \
    mv "/tmp/t3/runtime/versions/$T3_VERSION" /opt/t3 && rm -rf /tmp/t3 && chmod 755 /opt/t3 && \
    ln -s /opt/t3/t3 /usr/local/bin/t3 && \
    case "$TARGETARCH" in \
      amd64) codex_arch=x64; codex_target=x86_64-unknown-linux-musl ;; \
      arm64) codex_arch=arm64; codex_target=aarch64-unknown-linux-musl ;; \
      *) echo "Unsupported workspace architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac && \
    CODEX_BIN="/opt/workspace-tools/node_modules/@openai/codex-linux-$codex_arch/vendor/$codex_target/bin" && \
    install -m 755 "$CODEX_BIN/codex" /usr/local/bin/codex-native && \
    for helper in "$CODEX_BIN"/*; do \
      if [ -f "$helper" ] && [ "$helper" != "$CODEX_BIN/codex" ]; then ln -s "$helper" "/usr/local/bin/$(basename "$helper")" || exit 1; fi; \
    done && \
    codex --version && codex-native --version && claude --version && t3 --version && \
    useradd -m -u 10001 workbench && mkdir -p /data/files && chown -R 10001:10001 /data

FROM workspace-tools AS workspace-base
RUN apt-get update && apt-get install -y --no-install-recommends xvfb fluxbox x11vnc novnc websockify xterm x11-utils fonts-dejavu-core && rm -rf /var/lib/apt/lists/*
COPY --from=workspace-build /out/workbench-tools /usr/local/bin/workbench-tools
RUN ln -s workbench-tools /usr/local/bin/workbench-desktop && ln -s workbench-tools /usr/local/bin/workbench-preview
COPY --from=workspace-build /out/service /usr/local/bin/service
COPY --from=workspace-build /out/healthcheck /usr/local/bin/healthcheck
COPY --from=workspace-build /out/workspace-mcp /usr/local/bin/workspace-mcp
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/service"]
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s CMD ["/usr/local/bin/healthcheck"]

FROM python:3.14-slim-bookworm@sha256:3fe1a0b6c2d6b2ebb572906cdabeca75aeb558c0c17db5ffefbcf8094871c32b AS hermes-build
RUN apt-get update && apt-get install -y --no-install-recommends build-essential libolm-dev git && rm -rf /var/lib/apt/lists/*
ARG HERMES_COMMIT
ARG HERMES_SOURCE_SHA256
ADD --checksum=sha256:${HERMES_SOURCE_SHA256} https://codeload.github.com/NousResearch/hermes-agent/tar.gz/${HERMES_COMMIT} /tmp/hermes.tgz
WORKDIR /opt/hermes
RUN tar -xzf /tmp/hermes.tgz --strip-components=1 && rm /tmp/hermes.tgz && \
    python -m pip install --no-cache-dir uv==0.13.7 && uv sync --frozen --no-dev --extra matrix --extra mcp
COPY integrations/hermes /tmp/workbench-hermes
RUN uv pip install --python /opt/hermes/.venv/bin/python --no-deps /tmp/workbench-hermes

FROM workspace-base AS workspace-hermes-base
RUN apt-get update && apt-get install -y --no-install-recommends libolm3 && rm -rf /var/lib/apt/lists/*
COPY --from=hermes-build /usr/local/ /usr/local/
COPY --from=hermes-build /opt/hermes /opt/hermes
RUN ln -s workbench-tools /usr/local/bin/hermes-runner && \
    ln -s workbench-tools /usr/local/bin/hermes-matrix && \
    ln -s workbench-tools /usr/local/bin/hermes-digest && \
    ln -s workbench-tools /usr/local/bin/hermes-consume && \
    PYTHONPATH=/opt/hermes /usr/local/bin/hermes-runner --version

FROM go-source AS firecracker-init-build
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -trimpath -o /out/awb-init ./cmd/sandbox-init

# The guest init configures mounts and loopback before starting the workspace as uid 10001.
FROM workspace-hermes-base AS firecracker-guest
RUN apt-get update && apt-get install -y --no-install-recommends iproute2 && rm -rf /var/lib/apt/lists/* && \
    mkdir -p /boot-config /run /data /proc /sys /dev /tmp
COPY --from=firecracker-init-build /out/awb-init /sbin/awb-init
ENTRYPOINT ["/sbin/awb-init"]

FROM workspace-hermes-base AS workspace-hermes
USER 10001:10001

FROM workspace-base AS workspace
USER 10001:10001
