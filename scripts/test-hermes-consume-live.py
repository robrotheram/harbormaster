"""Exercise real pinned MCP types and the project assistant consumer."""
import asyncio
import importlib.util
import os
from pathlib import Path
from mcp import ClientSession
from mcp.client.streamable_http import streamable_http_client
import httpx2

spec = importlib.util.spec_from_file_location('consumer', Path(__file__).with_name('hermes-consume.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

async def main():
    async with httpx2.AsyncClient(headers={'Authorization': 'Bearer ' + os.environ['TEST_MCP_GRANT']}) as client:
        async with streamable_http_client(os.environ['TEST_MCP_ENDPOINT'], http_client=client) as (read, write):
            async with ClientSession(read, write) as session:
                await session.initialize()
                first = await module.consume(session, os.environ['TEST_PROJECT_ID'])
                second = await module.consume(session, os.environ['TEST_PROJECT_ID'])
                assert not first['failures'] and not second['failures'], (first, second)
                assert len(first['selected_status']) == 1
                keys = lambda result: [(x['export_id'], x['work_id'], x['attempt_id']) for x in result['selected_status']]
                assert keys(first) == keys(second), 'Consumer repeated work/attempt'
                print('PASS: real inbox discovery, submission and stable attempt')

asyncio.run(main())
