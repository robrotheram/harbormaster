"""Run with the guest's Python, no network: callback gates and failed-sync status."""
import asyncio
import contextlib
import io
import json
import os
from pathlib import Path
import tempfile
from types import SimpleNamespace
from unittest.mock import patch

from plugins.platforms.matrix import adapter as native_matrix
import run_agent


async def check():
    agents, replies, instances = [], [], []

    class Agent:
        def __init__(self, **kwargs):
            agents.append(kwargs)

        def run_conversation(self, text):
            assert text == 'approved message'
            self.tool_start_callback('call-test', 'terminal', {'command':'printf proof'})
            self.tool_complete_callback('call-test', 'terminal', {}, 'proof')
            return {'final_response': 'approved reply','input_tokens':10,'output_tokens':3}

    class Adapter:
        def __init__(self, config):
            assert config.token == 'workspace-test-grant'
            assert config.extra['homeserver'] == 'http://127.0.0.1:18084'
            assert config.extra['e2ee_mode'] == 'required'
            self.disconnected = False
            instances.append(self)

        def set_message_handler(self, handler):
            self.handler = handler

        async def _send_room_message(self, chat_id, content):
            if content['body'] == 'fail delivery':
                raise RuntimeError('mock send failed')
            return '$sent-event'

        async def connect(self):
            def event(room, user):
                return SimpleNamespace(source=SimpleNamespace(chat_id=room, user_id=user), text='approved message')

            assert await self.handler(event('!foreign:example.org', '@alice:example.org')) is None
            assert await self.handler(event('!project:example.org', '@mallory:example.org')) is None
            assert agents == [], 'unauthorized event created model/session state'
            for _ in range(2):
                reply=await self.handler(event('!project:example.org', '@alice:example.org'))
                replies.append(reply)
                await self._send_room_message('!project:example.org', {'body':reply})
            try:
                await self._send_room_message('!project:example.org', {'body':'fail delivery'})
            except RuntimeError:
                pass
            # A revoked token makes the pinned adapter's sync task return.
            self._sync_task = asyncio.create_task(asyncio.sleep(0.01))
            return True

        async def disconnect(self):
            self.disconnected = True

    with tempfile.TemporaryDirectory() as directory:
        env = {
            'HERMES_HOME': directory,
            'HERMES_MODEL': 'approved-model',
            'INFERENCE_URL': 'http://127.0.0.1:18081',
            'WORKBENCH_INFERENCE_GRANT': 'workspace-test-grant',
            'HERMES_MATRIX_CONFIG': json.dumps({'homeserver': 'https://matrix.example.org', 'bot_user_id': '@bot:example.org', 'room_ids': ['!project:example.org'], 'sender_ids': ['@alice:example.org']}),
        }
        captured=io.StringIO()
        with contextlib.redirect_stderr(captured), patch.dict(os.environ, env), patch.object(native_matrix, 'MatrixAdapter', Adapter), patch.object(run_agent, 'AIAgent', Agent):
            from workbench_hermes.matrix import main
            try:
                await main()
            except RuntimeError as error:
                assert str(error) == 'Matrix sync stopped'
            else:
                raise AssertionError('dead sync retained a connected process')
        assert not (Path(directory) / 'matrix-connected').exists()
        assert instances[0].disconnected
        assert replies == ['approved reply','approved reply'] and len(agents) == 1
        frames=[json.loads(line[len('AWB_HISTORY '):]) for line in captured.getvalue().splitlines() if line.startswith('AWB_HISTORY ')]
        users=[f for f in frames if f['kind']=='user.message']
        assert len(users)==2 and users[0]['episode'] != users[1]['episode']
        for user in users:
            episode=[f for f in frames if f['episode']==user['episode']]
            assert any(f['kind']=='tool.call' and 'printf proof' in f['content'] for f in episode)
            assert any(f['kind']=='tool.result' and f['content']=='proof' for f in episode)
            assert any(f['kind']=='message.delivery' and f['status']=='sent' and f['message_id']=='$sent-event' for f in episode)
        assert any(f['kind']=='message.delivery' and f['status']=='failed' for f in frames)
        assert len([f for f in frames if f['kind']=='run.completed'])==2
        assert agents[0]['api_key'] == 'workspace-test-grant'
        assert agents[0]['base_url'] == 'http://127.0.0.1:18081/codex'
        assert agents[0]['provider'] == 'openai-codex'
    print('PASS: Matrix room/sender gate, fixed inference grant route, required encryption, dead-sync cleanup')
