"""A deterministic digest job; Workbench owns its schedule and occurrence key.

Run inside the Hermes workspace from a Workbench-owned task. No Hermes cron.
The caller supplies an immutable source fixture and a stable occurrence, including
on retry. An optional work-request export must already have an explicit grant.
"""
import argparse
import asyncio
import json
import os
from .common import workbench_endpoint
import re
from pathlib import Path

async def digest(client, project, workspace, occurrence, source, request_grant=None):
    if not re.fullmatch(r"[A-Za-z0-9_-]{1,100}", occurrence):
        raise ValueError("Invalid digest occurrence")
    async def call(name, arguments):
        result = await client.call_tool(name, arguments)
        if result.is_error:
            raise RuntimeError(str(result.content))
        return result
    receipt = await call('note_publish', {
        'project_id': project, 'workspace_id': workspace,
        'write': {'path': 'digest-' + occurrence + '.md', 'body': source['body'],
                  'sources': source['sources'], 'expected_digest': '', 'occurrence': occurrence}})
    if request_grant:
        await call('exchange_publish', {'project_id': project, 'exchange_id': request_grant,
            'occurrence': occurrence, 'body': json.dumps(source['request'], sort_keys=True)})
    return receipt

async def main():
    from mcp import ClientSession
    from mcp.client.streamable_http import streamable_http_client
    import httpx2
    parser = argparse.ArgumentParser()
    parser.add_argument('--source', required=True)
    parser.add_argument('--occurrence', default=os.environ.get('WORKBENCH_TASK_ID'))
    parser.add_argument('--request-grant')
    args = parser.parse_args()
    if not args.occurrence:
        parser.error("--occurrence or WORKBENCH_TASK_ID is required")
    # This is a fixed broker capability, never an arbitrary URL from a job file.
    token = os.environ['WORKSPACE_TOKEN']
    async with httpx2.AsyncClient(headers={'Authorization': 'Bearer ' + token}) as http_client, streamable_http_client(workbench_endpoint(), http_client=http_client) as (read, write):
        async with ClientSession(read, write) as session:
            await session.initialize()
            result = await digest(session, os.environ['PROJECT_ID'], os.environ['WORKSPACE_ID'],
                args.occurrence, json.loads(Path(args.source).read_text()), args.request_grant)
            print(result.model_dump_json())

if __name__ == '__main__':
    asyncio.run(main())
