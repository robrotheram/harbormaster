#!/usr/bin/env python3
"""Check host signing-key provisioning without root or touching a live service."""
import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('installer', Path(__file__).with_name('firecracker-install.py'))
installer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)


class ProvisioningTests(unittest.TestCase):
    def test_uses_existing_authority_with_private_root_ownership(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            source = root / '.local/secrets/control_token'
            source.parent.mkdir(parents=True)
            source.write_text('synthetic-existing-authority-key-' * 2)
            destination = root / 'host-secrets'
            destination.mkdir()
            with patch.object(installer, 'ROOT', root), patch.object(installer.os, 'chown') as chown:
                installer.install_project_authority(destination)
            target = destination / 'project_mcp_signing_key'
            self.assertEqual(target.read_bytes(), source.read_bytes())
            self.assertEqual(target.stat().st_mode & 0o777, 0o600)
            chown.assert_called_once_with(target, 0, 0)

    def test_invalid_authority_does_not_replace_installed_key(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            source = root / '.local/secrets/control_token'
            source.parent.mkdir(parents=True)
            destination = root / 'host-secrets'
            destination.mkdir()
            target = destination / 'project_mcp_signing_key'
            target.write_text('retain-existing-key')
            for invalid in [None, '', 'short']:
                if invalid is not None:
                    source.write_text(invalid)
                with patch.object(installer, 'ROOT', root), self.assertRaises(SystemExit):
                    installer.install_project_authority(destination)
                self.assertEqual(target.read_text(), 'retain-existing-key')


if __name__ == '__main__':
    unittest.main()
