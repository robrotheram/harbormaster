import asyncio
import unittest
from mcp.types import CallToolResult, TextContent
from workbench_hermes import digest as module

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
    source = {'body': 'Sourced summary', 'sources': ['https://example.org/source'], 'request': {'kind': 'research'}}
    for _ in range(2):
        await module.digest(client, 'personal', 'workspace', 'fixture-day', source, 'granted-export')
    assert len(client.notes) == 1
    assert client.calls[1][0] == 'exchange_publish'
    assert client.calls[1][1]['exchange_id'] == 'granted-export'
    print('PASS: pinned SDK digest types, stable occurrence, sourced note and explicit exchange')

class DigestContract(unittest.TestCase):
    def test_stable_publication(self):
        asyncio.run(test())
