#!/opt/hermes/.venv/bin/python
"""Deterministic project consumer; invoke only from a Workbench-owned task."""
import asyncio
import json
import os
from hermes_common import workbench_endpoint

def payload(result):
    if result.is_error:
        raise RuntimeError(str(result.content))
    if result.structured_content is not None:
        return result.structured_content
    return json.loads(next(c.text for c in result.content if c.type == 'text'))

async def consume(client, project):
    async def call(name, **args):
        return payload(await client.call_tool(name, dict(project_id=project, **args)))
    after = ''
    outcomes, failures = [], []
    while True:
        page = await call('exchange_inbox', after=after, limit=50)
        for item in page['items']:
            if item['operation'] != 'work-request':
                continue
            try:
                # Submit again even with a receipt: it rechecks grant and factory
                # authorization. A crash before run is repaired on the next scan.
                work = await call('exchange_submit', export_id=item['id'])
                attempt = await call('work_run', id=work['id'],
                                     idempotency_key='exchange-' + item['id'])
                outcomes.append({'export_id': item['id'], 'work_id': work['id'],
                                 'attempt_id': attempt['id'], 'state': attempt['state']})
            except Exception as exc:
                failures.append({'export_id': item['id'], 'error': str(exc)})
        after = page['next']
        if not after:
            break
    return {'selected_status': outcomes, 'failures': failures}

async def main():
    from mcp import ClientSession
    from mcp.client.streamable_http import streamable_http_client
    import httpx2
    async with httpx2.AsyncClient(headers={'Authorization': 'Bearer ' + os.environ['WORKSPACE_TOKEN']}) as http_client:
        async with streamable_http_client(workbench_endpoint(), http_client=http_client) as (read, write):
            async with ClientSession(read, write) as session:
                await session.initialize()
                result = await consume(session, os.environ['PROJECT_ID'])
                print(json.dumps(result, sort_keys=True))
                if result['failures']:
                    raise SystemExit(1)

if __name__ == '__main__':
    asyncio.run(main())
