#!/usr/bin/env python3
"""Install the staged jailer, host controller and guest assets. Requires root."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
STAGE = ROOT / '.local/firecracker'
DEST = Path('/opt/agent-workbench/firecracker')

def run(*args):
    subprocess.run(args, check=True)

def install(source, target, mode):
    shutil.copyfile(source, target)
    os.chown(target, 0, 0)
    target.chmod(mode)

def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()

def install_project_authority(secrets):
    # The host broker signs short-lived, project-scoped MCP credentials. This
    # key must match the existing control-plane/MCP authority, never a new key.
    source = ROOT / '.local/secrets/control_token'
    if not source.is_file() or len(source.read_text().strip()) < 32:
        raise SystemExit('Control-plane signing authority is missing or invalid; run scripts/bootstrap.py first.')
    install(source, secrets / 'project_mcp_signing_key', 0o600)

def accept_and_start():
    secrets = Path('/etc/agent-workbench/firecracker')
    run(str(DEST / 'prepare-host'))
    env = dict(os.environ,
               CONTROLLER_TOKEN_FILE=str(secrets / 'controller_token'),
               GATEWAY_SERVICE_TOKEN_FILE=str(secrets / 'gateway_service_token'),
               GRANT_SECRET_FILE=str(secrets / 'grant_secret'))
    subprocess.run([str(DEST / 'controller'), '--self-test'], env=env, check=True)
    run('systemctl', 'start', 'agent-workbench-firecracker.service')
    run('systemctl', 'is-active', 'agent-workbench-firecracker.service')
    run(str(DEST / 'firecracker'), '--version')
    print('Installed and acceptance passed. Workbench remains stopped; test guest has been stopped.')

def main():
    if os.geteuid() != 0:
        raise SystemExit('Administrator authentication is required for the jailer, cgroups and network namespaces.')
    if not Path('/dev/kvm').is_char_device():
        raise SystemExit('KVM unavailable')
    if not Path('/sys/fs/cgroup/cgroup.controllers').is_file():
        raise SystemExit('cgroup v2 required')
    for name in ('firecracker', 'jailer', 'controller', 'vmlinux', 'rootfs.tar', 'sources.json'):
        if not (STAGE / name).is_file():
            raise SystemExit('Run scripts/firecracker-stage.py first; missing ' + name)
    # Fail if called while guests/controller are running; never replace live
    # binaries, credentials or an image beneath a running VMM.
    status = subprocess.run(['systemctl', 'is-active', '--quiet', 'agent-workbench-firecracker.service'])
    if status.returncode == 0:
        raise SystemExit('Stop agent-workbench-firecracker.service before reinstalling.')
    if sys.argv[1:] == ['--controller-only']:
        if not (DEST / 'manifest.json').is_file():
            raise SystemExit('Full installation required first.')
        install_project_authority(Path('/etc/agent-workbench/firecracker'))
        install(STAGE / 'controller', DEST / 'controller', 0o755)
        accept_and_start()
        return
    if sys.argv[1:]:
        raise SystemExit('Unknown installation option')
    DEST.mkdir(parents=True, exist_ok=True, mode=0o755)
    for name in ('firecracker', 'jailer', 'controller'):
        install(STAGE / name, DEST / name, 0o755)
    install(STAGE / 'vmlinux', DEST / 'vmlinux', 0o444)
    install(STAGE / 'sources.json', DEST / 'sources.json', 0o444)
    install(STAGE / 'hermes-runtime.json', DEST / 'hermes-runtime.json', 0o444)
    for name in ('prepare-host', 'cleanup'):
        install(ROOT / 'deploy/firecracker' / name, DEST / name, 0o755)
    with tempfile.TemporaryDirectory(prefix='awb-guest-', dir='/var/tmp') as temp:
        # This is the operator-built immutable image, never a guest-modified disk.
        run('tar', '--extract', '--file', str(STAGE / 'rootfs.tar'), '--directory', temp, '--numeric-owner')
        image = DEST / 'rootfs.ext4'
        with image.open('wb') as stream:
            stream.truncate(6 << 30)
        run('mkfs.ext4', '-q', '-F', '-d', temp, str(image))
        image.chmod(0o444)
    manifest = {name: digest(DEST / name) for name in ('firecracker', 'jailer', 'vmlinux', 'rootfs.ext4', 'hermes-runtime.json')}
    (DEST / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    (DEST / 'manifest.json').chmod(0o444)
    secrets = Path('/etc/agent-workbench/firecracker')
    secrets.mkdir(parents=True, exist_ok=True, mode=0o700)
    for name in ('controller_token', 'gateway_service_token', 'grant_secret'):
        install(ROOT / '.local/secrets' / name, secrets / name, 0o600)
    install_project_authority(secrets)
    install(ROOT / 'deploy/firecracker/agent-workbench-firecracker.service', Path('/etc/systemd/system/agent-workbench-firecracker.service'), 0o644)
    run('systemctl', 'daemon-reload')
    # Real KVM acceptance precedes starting the management service. Failed
    # acceptance leaves the service and all workspaces stopped.
    accept_and_start()

if __name__ == '__main__':
    main()
