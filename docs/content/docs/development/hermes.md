+++
title = "Hermes integration package"
aliases = ["/docs/reference/integrations/hermes/readme/"]
+++

The workspace image includes `workbench-hermes`, the adapter for the Hermes Python SDK. The compiled `workbench-tools` launchers run the adapter under the supervisor's process identity. The worker needs no runtime downloads.

The [runtime manifest](../../../../deploy/hermes/runtime.json) pins the upstream source. The [project tools manifest](../../../../internal/shared/project-tools.json) defines the MCP contract. Update the packaged copy when you change either manifest. `make hermes-check` detects drift.

The portable tests use SDK stubs and Python's standard library.

```sh
make hermes-check
```

Run the SDK contract test in the Hermes image. Mount the test directory read-only.

```sh
PYTHONPATH=integrations/hermes/tests:/opt/hermes /opt/hermes/.venv/bin/python \
  -m unittest workbench_hermes_tests.sdk_digest -v
```

Opt-in Go integration tests run the `live_*` fixtures. The production entry points are `hermes-runner`, `hermes-matrix`, `hermes-digest`, and `hermes-consume`. They share configuration and event framing in `common.py`.
