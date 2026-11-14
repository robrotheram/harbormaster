# Contributing

Use the Go version declared in `go.mod`, Node.js 24, npm, and Python 3.12 or
newer. The default checks need no provider accounts, credentials, cluster, or
Docker daemon. PostgreSQL and live runtime acceptance are separate checks.

```sh
make check
cd frontend && npx playwright install chromium && cd ..
make browser-check
```

`make check` builds every Go command, runs tests with the race detector, checks
formatting and package boundaries, runs vet and portable Python adapter contracts,
and installs/tests/builds the UI.
The browser suite uses synthetic data and a temporary local test host. It must
not depend on an installed desktop app or a developer's browser cache path.

GitLab CI (`.gitlab-ci.yml`) and GitHub CI (`.github/workflows/check.yml`) run
the same portable checks. GitLab retains failing browser traces and Go coverage;
see [release operations](docs/operations.md) for deployment acceptance.

CI also sets `TEST_DATABASE_URL` to a disposable PostgreSQL 17 instance. To run
that contract locally, supply a PostgreSQL URL for a test role allowed to create
schemas, then run `go test -race ./internal/store -run TestPostgresMetadataContract`.
The test creates and drops only its uniquely named schema. Never supply
production credentials to test tooling.

## Making changes

Read [the current architecture](docs/architecture.md) and
[service contracts](docs/contracts.md) first. Keep an operation's policy in its
owning application module; transports adapt it and infrastructure implements
its interfaces. Introduce interfaces at actual substitution boundaries, rather
than wrapping every helper. Changes spanning HTTP, runtime clients, and
controllers need a test that crosses those boundaries.

Use small commits with a concrete problem, resulting behavior, and validation
in the description. Add tests for ownership, recovery, concurrency, or protocol
behavior affected by a change. Keep fixtures synthetic; never commit tokens,
private history, workspace files, `.local`, or build/browser artifacts.

Compatibility paths need a documented consumer and a removal condition. Keep
changes to isolation policy, runtime capability claims, and data retention
explicit. Do not infer project ownership from display names or delete/move
existing volumes as part of a migration.

## Live acceptance

`make up` builds and runs the development deployment; `make smoke` exercises
its fixture flows. These commands use Docker and create local state. Do not
use `docker compose down --volumes` to clean up a development session.

Kubernetes, Kata, Firecracker, subscription agents, and Matrix need their own
operator-provisioned infrastructure and acceptance checks. Unit tests simulate
their dependencies and do not establish real VM or network isolation. Consult
the relevant guide under `docs/` or `deploy/` before running live checks.

## License

The project is available under the [MIT license](LICENSE). Third-party runtimes,
dependencies, fonts, and integrations retain their respective licenses.
