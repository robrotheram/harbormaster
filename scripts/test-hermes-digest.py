"""Pinned SDK result types and stable publication/exchange request contract."""
import asyncio
import importlib.util
from pathlib import Path
from mcp.types import CallToolResult, TextContent

spec = importlib.util.spec_from_file_location('digest', Path(__file__).with_name('hermes-digest.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class Client:
    def __init__(self):
        self.calls = []
        self.notes = {}
    async def call_tool(self, name, arguments):
        self.calls.append((name, arguments))
        if name == 'note_publish':
            write = arguments['write']
            assert write['sources'] and write['expected_digest'] == ''
            prior = self.notes.setdefault(write['occurrence'], write)
            assert prior == write
        return CallToolResult(content=[TextContent(type='text', text='receipt')], is_error=False)

async def test():
    client = Client()
    source = {'body': 'Sourced summary', 'sources': ['https://example.org/source']}
    for _ in range(2):
        await module.digest(client, 'personal', 'workspace', 'fixture-day', source, 'granted-export')
    assert len(client.notes) == 1
    assert client.calls[1] == ('exchange_submit', {'project_id': 'personal', 'export_id': 'granted-export'})
    print('PASS: pinned SDK digest types, stable occurrence, sourced note and explicit exchange')

asyncio.run(test())
