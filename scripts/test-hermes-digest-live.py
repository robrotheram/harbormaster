"""Real pinned MCP client against the composed control plane and private guest."""
import asyncio
import importlib.util
import os
from pathlib import Path
from mcp import ClientSession
from mcp.client.streamable_http import streamable_http_client
import httpx2

spec = importlib.util.spec_from_file_location('digest', Path(__file__).with_name('hermes-digest.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

async def main():
    headers = {'Authorization': 'Bearer ' + os.environ['TEST_MCP_GRANT']}
    async with httpx2.AsyncClient(headers=headers) as client:
        async with streamable_http_client(os.environ['TEST_MCP_ENDPOINT'], http_client=client) as (read, write):
            async with ClientSession(read, write) as session:
                await session.initialize()
                source = {'body': 'Sourced summary', 'sources': ['https://example.org/paper']}
                results = []
                for _ in range(2):
                    result = await module.digest(session, os.environ['TEST_PROJECT_ID'],
                        os.environ['TEST_WORKSPACE_ID'], 'task-fixture-one', source)
                    results.append(result.model_dump())
                assert results[0] == results[1], 'Publication retry changed receipt'
                print('PASS: real MCP digest publication and durable retry')

asyncio.run(main())
