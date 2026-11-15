"""Package data must match the authoritative runtime contracts."""
import json
from pathlib import Path
import unittest
from workbench_hermes import common

class PackageContracts(unittest.TestCase):
    def test_packaged_contracts(self):
        root = Path(__file__).resolve().parents[4]
        package = Path(common.__file__).parent
        for packaged, source in [('project-tools.json', 'internal/shared/project-tools.json'), ('hermes-runtime.json', 'deploy/hermes/runtime.json')]:
            self.assertEqual(json.loads((package / packaged).read_text()), json.loads((root / source).read_text()))
