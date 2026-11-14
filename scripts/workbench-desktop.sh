#!/bin/bash
# Runs entirely inside a private workspace. The supervisor owns the process group.
set -euo pipefail
export DISPLAY=:99
children=()
cleanup() { if ((${#children[@]})); then kill "${children[@]}" 2>/dev/null || true; fi; }
trap cleanup EXIT INT TERM
Xvfb "$DISPLAY" -screen 0 1440x900x24 -nolisten tcp -ac &
children+=("$!")
for attempt in {1..50}; do
    kill -0 "${children[0]}" || exit 1
    xdpyinfo >/dev/null 2>&1 && break
    sleep .1
done
xdpyinfo >/dev/null
fluxbox &
children+=("$!")
x11vnc -display "$DISPLAY" -localhost -rfbport 5901 -nopw -forever -shared -quiet &
children+=("$!")
/usr/bin/python3 - <<'READY'
import socket, time
for attempt in range(50):
    try:
        with socket.create_connection(('127.0.0.1', 5901), timeout=.2):
            break
    except OSError:
        time.sleep(.1)
else:
    raise SystemExit('Desktop display server did not become ready')
READY
xterm -title "Workspace terminal" -geometry 100x30+35+35 &
children+=("$!")
# Debian's noVNC root is a directory listing. Redirect to the interactive client.
web_root=$(mktemp -d /tmp/workbench-novnc.XXXXXX)
for asset in /usr/share/novnc/*; do ln -s "$asset" "$web_root/"; done
printf '%s\n' '<!doctype html><meta http-equiv="refresh" content="0;url=vnc.html?autoconnect=1&amp;resize=scale&amp;path=websockify"><title>Workspace desktop</title>' > "$web_root/index.html"
websockify --web="$web_root" "127.0.0.1:${PORT:?}" 127.0.0.1:5901 &
children+=("$!")
wait -n "${children[0]}" "${children[1]}" "${children[2]}" "${children[4]}"
