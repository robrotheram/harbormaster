#!/usr/bin/env python3
"""Stage an official release, kernel and trusted guest image; never needs root."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
STAGE = ROOT / '.local/firecracker'
VERSION = 'v1.17.0'
ARCHIVE = f'firecracker-{VERSION}-x86_64.tgz'
URL = f'https://github.com/firecracker-microvm/firecracker/releases/download/{VERSION}/'
# Upstream release checksum, pinned so an upstream asset/checksum replacement
# fails rather than being silently accepted.
RELEASE_SHA256 = '06094a1108ae9e82aa4c23a775aa92758f53f1175d422270d9d6162cb9ade558'
KERNEL_URL = 'https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/20261105-6ea6634994fa-0/x86_64/vmlinux-6.1.188'
KERNEL_SHA256 = '31fe88d6c72b93d5c9c63cca599ccadec49fd35673bbfd04850d6c00ff27f032'

def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()

def run(*args, **kwargs):
    return subprocess.run(args, cwd=ROOT, check=True, **kwargs)

def main():
    if os.uname().machine != 'x86_64':
        raise SystemExit('This pinned installation supports x86_64 only.')
    STAGE.mkdir(parents=True, exist_ok=True, mode=0o700)
    archive = STAGE / ARCHIVE
    if not archive.exists():
        urllib.request.urlretrieve(URL + ARCHIVE, archive)
    if digest(archive) != RELEASE_SHA256:
        raise SystemExit('Release SHA256 mismatch')
    with tarfile.open(archive) as stream:
        stream.extractall(STAGE, filter='data')
    release = STAGE / f'release-{VERSION}-x86_64'
    for name in ('firecracker', 'jailer'):
        shutil.copyfile(release / f'{name}-{VERSION}-x86_64', STAGE / name)
        (STAGE / name).chmod(0o755)
    kernel = STAGE / 'vmlinux'
    if not kernel.exists():
        urllib.request.urlretrieve(KERNEL_URL, kernel)
    if digest(kernel) != KERNEL_SHA256:
        raise SystemExit('Kernel SHA256 mismatch')
    env = dict(os.environ, CGO_ENABLED='0')
    for pkg, output in [('sandbox-init', 'sandbox-init'), ('workspace', 'service'), ('firecracker-controller', 'controller')]:
        run('go', 'build', '-trimpath', '-o', str(STAGE / output), './cmd/' + pkg, env=env)
    shutil.copyfile(ROOT / 'scripts/workbench-desktop.sh', STAGE / 'workbench-desktop')
    (STAGE / 'workbench-desktop').chmod(0o755)
    shutil.copyfile(ROOT / 'scripts/workbench-preview.py', STAGE / 'workbench-preview')
    (STAGE / 'workbench-preview').chmod(0o755)
    integration = json.loads((ROOT / 'deploy/hermes/runtime.json').read_text())
    shutil.copyfile(ROOT / 'deploy/firecracker/Dockerfile', STAGE / 'Dockerfile')
    shutil.copyfile(ROOT / 'scripts/hermes-runner.py', STAGE / 'hermes-runner')
    (STAGE / 'hermes-runner').chmod(0o755)
    shutil.copyfile(ROOT / 'scripts/hermes-matrix.py', STAGE / 'hermes-matrix')
    (STAGE / 'hermes-matrix').chmod(0o755)
    shutil.copyfile(ROOT / 'scripts/hermes-digest.py', STAGE / 'hermes-digest')
    (STAGE / 'hermes-digest').chmod(0o755)
    shutil.copyfile(ROOT / 'scripts/hermes-consume.py', STAGE / 'hermes-consume')
    (STAGE / 'hermes-consume').chmod(0o755)
    shutil.copyfile(ROOT / 'internal/shared/project-tools.json', STAGE / 'project-tools.json')
    shutil.copyfile(ROOT / 'scripts/hermes_common.py', STAGE / 'hermes_common.py')
    shutil.copyfile(ROOT / 'deploy/hermes/runtime.json', STAGE / 'hermes-runtime.json')
    run('docker', 'build', '--build-arg', 'HERMES_COMMIT=' + integration['commit'],
        '--build-arg', 'HERMES_SOURCE_SHA256=' + integration['source_sha256'],
        '-t', 'agent-workbench-firecracker-guest:local', str(STAGE))
    image = subprocess.check_output(['docker', 'image', 'inspect', 'agent-workbench-firecracker-guest:local', '--format', '{{.Id}}'], text=True).strip()
    container = subprocess.check_output(['docker', 'create', '--network', 'none', image], text=True).strip()
    try:
        run('docker', 'export', '-o', str(STAGE / 'rootfs.tar'), container)
    finally:
        run('docker', 'rm', container, stdout=subprocess.DEVNULL)
    (STAGE / 'sources.json').write_text(json.dumps({'firecracker_version': VERSION, 'release_sha256': RELEASE_SHA256, 'kernel_url': KERNEL_URL, 'kernel_sha256': KERNEL_SHA256, 'guest_image': image}, indent=2) + '\n')
    print('Staged binaries and guest image in', STAGE)
    print('Root installation command: pkexec python3', ROOT / 'scripts/firecracker-install.py')

if __name__ == '__main__':
    main()
