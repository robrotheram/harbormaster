#!/usr/bin/env python3
"""Create fresh reference service credentials; preserve the identity on reruns.
Reads only this deployment's CNPG credentials and identity Secret. Never prints
secret values or writes them to disk. Requires kubectl and explicit context.
"""
import argparse
import base64
import json
import secrets
import subprocess
from urllib.parse import quote


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--context', required=True)
    args = parser.parse_args()
    command = ['kubectl', '--context', args.context, '--request-timeout=15s']

    def read(name):
        result = subprocess.run(command + ['-n', 'workbench-system', 'get',
                                'secret', name, '--ignore-not-found', '-o', 'json'],
                                capture_output=True, text=True, check=True)
        return json.loads(result.stdout) if result.stdout.strip() else None

    def save(namespace, name, data, create=False):
        obj = {'apiVersion': 'v1', 'kind': 'Secret',
               'metadata': {'name': name, 'namespace': namespace},
               'type': 'Opaque', 'stringData': data}
        operation = ['create'] if create else ['apply', '--server-side', '--field-manager=harbormaster-bootstrap']
        subprocess.run(command + operation + ['-f', '-'], input=json.dumps(obj),
                       text=True, check=True, stdout=subprocess.DEVNULL)
        print(f'Configured Secret {namespace}/{name}')

    def values(obj):
        return {k: base64.b64decode(v).decode() for k, v in obj['data'].items()}

    db, ca = read('harbormaster-db-app'), read('harbormaster-db-ca')
    if not db or not ca:
        raise SystemExit('Provision the reference PostgreSQL instance first.')
    identity = read('harbormaster-identity')
    if identity:
        identity = values(identity)
    else:
        identity = {key: secrets.token_urlsafe(48) for key in
                    ['control_token', 'controller_token', 'gateway_service_token', 'grant_secret', 'mcp_token']}
        save('workbench-system', 'harbormaster-identity', identity, create=True)
    database = values(db)
    dsn = (f"postgres://{quote(database['username'], safe='')}:{quote(database['password'], safe='')}"
           '@harbormaster-db-rw.workbench-system.svc:5432/workbench'
           '?sslmode=verify-full&sslrootcert=/run/secrets/database_ca.crt')
    mapping = {
        'controlplane': ['control_token', 'controller_token'],
        'runtime-controller': ['controller_token', 'gateway_service_token', 'grant_secret'],
        'dashboard': ['control_token'], 'mcp': ['control_token', 'mcp_token'],
        'inference-gateway': ['gateway_service_token'],
        'credential-broker': ['gateway_service_token'], 'egress-gateway': ['gateway_service_token'],
    }
    for service, keys in mapping.items():
        data = {key: identity[key] for key in keys}
        gateway = service in ['inference-gateway', 'credential-broker', 'egress-gateway']
        if service == 'controlplane':
            data.update(database_url=dsn, **{'database_ca.crt': values(ca)['ca.crt']})
        if service == 'inference-gateway':
            data['subscription_vault_token'] = ''  # Fixture-only until Vault is provisioned.
        save('workbench-egress' if gateway else 'workbench-system', service + '-secrets', data)
    # Public fixture policies contain no upstream account credentials.
    from pathlib import Path
    root = Path(__file__).resolve().parents[3]
    for service, filename in [('inference-gateway', 'inference.json'), ('credential-broker', 'broker.json')]:
        save('workbench-egress', service + '-config', {service + '.json': (root / 'config' / filename).read_text()})
    # The renderer mounts a configuration Secret for every gateway.
    save('workbench-egress', 'egress-gateway-config', {})


if __name__ == '__main__':
    main()
