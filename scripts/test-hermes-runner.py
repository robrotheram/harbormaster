"""Run with guest Python, no network: provider routing and task failure handling."""
import io
import json
import os
from pathlib import Path
import runpy
import tempfile
from unittest.mock import patch

import run_agent

seen = []
answer = {}


class Agent:
    def __init__(self, **kwargs):
        seen.append(kwargs)
        self.provider, self.base_url = kwargs['provider'], kwargs['base_url']

    def run_conversation(self, prompt):
        assert prompt == 'synthetic task'
        return answer


with tempfile.TemporaryDirectory() as directory:
    env = {'HERMES_HOME': directory, 'HERMES_MODEL': 'approved-model',
           'INFERENCE_URL': 'http://127.0.0.1:18081', 'WORKBENCH_INFERENCE_GRANT': 'workspace-test-grant'}
    with patch.dict(os.environ, env), patch.object(run_agent, 'AIAgent', Agent):
        main = runpy.run_path(str(Path(__file__).with_name('hermes-runner.py') if Path(__file__).with_name('hermes-runner.py').exists() else Path('/usr/local/bin/hermes-runner')))['main']
        for flags in ({'completed': True}, {'failed': True}, {'completed': False}, {'error': 'synthetic failure'}):
            answer = {'final_response': 'synthetic response', **flags}
            out = io.StringIO()
            with patch('sys.stdin', io.StringIO('synthetic task')), patch('sys.stdout', out):
                if flags == {'completed': True}:
                    main()
                    assert json.loads(out.getvalue()) == {'result': 'synthetic response', 'is_error': False}
                else:
                    try:
                        main()
                    except RuntimeError:
                        pass
                    else:
                        raise AssertionError('failed engine result was reported as successful')
        assert all(v['api_key'] == 'workspace-test-grant' and v['provider'] == 'openai-codex'
                   and v['base_url'] == 'http://127.0.0.1:18081/codex' and v['fallback_model'] == {} for v in seen)
        assert (Path(directory) / 'config.yaml').stat().st_mode & 0o777 == 0o600
print('PASS: fixed grant/provider route, no fallback, private config, failed and partial task handling')
