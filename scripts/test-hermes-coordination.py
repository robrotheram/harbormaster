#!/usr/bin/env python3
"""Project-only MCP configuration contracts; no Hermes/network required."""
import base64
import json
import os
from pathlib import Path
import tempfile
import time
import unittest
from unittest.mock import patch
from hermes_common import configure, enabled_toolsets

class CoordinationConfig(unittest.TestCase):
    def test_project_mcp_opt_in(self):
        with tempfile.TemporaryDirectory() as directory:
            credential = Path(directory) / 'grant'
            payload = base64.urlsafe_b64encode(json.dumps({'project_id': 'personal',
                'aud': 'workbench-project', 'exp': int(time.time()) + 60}).encode()).decode().rstrip('=')
            credential.write_text('wpa1.' + payload + '.signature-verified-by-ingress')
            env = {'HERMES_HOME': directory, 'HERMES_MODEL': 'model', 'INFERENCE_URL': 'https://inference',
                   'WORKBENCH_INFERENCE_GRANT': 'inference-only', 'WORKBENCH_PROJECT_ID': 'personal',
                   'WORKBENCH_MCP_URL': 'https://workbench/mcp', 'WORKBENCH_MCP_TOKEN_FILE': str(credential)}
            with patch.dict(os.environ, env, clear=True):
                configure()
                config = json.loads((Path(directory) / 'config.yaml').read_text())
                self.assertIn('mcp-workbench', enabled_toolsets())
                self.assertNotIn('workspace_create', config['mcp_servers']['workbench']['tools']['include'])
                os.environ['WORKBENCH_PROJECT_ID'] = 'foreign'
                with self.assertRaises(ValueError):
                    configure()
                credential.write_text('management-token')
                with self.assertRaises(ValueError):
                    configure()

if __name__ == '__main__':
    unittest.main()
