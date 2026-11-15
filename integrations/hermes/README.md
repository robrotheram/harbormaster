# Hermes integration package

Hermes exposes a Python SDK. `workbench-hermes` is the installed runtime adapter
for that SDK; it is built into the immutable image, with no runtime downloads.
The compiled `workbench-tools` launchers execute its modules and retain the
supervisor's process identity and signal ownership.

`deploy/hermes/runtime.json` pins upstream source. `internal/shared/project-tools.json`
defines the MCP tool contract. Package copies of those files are checked against
the originals by `make hermes-check`; update both when changing a contract.

Portable tests use SDK stubs and require only Python's standard library:

```sh
make hermes-check
```

The SDK contract test runs inside the Hermes image with the tests mounted read-only:

```sh
PYTHONPATH=integrations/hermes/tests:/opt/hermes /opt/hermes/.venv/bin/python \
  -m unittest workbench_hermes_tests.sdk_digest -v
```

The `live_*` modules are fixtures invoked by opt-in Go integration tests. They are
not deployment tools. Production entrypoints are `hermes-runner`, `hermes-matrix`,
`hermes-digest` and `hermes-consume`; their implementation shares `common.py`.
