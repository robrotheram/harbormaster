#!/opt/hermes/.venv/bin/python
"""Project-bound encrypted Matrix adapter using the official Hermes adapter."""
import asyncio
import hashlib
import json
import os
import signal
import sys
import uuid
import time

from hermes_common import enabled_toolsets, configure, emit, record

async def main():
    from gateway.config import PlatformConfig
    from plugins.platforms.matrix.adapter import MatrixAdapter
    from run_agent import AIAgent
    config = json.loads(os.environ['HERMES_MATRIX_CONFIG'])
    rooms, senders = set(config['room_ids']), set(config['sender_ids'])
    home, model, endpoint, grant = configure()
    os.environ['MATRIX_ALLOWED_ROOMS'] = ','.join(rooms)
    os.environ['MATRIX_ALLOWED_USERS'] = ','.join(senders)
    os.environ['MATRIX_REQUIRE_MENTION'] = 'false'
    adapter = MatrixAdapter(PlatformConfig(enabled=True, token=grant, typing_indicator=False,
        gateway_restart_notification=False, extra={'homeserver': 'http://127.0.0.1:18084',
        'user_id': config['bot_user_id'], 'e2ee_mode': 'required', 'auto_thread': False}))
    lock = asyncio.Lock()
    agents = {}
    pending_context = {}
    if hasattr(adapter, '_send_room_message'):
        original_send = adapter._send_room_message
        async def recorded_send(chat_id, content):
            context = pending_context.get(chat_id)
            try:
                message_id = await original_send(chat_id, content)
            except Exception:
                if context:
                    record('message.delivery', content.get('body', ''), status='failed', **context)
                raise
            if context:
                record('message.delivery', content.get('body', ''), status='sent', message_id=str(message_id), **context)
            return message_id
        adapter._send_room_message = recorded_send

    async def message(event):
        source = event.source
        # Defense in depth; the host broker filters before the SDK receives events.
        if source.chat_id not in rooms or source.user_id not in senders:
            return None
        text = getattr(event, 'text', '')
        if not isinstance(text, str) or not text or len(text.encode()) > 65536:
            return None
        context = {'episode': 'ep_' + uuid.uuid4().hex, 'room_id': source.chat_id, 'sender_id': source.user_id}
        record('user.message', text, message_id=str(getattr(event, 'message_id', '') or ''), **context)
        emit('message.received')
        async with lock:
            pending_context[source.chat_id] = context
            started = time.monotonic()
            record('run.started', status='running', **context)
            emit('response.started')
            key = source.chat_id + '\0' + source.user_id
            if key not in agents:
                agents[key] = AIAgent(model=model, provider='openai-codex', api_mode='codex_responses',
                    base_url=endpoint, api_key=grant, max_iterations=24,
                    enabled_toolsets=enabled_toolsets(), quiet_mode=True,
                    tool_start_callback=lambda call_id, name, *args: emit('tool.started', name),
                    tool_complete_callback=lambda call_id, name, *args: emit('tool.completed', name),
                    skip_background_review=True, fallback_model={}, cwd=os.getcwd(),
                    run_budget_seconds=540, session_id='matrix-' + hashlib.sha256(key.encode()).hexdigest())
            agents[key].tool_start_callback = lambda call_id, name, args: (emit('tool.started', name), record('tool.call', args, tool=name, call_id=call_id, **context))
            agents[key].tool_complete_callback = lambda call_id, name, args, result: (emit('tool.completed', name), record('tool.result', result, tool=name, call_id=call_id, **context))
            try:
                result = await asyncio.to_thread(agents[key].run_conversation, text)
            except Exception:
                record('run.failed', status='failed', elapsed_ms=int((time.monotonic()-started)*1000), **context)
                emit('response.failed')
                return 'Hermes could not complete this request. Check the workspace provider connection and retry.'
            if result.get('failed') or result.get('error') or result.get('completed') is False:
                record('run.failed', status='failed', elapsed_ms=int((time.monotonic()-started)*1000), **context)
                emit('response.failed')
                return 'Hermes could not complete this request. Check the workspace provider connection and retry.'
            record('assistant.message', result.get('final_response', ''), status='prepared', **context)
            record('run.completed', status='succeeded', elapsed_ms=int((time.monotonic()-started)*1000), input_tokens=int(result.get('input_tokens') or 0), output_tokens=int(result.get('output_tokens') or 0), **context)
            emit('response.completed')
            return result.get('final_response')

    adapter.set_message_handler(message)
    if not await adapter.connect():
        raise RuntimeError('Matrix authentication or required encryption unavailable')
    marker = home / 'matrix-connected'
    marker.write_text('connected')
    emit('service.connected')
    stop = asyncio.Event()
    for sig in (signal.SIGTERM, signal.SIGINT):
        asyncio.get_running_loop().add_signal_handler(sig, stop.set)
    stopping = asyncio.create_task(stop.wait())
    try:
        # The pinned adapter stops its sync task on revoked credentials. Do
        # not leave a live process/Connected marker after that task has exited.
        if adapter._sync_task is None:
            raise RuntimeError('Matrix sync did not start')
        await asyncio.wait([stopping, adapter._sync_task], return_when=asyncio.FIRST_COMPLETED)
        if not stop.is_set():
            raise RuntimeError('Matrix sync stopped')
    finally:
        stopping.cancel()
        marker.unlink(missing_ok=True)
        await adapter.disconnect()

if __name__ == '__main__':
    try:
        asyncio.run(main())
    except Exception:
        sys.exit(1)
