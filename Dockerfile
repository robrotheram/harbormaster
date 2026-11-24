FROM node:26-bookworm-slim@sha256:5df79989ed685e09f0a98019db4cebfb22fb9df1a53c0161664a011f7ff6b0f1 AS frontend-build
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

FROM scratch AS mise-download
ADD --checksum=sha256:be6f46a4cc3d5532acf359f163cf7ce80ceb43e5718d7b9049165a340fb02e2d https://github.com/jdx/mise/releases/download/v2026.11.11/mise-v2026.11.11-linux-x64 /opt/mise/amd64/mise
ADD --checksum=sha256:2fd4b495e19d8a10dcfc0e0badea539fd745bd697473b59d18acc2ee8ef8c815 https://github.com/jdx/mise/releases/download/v2026.11.11/mise-v2026.11.11-linux-arm64 /opt/mise/arm64/mise

# T3's standalone executable needs glibc, libatomic and its adjacent assets.
# Trixie provides the Xwayland version required by xwayland-satellite.
FROM node:26-trixie-slim@sha256:abe7e5a82b61a8ef67b315b85497c9e65173387ccf2f4a029248184b8e020be5 AS workspace-tools
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git python3 ripgrep libatomic1 && \
    rm -rf /var/lib/apt/lists/*
COPY tools/package.json tools/package-lock.json /opt/workspace-tools/
COPY --from=mise-download --chmod=755 /opt/mise/${TARGETARCH}/mise /usr/local/bin/mise
RUN --mount=type=cache,target=/root/.npm npm ci --prefix /opt/workspace-tools --omit=dev --no-audit --no-fund && \
    for cli in codex claude skills; do ln -s "/opt/workspace-tools/node_modules/.bin/$cli" "/usr/local/bin/$cli"; done && \
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
    codex --version && codex-native --version && claude --version && t3 --version && mise --version && skills --version && \
    useradd -m -u 10001 workbench && mkdir -p /data/files && chown -R 10001:10001 /data

FROM rust:1.94-trixie@sha256:652612f07bfbbdfa3af34761c1e435094c00dde4a98036132fca28c7bb2b165c AS xwayland-satellite-build
RUN apt-get update && apt-get install -y --no-install-recommends clang libxcb1-dev libxcb-cursor-dev && rm -rf /var/lib/apt/lists/*
ADD --checksum=sha256:8b32873d7d740143ce9efa3cfb43c17d0176b89d2e8ffaa497db709f154fc478 https://codeload.github.com/Supreeeme/xwayland-satellite/tar.gz/refs/tags/v0.8.3 /tmp/xwayland-satellite.tgz
WORKDIR /src
RUN tar -xzf /tmp/xwayland-satellite.tgz --strip-components=1 && cargo build --locked --release --bin xwayland-satellite && \
    install -D -s -m 755 target/release/xwayland-satellite /out/xwayland-satellite

FROM workspace-tools AS workspace-base
# Xwayland loads libEGL at runtime.
RUN apt-get update && apt-get install -y --no-install-recommends dbus-daemon foot fonts-dejavu-core libegl1 libxcb-cursor0 python3-venv xwayland && rm -rf /var/lib/apt/lists/*
COPY --from=xwayland-satellite-build /out/xwayland-satellite /usr/local/bin/xwayland-satellite
RUN --mount=type=bind,source=tools/wland-viewer.patch,target=/tmp/wland-viewer.patch \
    cd /opt/workspace-tools/node_modules/wland/dist && \
    git apply /tmp/wland-viewer.patch && \
    relay=$(XDG_CACHE_HOME=/tmp/wland-build node --input-type=module -e 'import { serverBinary } from "./index.mjs"; console.log(serverBinary())') && \
    install -m 755 "$relay" /usr/local/bin/wlandr && rm -rf /tmp/wland-build && \
    mkdir -p /opt/wland/web && cp index.html wland.wasm *.mjs /opt/wland/web/ && \
    rm /opt/wland/web/cli.mjs /opt/wland/web/headless.mjs /opt/wland/web/index.mjs
RUN --mount=type=cache,target=/root/.cache/pip python3 -m venv /opt/workbench-analysis && \
    /opt/workbench-analysis/bin/python -m pip install --only-binary=:all: duckdb==1.5.4
COPY --from=workspace-build /out/workbench-tools /usr/local/bin/workbench-tools
RUN ln -s workbench-tools /usr/local/bin/workbench-desktop && ln -s workbench-tools /usr/local/bin/workbench-preview && ln -s workbench-tools /usr/local/bin/workbench-analysis
COPY --from=workspace-build /out/service /usr/local/bin/service
COPY --from=workspace-build /out/healthcheck /usr/local/bin/healthcheck
COPY --from=workspace-build /out/workspace-mcp /usr/local/bin/workspace-mcp
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/service"]
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s CMD ["/usr/local/bin/healthcheck"]

FROM python:3.15-slim-bookworm@sha256:96599df822d0e1c6bb700014a05c7f3c8cabc6e0f7ff3c4f9f69cee033b4b5d9 AS hermes-build
RUN apt-get update && apt-get install -y --no-install-recommends build-essential libolm-dev git && rm -rf /var/lib/apt/lists/*
ARG HERMES_COMMIT
ARG HERMES_SOURCE_SHA256
ADD --checksum=sha256:${HERMES_SOURCE_SHA256} https://codeload.github.com/NousResearch/hermes-agent/tar.gz/${HERMES_COMMIT} /tmp/hermes.tgz
WORKDIR /opt/hermes
# Keep uv-managed interpreters with the virtualenv copied into the runtime image.
ENV UV_PYTHON_INSTALL_DIR=/opt/hermes/.python
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
