"""Shared policy and framing for the pinned workspace-local Hermes adapters."""
import json
import os
from pathlib import Path
import sys
import tempfile


def record(kind, value='', **metadata):
    text = value if isinstance(value, str) else json.dumps(value, default=str)
    raw = text.encode('utf-8')
    truncated = len(raw) > 65536
    if truncated:
        text = raw[:65536].decode('utf-8', errors='ignore')
    print('AWB_HISTORY ' + json.dumps({'schema_version': 1, 'kind': kind, 'content': text,
          'truncated': truncated, **metadata}), file=sys.stderr, flush=True)


def emit(kind, tool=None):
    print('AWB_EVENT ' + json.dumps({'schema_version': 1, 'kind': kind, 'tool': tool or ''}),
          file=sys.stderr, flush=True)


def configure():
    """Write the private no-fallback policy and return the approved route."""
    home = Path(os.environ['HERMES_HOME'])
    home.mkdir(parents=True, exist_ok=True)
    model = os.environ['HERMES_MODEL']
    endpoint = os.environ['INFERENCE_URL'].rstrip('/') + '/codex'
    grant = os.environ['WORKBENCH_INFERENCE_GRANT']
    config = {
        'model': {'default': model, 'provider': 'openai-codex',
                  'base_url': endpoint, 'context_length': 128000},
        'fallback_models': [], 'fallback_model': {},
        'auxiliary': {'vision': {'provider': 'main'},
                      'compression': {'provider': 'main'},
                      'title': {'provider': 'main'}},
        'terminal': {'backend': 'local', 'cwd': os.getcwd()},
        'gateway': {'trust_env': False},
    }
    with tempfile.NamedTemporaryFile(mode='w', dir=home, delete=False) as stream:
        temporary = Path(stream.name)
        try:
            json.dump(config, stream)
            stream.flush()
            os.replace(temporary, home / 'config.yaml')
        finally:
            temporary.unlink(missing_ok=True)
    return home, model, endpoint, grant


def runtime_identity():
    """The staged manifest also pins the image source and host admission record."""
    manifest = json.loads(Path(__file__).with_name('hermes-runtime.json').read_text())
    return {key: manifest[key] for key in ('version', 'commit')}
