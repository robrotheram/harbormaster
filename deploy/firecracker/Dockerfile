# Hermes and Python are baked into the immutable guest; no runtime downloads.
FROM python:3.14-slim-bookworm AS hermes-build
RUN apt-get update && apt-get install -y --no-install-recommends build-essential libolm-dev git && rm -rf /var/lib/apt/lists/*
ARG HERMES_COMMIT
ARG HERMES_SOURCE_SHA256
ADD --checksum=sha256:${HERMES_SOURCE_SHA256} https://codeload.github.com/NousResearch/hermes-agent/tar.gz/${HERMES_COMMIT} /tmp/hermes.tgz
RUN mkdir /opt/hermes && tar -xzf /tmp/hermes.tgz -C /opt/hermes --strip-components=1 && rm /tmp/hermes.tgz && python -m pip install --no-cache-dir uv==0.13.7 && cd /opt/hermes && uv sync --frozen --no-dev --extra matrix --extra mcp

FROM agent-workbench-workspace:dev
USER root
RUN apt-get update && apt-get install -y --no-install-recommends iproute2 libolm3 && rm -rf /var/lib/apt/lists/* && mkdir -p /boot-config /run /data /proc /sys /dev /tmp
COPY --from=hermes-build /usr/local/ /usr/local/
COPY --from=hermes-build /opt/hermes /opt/hermes
COPY sandbox-init /sbin/awb-init
COPY service /usr/local/bin/service
COPY hermes-runner /usr/local/bin/hermes-runner
COPY hermes-matrix /usr/local/bin/hermes-matrix
ENTRYPOINT ["/sbin/awb-init"]

COPY hermes_common.py hermes-runtime.json /usr/local/bin/

COPY hermes-digest /usr/local/bin/hermes-digest
COPY hermes-consume /usr/local/bin/hermes-consume
