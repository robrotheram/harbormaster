#!/usr/bin/env python3
"""Exercise a running Compose deployment with synthetic data only."""
import json
import os
from pathlib import Path
import sys
import time
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen
import uuid

root = Path(__file__).resolve().parent.parent
base = os.environ.get("CONTROLPLANE_URL", "http://127.0.0.1:8090")
token = (root / ".local/secrets/control_token").read_text().strip()

def call(method, path, payload=None):
    request = Request(base + path, method=method, data=json.dumps(payload).encode() if payload is not None else None,
                      headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
    with urlopen(request, timeout=40) as response:
        return json.load(response)

try:
    ws = call("POST", "/v1/workspaces", {"project_id": "default", "idempotency_key": "functional-smoke-workspace"})
    identifier = ws["id"]
    prefix = "/v1/workspaces/" + identifier
    key = "smoke_" + uuid.uuid4().hex
    body = {"instruction": "Synthetic functional smoke test.", "idempotency_key": key}
    task = call("POST", prefix + "/tasks", body)
    replay = call("POST", prefix + "/tasks", body)
    assert task["id"] == replay["id"], "duplicate task on retry"
    task_path = prefix + "/tasks/" + task["id"]
    deadline = time.monotonic() + 60
    while task["state"] in ("queued", "running", "cancel_requested") and time.monotonic() < deadline:
        time.sleep(0.1)
        task = call("GET", task_path)
    assert task["state"] == "succeeded", "task did not succeed"
    before = call("GET", task_path + "/result")
    assert "SYNTHETIC FIXTURE" in before["content"], "expected fixture inference mode"
    stopped = call("POST", prefix + "/stop", {"idempotency_key": key + "_stop"})
    assert stopped["state"] == "STOPPED", "workspace did not stop"
    after = call("GET", task_path + "/result")
    assert after == before, "result lost across stop/start"
    current = call("GET", prefix)
    assert current["isolation_status"] == "unverified", "unexpected isolation claim"
    assert current["network_status"] == "unverified", "unexpected network claim"
    call("POST", prefix + "/stop", {"idempotency_key": key + "_final_stop"})
    print("Functional smoke passed: task deduplication, inference, stop/start and persistent result.")
    print("This does not verify VM isolation, host/network containment or T3 compatibility.")
except (HTTPError, URLError, TimeoutError, AssertionError, KeyError) as error:
    if isinstance(error, HTTPError):
        print(f"Smoke failed: control API returned HTTP {error.code}.", file=sys.stderr)
    elif isinstance(error, AssertionError):
        print(f"Smoke failed: {error}", file=sys.stderr)
    else:
        print("Smoke failed: deployment unavailable or returned an invalid response.", file=sys.stderr)
    sys.exit(1)
