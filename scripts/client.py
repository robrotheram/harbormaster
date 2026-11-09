#!/usr/bin/env python3
"""Small local operator client. Credentials are read from disk, never echoed."""
import argparse
import json
import os
from pathlib import Path
import sys
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen
import uuid

root = Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("command", choices=["create", "list", "get", "start", "stop", "submit", "status", "result", "cancel", "usage"])
parser.add_argument("arguments", nargs="*")
parser.add_argument("--key", default=None, help="Reuse this key to safely retry a mutation")
parser.add_argument("--url", default=os.environ.get("CONTROLPLANE_URL", "http://127.0.0.1:8090"))
parser.add_argument("--provider", choices=["codex", "claude"], help="Native subscription agent for submit")
args = parser.parse_args()
counts = {"create": 0, "list": 0, "usage": 0, "get": 1, "start": 1, "stop": 1, "submit": 2, "status": 2, "result": 2, "cancel": 2}
if len(args.arguments) != counts[args.command]:
    parser.error(f"{args.command} expects {counts[args.command]} argument(s)")
for identifier in args.arguments[:1 if args.command in ("get", "start", "stop", "submit") else 2]:
    if args.command not in ("create", "list", "usage") and (not identifier.replace("_", "").replace("-", "").isalnum()):
        parser.error("invalid identifier")
try:
    token = Path(os.environ.get("CONTROL_TOKEN_FILE", root / ".local/secrets/control_token")).read_text().strip()
except OSError:
    sys.exit("Control token unavailable. Run make bootstrap first.")
path = "/v1/workspaces"
method, payload = "GET", None
key = args.key or "request_" + uuid.uuid4().hex
if args.command == "create":
    method, payload = "POST", {"project_id": "default", "idempotency_key": key}
elif args.command == "usage":
    path = "/v1/usage"
elif args.command != "list":
    path += "/" + args.arguments[0]
    if args.command in ("start", "stop"):
        path += "/" + args.command
        method, payload = "POST", {"idempotency_key": key}
    elif args.command == "submit":
        path += "/tasks"
        method, payload = "POST", {"instruction": args.arguments[1], "idempotency_key": key}
        if args.provider:
            payload["provider"] = args.provider
    elif args.command in ("status", "result", "cancel"):
        path += "/tasks/" + args.arguments[1]
        if args.command in ("result", "cancel"):
            path += "/" + args.command
        if args.command == "cancel":
            method, payload = "POST", {"idempotency_key": key}
data = json.dumps(payload).encode() if payload is not None else None
request = Request(args.url.rstrip("/") + path, data=data, method=method, headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
try:
    with urlopen(request, timeout=40) as response:
        print(json.dumps(json.load(response), indent=2))
except HTTPError as error:
    try:
        print(json.dumps(json.load(error), indent=2), file=sys.stderr)
    except (ValueError, OSError):
        print(f"Request failed ({error.code})", file=sys.stderr)
    sys.exit(1)
except (URLError, TimeoutError):
    sys.exit("Control plane unavailable. Check docker compose ps and the configured address.")
