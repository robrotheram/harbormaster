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
    mcp_url = os.environ.get('WORKBENCH_MCP_URL')
    token_file = os.environ.get('WORKBENCH_MCP_TOKEN_FILE')
    if mcp_url or token_file:
        if not mcp_url or not token_file:
            raise ValueError('MCP URL and project credential file required together')
        workbench_endpoint()
        token = Path(token_file).read_text().strip()
        if len(token) != 64 or any(c not in '0123456789abcdef' for c in token) or not os.environ.get('WORKBENCH_PROJECT_ID'):
            raise ValueError('Missing generation grant or project')
        config['mcp_servers'] = {'workbench': {'url': mcp_url,
            'headers': {'Authorization': 'Bearer ' + token},
            'tools': {'include': project_tool_names()}}}
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


def enabled_toolsets():
    tools = ['terminal', 'file', 'memory']
    if os.environ.get('WORKBENCH_MCP_URL'):
        tools.append('mcp-workbench')
    return tools


def discover_workbench():
    """Use pinned Hermes discovery, which registers the mcp-workbench toolset."""
    if os.environ.get('WORKBENCH_MCP_URL'):
        from tools.mcp_tool_discovery import discover_mcp_tools, mcp_servers_ready
        discover_mcp_tools(['workbench'])
        if 'workbench' not in mcp_servers_ready():
            raise RuntimeError('Workbench MCP discovery unavailable')


def workbench_endpoint():
    """Fixed runtime-owned routes for deterministic assistant commands."""
    endpoint = os.environ.get('WORKBENCH_MCP_URL', 'http://127.0.0.1:18085/mcp')
    if endpoint not in ('http://127.0.0.1:18085/mcp', 'http://hermes-mcp.harbormaster.svc:8080/hermes-mcp'):
        raise ValueError('Unsupported workspace MCP transport')
    return endpoint
def project_tool_names():
    return json.loads(Path(__file__).with_name('project-tools.json').read_text())
