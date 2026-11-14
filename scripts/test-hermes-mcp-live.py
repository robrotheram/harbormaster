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
        'tools': {'include': ['work_list', 'note_publish', 'board_list', 'board_update']}}}}))
    from tools.mcp_tool_discovery import discover_mcp_tools, mcp_servers_ready
    from tools.registry import registry
    discover_mcp_tools(['workbench'])
    assert 'workbench' in mcp_servers_ready()
    names = registry.get_tool_names_for_toolset('mcp-workbench')
    assert len(names) == 4, names
    name = next(name for name in names if name.endswith('work_list'))
    result = registry.get_entry(name).handler({'project_id': os.environ.get('TEST_MCP_PROJECT', 'personal')})
    assert 'error' not in str(result).lower(), result
    board = next(name for name in names if name.endswith('board_list'))
    update = next(name for name in names if name.endswith('board_update'))
    assert 'description' in registry.get_entry(update).schema['parameters']['properties']
    result = registry.get_entry(board).handler({'project_id': os.environ.get('TEST_MCP_PROJECT', 'personal')})
    assert 'Invalid structured content' not in str(result), result
    if isinstance(result, dict):
        assert not result.get('error') and not result.get('is_error'), result
    print('Pinned Hermes discovery, description schema and board invocation passed')
