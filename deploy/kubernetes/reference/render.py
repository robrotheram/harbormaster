#!/usr/bin/env python3
"""Render the fresh k3s reference environment from a resolved image lock file."""
import argparse
import json
import subprocess
import sys
import tempfile
from pathlib import Path

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--images', type=Path, required=True)
p.add_argument('--output', type=Path, required=True)
p.add_argument('--gateway-endpoints', type=Path, required=True)
p.add_argument('--allow-unverified-reference', action='store_true', help='Enable this non-production reference runtime after live acceptance; never claims verified isolation')
a = p.parse_args()
config = {
    'project_id': 'default', 'project_namespace': 'workbench-reference',
    'storage_class': 'harbormaster-retain', 'host': 'harbormaster.exceptionerror.io',
    'tls_secret': 'harbormaster-tls', 'ingress_class': 'traefik',
    'image_pull_secrets': ['harbormaster-registry'],
    'images': json.loads(a.images.read_text())['images'],
    'auth_annotations': {
        'traefik.ingress.kubernetes.io/router.entrypoints': 'websecure',
        'traefik.ingress.kubernetes.io/router.tls': 'true',
        'traefik.ingress.kubernetes.io/router.middlewares': 'oauth2-proxy-oauth-redirect@kubernetescrd,workbench-system-harbormaster-auth@kubernetescrd',
    },
    'environment': {
        'dashboard': {
            'DASHBOARD_PUBLIC_ORIGIN': 'https://harbormaster.exceptionerror.io',
            'OAUTH2_PROXY_AUTH_URL': 'http://oauth2-proxy.oauth2-proxy.svc.cluster.local:80/oauth2/auth',
            'OAUTH2_PROXY_PUBLIC_URL': 'https://oidc.exceptionerror.io',
        },
        'controlplane': {'MEMORY_QUOTA_BYTES': '2147483648'},
        'runtime-controller': {'DEPLOYMENT_ID': 'harbormaster', 'REQUIRE_VERIFIED_ISOLATION': 'false' if a.allow_unverified_reference else 'true'},
        'inference-gateway': {'GATEWAY_MODE': 'fixture'},
        'credential-broker': {'GATEWAY_MODE': 'fixture'},
        'egress-gateway': {'GATEWAY_MODE': 'fixture'},
    },
}
# Use the trusted gateway Service IPs: project policy intentionally blocks the
# cluster's general recursive DNS. This permits fixed gateways without granting
# external DNS access to workspaces. Re-resolve if Services are recreated.
endpoints = json.loads(a.gateway_endpoints.read_text())
expected = {'KUBERNETES_INFERENCE_URL', 'KUBERNETES_BROKER_URL', 'KUBERNETES_EGRESS_URL'}
if set(endpoints) != expected:
    p.error('gateway endpoint file must contain exactly the three runtime gateway URLs')
from urllib.parse import urlparse
from ipaddress import ip_address
for value in endpoints.values():
    parsed = urlparse(value)
    if parsed.scheme != 'http' or parsed.port != 8080 or parsed.path or parsed.query or parsed.fragment or parsed.username:
        p.error('gateway endpoints must be http://SERVICE_IP:8080')
    ip_address(parsed.hostname)
config['environment']['runtime-controller'].update(endpoints)
# Do not open execution simply because a RuntimeClass object exists. Change this
# setting only after an operator explicitly accepts an unverified test runtime.
with tempfile.TemporaryDirectory() as directory:
    source = Path(directory) / 'config.json'
    source.write_text(json.dumps(config))
    subprocess.run([sys.executable, str(Path(__file__).resolve().parents[1] / 'render_platform.py'),
                    str(source), '--output', str(a.output)], check=True)
# Preserve the control plane's internal service names for health observations.
manifest = json.loads(a.output.read_text())
for name in ['inference-gateway', 'credential-broker', 'egress-gateway']:
    manifest['items'].append({'apiVersion': 'v1', 'kind': 'Service',
        'metadata': {'name': name, 'namespace': 'workbench-system'},
        'spec': {'type': 'ExternalName', 'externalName': name + '.workbench-egress.svc.cluster.local'}})
a.output.write_text(json.dumps(manifest, indent=2) + '\n')
