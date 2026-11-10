#!/opt/hermes/.venv/bin/python
"""A deterministic digest job; Workbench owns its schedule and occurrence key.

Run inside the Hermes workspace from a Workbench-owned task. No Hermes cron.
The caller supplies an immutable source fixture and a stable occurrence, including
on retry. An optional work-request export must already have an explicit grant.
"""
import argparse
import asyncio
import json
import os
from pathlib import Path

async def digest(client, project, workspace, occurrence, source, request_export=None):
    async def call(name, arguments):
        result = await client.call_tool(name, arguments)
        if result.is_error:
            raise RuntimeError(str(result.content))
        return result
    receipt = await call('note_publish', {
        'project_id': project, 'workspace_id': workspace,
        'write': {'path': 'digests/' + occurrence + '.md', 'body': source['body'],
                  'sources': source['sources'], 'expected_digest': '', 'occurrence': occurrence}})
    if request_export:
        await call('exchange_submit', {'project_id': project, 'export_id': request_export})
    return receipt

async def main():
    from mcp import ClientSession
    from mcp.client.streamable_http import streamable_http_client
    import httpx2
    parser = argparse.ArgumentParser()
    parser.add_argument('--source', required=True)
    parser.add_argument('--occurrence', required=True)
    parser.add_argument('--request-export')
    args = parser.parse_args()
    # This is a fixed broker capability, never an arbitrary URL from a job file.
    token = os.environ['WORKSPACE_TOKEN']
    async with httpx2.AsyncClient(headers={'Authorization': 'Bearer ' + token}) as http_client, streamable_http_client('http://127.0.0.1:18085/mcp', http_client=http_client) as (read, write, _):
        async with ClientSession(read, write) as session:
            await session.initialize()
            result = await digest(session, os.environ['PROJECT_ID'], os.environ['WORKSPACE_ID'],
                args.occurrence, json.loads(Path(args.source).read_text()), args.request_export)
            print(result.model_dump_json())

if __name__ == '__main__':
    asyncio.run(main())
