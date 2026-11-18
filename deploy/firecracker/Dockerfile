# Hermes and Python are baked into the immutable guest; no runtime downloads.
ARG WORKSPACE_IMAGE=agent-workbench-workspace:dev
FROM python:3.14-slim-bookworm AS hermes-build
RUN apt-get update && apt-get install -y --no-install-recommends build-essential libolm-dev git && rm -rf /var/lib/apt/lists/*
ARG HERMES_COMMIT
ARG HERMES_SOURCE_SHA256
ADD --checksum=sha256:${HERMES_SOURCE_SHA256} https://codeload.github.com/NousResearch/hermes-agent/tar.gz/${HERMES_COMMIT} /tmp/hermes.tgz
RUN mkdir /opt/hermes && tar -xzf /tmp/hermes.tgz -C /opt/hermes --strip-components=1 && rm /tmp/hermes.tgz && python -m pip install --no-cache-dir uv==0.13.7 && cd /opt/hermes && uv sync --frozen --no-dev --extra matrix --extra mcp

COPY integrations/hermes /tmp/workbench-hermes
RUN uv pip install --python /opt/hermes/.venv/bin/python --no-deps /tmp/workbench-hermes

FROM golang:1.27.4-bookworm AS guest-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/awb-init ./cmd/sandbox-init && CGO_ENABLED=0 go build -trimpath -o /out/service ./cmd/workspace && CGO_ENABLED=0 go build -trimpath -o /out/workbench-tools ./cmd/workbench-tools

FROM ${WORKSPACE_IMAGE}
USER root
RUN apt-get update && apt-get install -y --no-install-recommends iproute2 libolm3 && rm -rf /var/lib/apt/lists/* && mkdir -p /boot-config /run /data /proc /sys /dev /tmp
COPY --from=hermes-build /usr/local/ /usr/local/
COPY --from=hermes-build /opt/hermes /opt/hermes
COPY --from=guest-build /out/awb-init /sbin/awb-init
COPY --from=guest-build /out/service /usr/local/bin/service
COPY --from=guest-build /out/workbench-tools /usr/local/bin/workbench-tools
ENTRYPOINT ["/sbin/awb-init"]
RUN ln -sf workbench-tools /usr/local/bin/hermes-runner && ln -sf workbench-tools /usr/local/bin/hermes-matrix && ln -sf workbench-tools /usr/local/bin/hermes-digest && ln -sf workbench-tools /usr/local/bin/hermes-consume && PYTHONPATH=/opt/hermes /usr/local/bin/hermes-runner --version
