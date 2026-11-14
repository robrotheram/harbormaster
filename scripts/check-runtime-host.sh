#!/bin/sh
# Run as the controller's configured user with its actual Compose socket mount.
# A fresh container also avoids probing an old systemd runtime-directory mount.
set -eu
docker compose run --rm --no-deps -T --entrypoint /bin/sh runtime-controller -ec '
    if [ "${RUNTIME_BACKEND:-docker}" != firecracker ]; then
        exit 0
    fi
    socket=${FIRECRACKER_SOCKET:-/run/agent-workbench/controller.sock}
    echo "Checking Firecracker host controller at $socket..."
    if ! /usr/local/bin/healthcheck --unix-socket "$socket"; then
        echo "Firecracker host controller unavailable; stopping make up (no runtime fallback)." >&2
        echo "Check: systemctl status agent-workbench-firecracker.service" >&2
        echo "Logs: journalctl -u agent-workbench-firecracker.service" >&2
        echo "If installed but stopped: sudo systemctl start agent-workbench-firecracker.service" >&2
        echo "For installation instructions, see docs/firecracker.md. Then retry make up." >&2
        exit 1
    fi
    echo "Firecracker host controller is healthy."
'
