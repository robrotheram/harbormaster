import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

RENDER = Path(__file__).with_name('render.py')
SERVICES = ['controlplane', 'runtime-controller', 'dashboard', 'mcp',
            'inference-gateway', 'credential-broker', 'egress-gateway', 'workspace']


class ReferenceManifestTests(unittest.TestCase):
    def render(self, image_suffix, allow=False):
        with tempfile.TemporaryDirectory() as directory:
            source, output = Path(directory) / 'images.json', Path(directory) / 'platform.json'
            source.write_text(json.dumps({'images': {name: 'ghcr.io/example/' + name + image_suffix for name in SERVICES}}))
            result = subprocess.run([sys.executable, str(RENDER), '--images', str(source), '--output', str(output), '--gateway-endpoints', str(RENDER.with_name('gateway-endpoints.json'))] + (['--allow-unverified-reference'] if allow else []), capture_output=True, text=True)
            return result, json.loads(output.read_text()) if output.exists() else None

    def test_reference_keeps_execution_gated_and_credentials_separate(self):
        result, manifest = self.render('@sha256:' + 'a' * 64)
        self.assertEqual(result.returncode, 0, result.stderr)
        deployments = [o for o in manifest['items'] if o['kind'] == 'Deployment']
        self.assertEqual(len(deployments), 7)
        for deployment in deployments:
            pod = deployment['spec']['template']['spec']
            name = deployment['metadata']['name']
            self.assertEqual(pod['imagePullSecrets'], [{'name': 'harbormaster-registry'}])
            self.assertEqual(pod['automountServiceAccountToken'], name == 'runtime-controller')
            self.assertFalse(any('hostPath' in v for v in pod['volumes']))
            env = {e['name']: e['value'] for e in pod['containers'][0]['env']}
            if name == 'runtime-controller':
                self.assertEqual(env['REQUIRE_VERIFIED_ISOLATION'], 'true')
            if name == 'dashboard':
                self.assertEqual(env['DASHBOARD_PUBLIC_ORIGIN'], 'https://harbormaster.exceptionerror.io')
                self.assertTrue(env['OAUTH2_PROXY_AUTH_URL'].endswith('/oauth2/auth'))
            if name in ['inference-gateway', 'credential-broker']:
                self.assertEqual(env['GATEWAY_MODE'], 'fixture')
        self.assertFalse(any(o['kind'] == 'Secret' for o in manifest['items']))
        ingress = next(o for o in manifest['items'] if o['kind'] == 'Ingress')
        self.assertIn('harbormaster-auth', ingress['metadata']['annotations']['traefik.ingress.kubernetes.io/router.middlewares'])
        self.assertEqual(ingress['spec']['rules'][0]['http']['paths'][0]['backend']['service']['name'], 'dashboard')

    def test_reference_execution_requires_explicit_opt_in(self):
        result, manifest = self.render('@sha256:' + 'a' * 64, allow=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        runtime = next(o for o in manifest['items'] if o['kind'] == 'Deployment' and o['metadata']['name'] == 'runtime-controller')
        env = {e['name']: e['value'] for e in runtime['spec']['template']['spec']['containers'][0]['env']}
        self.assertEqual(env['REQUIRE_VERIFIED_ISOLATION'], 'false')
        self.assertEqual(env['RUNTIME_BACKEND'], 'kubernetes')

    def test_mutable_images_are_rejected_before_output(self):
        result, manifest = self.render(':edge')
        self.assertNotEqual(result.returncode, 0)
        self.assertIsNone(manifest)


if __name__ == '__main__':
    unittest.main()
