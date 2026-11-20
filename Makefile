.PHONY: bootstrap build test check fmt-check frontend-check browser-check hermes-check kubernetes-check postgres-check go-vuln-check up down logs smoke
bootstrap:
	go run ./cmd/workbench bootstrap
build:
	go build ./cmd/...
test:
	go test -race ./...
fmt-check:
	@test -z "$$(gofmt -l cmd internal deploy/kubernetes)" || (gofmt -l cmd internal deploy/kubernetes; exit 1)
frontend-check:
	cd frontend && npm ci && npm run format:check && npm test && npm run build
browser-check:
	cd frontend && npm run test:browser
hermes-check:
	PYTHONPATH=integrations/hermes/src:integrations/hermes/tests python3 -m unittest discover -s integrations/hermes/tests -v
kubernetes-check:
	@mkdir -p .local/tools
	GOBIN="$(CURDIR)/.local/tools" go install sigs.k8s.io/kustomize/kustomize/v5@v5.8.1
	KUSTOMIZE_BINARY="$(CURDIR)/.local/tools/kustomize" go test -count=1 ./deploy/kubernetes
go-vuln-check:
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
postgres-check:
	@test -n "$$TEST_DATABASE_URL" || (echo 'Set TEST_DATABASE_URL to a test PostgreSQL URL'; exit 1)
	go test -race ./internal/store -run TestPostgresMetadataContract -count=1
check: fmt-check build test frontend-check hermes-check kubernetes-check
	go vet ./...
up: bootstrap
	docker compose --profile build build
	go run ./cmd/workbench check-runtime-host
	DOCKER_SOCKET_GID="$$(stat -c %g /var/run/docker.sock)" docker compose up -d --wait
down:
	docker compose down
logs:
	docker compose logs --tail=100
smoke:
	go run ./cmd/workbench smoke
