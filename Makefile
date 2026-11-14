.PHONY: bootstrap build test check fmt-check frontend-check browser-check python-check postgres-check go-vuln-check up down logs smoke
bootstrap:
	python3 scripts/bootstrap.py
build:
	go build ./cmd/...
test:
	go test -race ./...
fmt-check:
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal; exit 1)
frontend-check:
	cd frontend && npm ci && npm run format:check && npm test && npm run build
browser-check:
	cd frontend && npm run test:browser
python-check:
	python3 -m unittest discover -s examples/factory/research -v
	python3 scripts/test-hermes-adapters.py
	python3 scripts/test-hermes-coordination.py
	python3 scripts/test-hermes-consume.py
	python3 scripts/test-firecracker-install.py
go-vuln-check:
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
postgres-check:
	@test -n "$$TEST_DATABASE_URL" || (echo 'Set TEST_DATABASE_URL to a test PostgreSQL URL'; exit 1)
	go test -race ./internal/store -run TestPostgresMetadataContract -count=1
check: fmt-check build test frontend-check python-check
	go vet ./...
up: bootstrap
	docker compose --profile build build
	sh scripts/check-runtime-host.sh
	docker compose up -d --wait
down:
	docker compose down
logs:
	docker compose logs --tail=100
smoke:
	python3 scripts/smoke.py
