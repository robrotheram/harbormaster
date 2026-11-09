#!/usr/bin/env python3
"""Local human project setup. Never prints application credentials."""
import argparse
import json
from pathlib import Path
import uuid
from urllib.request import Request, urlopen
from urllib.error import HTTPError, URLError

ROOT = Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("command", choices=["list", "create", "access"])
parser.add_argument("value", nargs="?")
parser.add_argument("--key")
parser.add_argument("--url", default="http://127.0.0.1:8090")
args = parser.parse_args()
try:
    token = (ROOT / ".local/secrets/control_token").read_text().strip()
    route, payload = "/v1/projects", None
    if args.command == "create":
        if not args.value:
            parser.error("create requires a project name")
        payload = {"name": args.value, "idempotency_key": args.key or "project-" + uuid.uuid4().hex}
    if args.command == "access":
        if not args.value or not args.value.replace("_", "").replace("-", "").isalnum():
            parser.error("access requires a project ID")
        route += "/" + args.value + "/agent-access"
        payload = {"ttl_seconds": 86400}
    req = Request(args.url.rstrip("/") + route, data=json.dumps(payload).encode() if payload else None,
                  headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
    with urlopen(req, timeout=40) as response:
        result = json.load(response)
    if args.command == "access":
        directory = ROOT / ".local/project-access"
        directory.mkdir(mode=0o700, parents=True, exist_ok=True)
        target = directory / (args.value + ".json")
        target.touch(mode=0o600, exist_ok=True)
        target.chmod(0o600)
        target.write_text(json.dumps({"project_id": args.value, "expires_at": result["expires_at"],
                                      "mcpServers": {"workbench": {"type": "http", "url": "http://127.0.0.1:8082/mcp",
                                      "headers": {"Authorization": "Bearer " + result["access_token"]}}}}, indent=2))
        print("Project-scoped MCP configuration saved privately:", target)
        print("Expires:", result["expires_at"])
    else:
        print(json.dumps(result, indent=2))
except HTTPError as error:
    try:
        code = json.load(error).get("code", "REQUEST_FAILED")
    except (ValueError, OSError):
        code = "REQUEST_FAILED"
    raise SystemExit("Project operation failed: " + code)
except (OSError, ValueError, KeyError, URLError):
    raise SystemExit("Project operation failed. Check the local deployment; no credentials were printed.")
