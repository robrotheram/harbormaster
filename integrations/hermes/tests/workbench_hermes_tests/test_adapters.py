"""Portable adapter contracts: stdlib only, no provider, SDK or network.

Guest tests import the real pinned SDK when run inside the guest. Minimal import
stubs let the same routing and failure assertions run locally and in CI.
"""
import contextlib
import io
import json
from pathlib import Path
import asyncio
import sys
from types import ModuleType, SimpleNamespace
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[4]


def sdk_stubs():
    modules = {name: ModuleType(name) for name in (
        'run_agent', 'gateway', 'gateway.config', 'plugins',
        'plugins.platforms', 'plugins.platforms.matrix',
        'plugins.platforms.matrix.adapter')}
    modules['run_agent'].AIAgent = object
    modules['gateway.config'].PlatformConfig = SimpleNamespace
    modules['plugins.platforms.matrix.adapter'].MatrixAdapter = object
    return modules


class AdapterContracts(unittest.TestCase):
    def test_runner_routing_and_task_failures(self):
        with patch.dict(sys.modules, sdk_stubs()), contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            from .runner_contract import check
            check()

    def test_matrix_authorization_delivery_and_sync_failure(self):
        with patch.dict(sys.modules, sdk_stubs()), contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            from .matrix_contract import check
            asyncio.run(check())

    def test_staged_identity_comes_from_manifest(self):
        from workbench_hermes import common as hermes_common
        manifest = json.loads((ROOT / 'deploy/hermes/runtime.json').read_text())
        with patch.object(hermes_common.Path, 'read_text', return_value=json.dumps(manifest)):
            self.assertEqual(hermes_common.runtime_identity(), {
                'version': manifest['version'], 'commit': manifest['commit']})


if __name__ == '__main__':
    unittest.main()
