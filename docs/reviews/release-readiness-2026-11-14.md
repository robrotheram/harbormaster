# Release readiness review — 2026-11-14

> Historical record. See the [documentation index](../README.md) for current setup and workflows.

## Scope and decision

Reviewed the current working tree, including existing uncommitted features,
with emphasis on Go service lifetime, module ownership, persistence, verification,
container packaging and operator recovery. This is a release engineering pass,
not a penetration test or certification of every code path.

The application has useful ownership boundaries and regression coverage:
application policy is separated from runtime implementations; architecture tests
guard imports; schema migrations precede listeners; PostgreSQL tests exercise
ownership, durable retry identity and reopen behavior. Retain these boundaries.
A broad late-stage module rewrite would add migration risk without evidence of a
specific defect.

Source publication can proceed once the intended complete working tree is
reviewed and committed and the new pipeline passes on GitLab. Shared-host or
production deployment still needs the runtime-specific acceptance and restore
rehearsal in [operations](../operations.md). No remote is configured in this
checkout, and this pass does not publish source or deploy services.

## Changes made

- Upgraded Go to 1.27.4 and `golang.org/x/text` to v0.41.0 after govulncheck
  identified three reachable advisories. Pinned the container build toolchain and
  added a pinned Go vulnerability scanner to both CI providers.
- Added GitLab verification jobs for Go formatting/build/vet/races and PostgreSQL,
  frontend formatting/unit/browser checks, Python contracts and Compose config.
  Coverage and failure traces have explicit expiry; credentials are synthetic.
- Reused the existing drain/cancel/join lifecycle for standalone dashboard,
  Docker/Kubernetes controller, egress, inference and broker HTTP listeners.
  Preserved broker/inference body/write timeouts and added a slow-body regression
  test. Gateway lease renewal now receives the process cancellation context.
- Allowed thirty seconds of Compose shutdown grace around the ten-second HTTP
  drain. Normal egress shutdown no longer invokes `log.Fatal(nil)`.
- Added the portable Firecracker signing-key installer tests to `make check`.
- Excluded local agent settings, CI caches and browser artifacts from release
  inputs as appropriate. Existing feature changes remain in place.
- Fixed the profile menu accessible name to include its visible initials and secondary text,
  after the browser accessibility suite exposed the mismatch.
- Added operator guidance for release evidence, single-replica limits, health
  interpretation, sensitive diagnostics, backups and recovery from migrations.

## Outstanding release evidence and follow-up

1. Run the pipeline on the destination GitLab instance. Local YAML parsing does
   not replace GitLab's configuration validation or prove runner/image access.
2. `npm audit` reports six dependency findings (two low, four moderate) through
   shadcn's registry/AST tooling: deepmerge prototype poisoning and minimatch
   amplification. The application imports `shadcn/tailwind.css`; do not remove or
   force-downgrade the package blindly. Review fixed compatible upstream versions
   before release; build-time exposure still matters even though these CLI
   dependencies are not application HTTP endpoints. No forced dependency update
   was applied. The Go advisory scan is now a CI gate; module checksum verification alone
   is not vulnerability scanning.
3. The lazy-loaded chart chunk exceeds Vite's 500 kB warning threshold. This is
   a performance follow-up, not a failed build; measure actual loading before
   introducing chunking rules.
4. Real provider, Vault, Matrix, native CLI, Firecracker/Kata/CNI and restoration
   checks require provisioned infrastructure. Synthetic tests cannot establish
   isolation or operational recovery. The default Compose stack remains a local
   development deployment, with a privileged controller and shared operator UI.
5. Shared HTTP draining cancels request contexts after timeout and waits for
   handlers. Handlers must cooperate; hijacked connections require their own
   ownership. Compose's stop grace remains the outer process termination bound.
6. Commit all required new feature files together with their tracked edits.
   Publishing only tracked-file diffs would omit many features already present
   in this checkout. Do not include `.env`, `.local`, agent settings or artifacts.

## Verification evidence

- Go 1.27.4: all commands build; `go vet ./...`, formatting, full
  `go test -race ./...` and `go mod verify` pass.
- `govulncheck` v1.9.0 reports no vulnerabilities after the updates.
- PostgreSQL 17: isolated migration/ownership/idempotency/reopen contract passes
  with the updated toolchain and dependencies; disposable container removed.
- Frontend: 18 unit-test files / 72 tests pass; production build, TypeScript and
  formatting pass. One concurrent formatting invocation was terminated (137);
  a clean rerun passed.
- Portable Python research/Hermes contracts and both Firecracker installer tests
  pass. Compose configuration and CI YAML parsing pass.
- Dashboard Docker target builds with Go 1.27.4. Its healthcheck passes while
  running non-root, read-only, without networking or capabilities; SIGTERM exits
  zero. Test containers were removed. This does not verify every service image
  or replace whole-stack fixture smoke/live runtime acceptance.
- Browser verification: the full 127-test run passed 120 and failed seven. Five
  failures shared the profile accessible-name defect; one navigation aborted and
  one worker fixture did not render in time. After the complete label fix, all
  seven failures plus six profile/AuthKit checks passed in a 13-test targeted
  rerun (2.1 minutes), including the full page accessibility sweep. No test was
  skipped or weakened to suppress accessibility findings. The two transient
  failures passed without timeout changes; monitor them in GitLab CI.
