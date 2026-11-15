import asyncio
import unittest
from types import SimpleNamespace
from workbench_hermes import consumer as module

class Client:
    def __init__(self):
        self.work, self.attempts = {}, {}
    async def call_tool(self, name, args):
        if name == 'exchange_inbox':
            data = {'items': [{'id': 'new' if args['after'] else 'old', 'operation': 'work-request'}],
                    'next': '' if args['after'] else 'cursor'}
        elif name == 'exchange_submit':
            if args['export_id'] == 'old':
                raise RuntimeError('transient creation failure')
            data = self.work.setdefault(args['export_id'], {'id': 'stable-work'})
        elif name == 'work_run':
            data = self.attempts.setdefault(args['idempotency_key'], {'id': 'stable-attempt', 'state': 'queued'})
        return SimpleNamespace(is_error=False, structured_content=data)

async def test():
    client = Client()
    first = await module.consume(client, 'project')
    second = await module.consume(client, 'project')
    assert first == second
    assert len(first['failures']) == 1
    assert len(client.work) == len(client.attempts) == 1
    assert first['selected_status'][0]['export_id'] == 'new'
    print('PASS: paginated consumer, failed old item, stable work and attempt')

class ConsumerContract(unittest.TestCase):
    def test_paginated_retry(self):
        asyncio.run(test())
