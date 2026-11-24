FROM node:26-bookworm-slim@sha256:5df79989ed685e09f0a98019db4cebfb22fb9df1a53c0161664a011f7ff6b0f1 AS frontend-build
WORKDIR /ui
COPY dashboard/package*.json ./
RUN npm ci
COPY dashboard/ ./
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

FROM gcr.io/distroless/static-debian13:nonroot@sha256:8ce6dd87e9823c5b48dcfb2c8604714ccb351ce59607327bcc4957448da279b5 AS platform
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
