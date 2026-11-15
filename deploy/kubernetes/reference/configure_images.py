#!/usr/bin/env python3
"""Resolve private GHCR images by digest and provision scoped pull credentials.
Token contents stay in memory and Kubernetes Secrets, never command arguments or
source files. All images must exist before any credential is installed.
"""
import argparse
import base64
import json
import re
import subprocess
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

SERVICES = ['controlplane', 'runtime-controller', 'dashboard', 'mcp',
            'inference-gateway', 'credential-broker', 'egress-gateway', 'workspace']


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--context', required=True)
    p.add_argument('--username')
    credentials = p.add_mutually_exclusive_group(required=True)
    credentials.add_argument('--token-file', type=Path)
    credentials.add_argument('--existing-pull-secret', action='store_true', help='Reuse the reference registry Secret without exporting credentials')
    p.add_argument('--tag', required=True)
    p.add_argument('--service', action='append', choices=SERVICES, help='Resolve only selected services into a separate candidate lock')
    p.add_argument('--output', type=Path, required=True)
    a = p.parse_args()
    if not re.fullmatch(r'(sha-[a-f0-9]{40}|v[0-9A-Za-z._-]+)', a.tag):
        p.error('use a full commit tag or an explicit version tag')
    command = ['kubectl', '--context', a.context, '--request-timeout=15s']
    if a.existing_pull_secret:
        secret = json.loads(subprocess.check_output(command + ['-n', 'workbench-system', 'get', 'secret', 'harbormaster-registry', '-o', 'json']))
        config = json.loads(base64.b64decode(secret['data']['.dockerconfigjson']))
        basic = config['auths']['ghcr.io']['auth']
    else:
        if not a.username:
            p.error('--username is required with --token-file')
        token = a.token_file.read_text().strip()
        if not token:
            p.error('empty pull token')
        basic = base64.b64encode(f'{a.username}:{token}'.encode()).decode()
    images = {}
    for service in a.service or SERVICES:
        repository = f'robrotheram/harbormaster/{service}'
        query = urllib.parse.urlencode({'service': 'ghcr.io', 'scope': f'repository:{repository}:pull'})
        request = urllib.request.Request('https://ghcr.io/token?' + query,
                                         headers={'Authorization': 'Basic ' + basic})
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                bearer = json.load(response)['token']
            request = urllib.request.Request(f'https://ghcr.io/v2/{repository}/manifests/{a.tag}',
                headers={'Authorization': 'Bearer ' + bearer,
                         'Accept': 'application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json'})
            with urllib.request.urlopen(request, timeout=30) as response:
                digest = response.headers['Docker-Content-Digest']
        except urllib.error.HTTPError as error:
            raise SystemExit(f'{service}: registry returned HTTP {error.code}; check build completion and read:packages access.') from None
        if not re.fullmatch(r'sha256:[a-f0-9]{64}', digest or ''):
            raise SystemExit(f'{service}: registry did not return a valid digest')
        images[service] = f'ghcr.io/{repository}@{digest}'
    docker_config = json.dumps({'auths': {'ghcr.io': {'auth': basic}}})
    for namespace in ['workbench-system', 'workbench-egress', 'workbench-reference']:
        obj = {'apiVersion': 'v1', 'kind': 'Secret',
               'metadata': {'name': 'harbormaster-registry', 'namespace': namespace},
               'type': 'kubernetes.io/dockerconfigjson',
               'stringData': {'.dockerconfigjson': docker_config}}
        subprocess.run(command + ['apply', '--server-side', '--field-manager=harbormaster-bootstrap', '-f', '-'],
                       input=json.dumps(obj), text=True, check=True, stdout=subprocess.DEVNULL)
    subprocess.run(command + ['-n', 'workbench-reference', 'patch', 'serviceaccount', 'workspace',
                   '--type=merge', '-p', json.dumps({'imagePullSecrets': [{'name': 'harbormaster-registry'}]})], check=True)
    a.output.parent.mkdir(parents=True, exist_ok=True)
    a.output.write_text(json.dumps({'tag': a.tag, 'images': images}, indent=2) + '\n')
    print(f'Resolved {len(images)} images and installed reference pull credentials.')


if __name__ == '__main__':
    main()
