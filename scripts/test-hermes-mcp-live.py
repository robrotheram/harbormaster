"""Invoked by the Go broker fixture against the real pinned Hermes installation."""
import json
import os
from pathlib import Path
import tempfile

with tempfile.TemporaryDirectory() as home:
    os.environ['HERMES_HOME'] = home
    Path(home, 'config.yaml').write_text(json.dumps({'mcp_servers': {'workbench': {
        'url': os.environ['TEST_MCP_ENDPOINT'],
        'headers': {'Authorization': 'Bearer ' + os.environ['TEST_MCP_GRANT']},
        'tools': {'include': ['work_list', 'note_publish']}}}}))
    from tools.mcp_tool_discovery import discover_mcp_tools, mcp_servers_ready
    from tools.registry import registry
    discover_mcp_tools(['workbench'])
    assert 'workbench' in mcp_servers_ready()
    names = registry.get_tool_names_for_toolset('mcp-workbench')
    assert len(names) == 2, names
    name = next(name for name in names if name.endswith('work_list'))
    result = registry.get_entry(name).handler({'project_id': 'personal'})
    assert 'error' not in str(result).lower(), result
    print('Pinned Hermes discovery and invocation passed')
