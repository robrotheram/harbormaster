#!/opt/hermes/.venv/bin/python
"""Hermes task adapter. Only workspace grants cross the VM boundary."""
import contextlib
import json
import os
import sys

from hermes_common import discover_workbench, enabled_toolsets, configure, emit, record, runtime_identity

def main():
    if sys.argv[1:] == ['--version']:
        with contextlib.redirect_stdout(sys.stderr):
            from run_agent import AIAgent
            from plugins.platforms.matrix.adapter import MatrixAdapter
            from mautrix.crypto import OlmMachine
        print(json.dumps(runtime_identity()))
        return
    home, model, endpoint, grant = configure()
    discover_workbench()
    prompt = sys.stdin.read(65537)
    if not prompt or len(prompt.encode()) > 65536:
        raise ValueError('Invalid task input')
    with contextlib.redirect_stdout(sys.stderr):
        from run_agent import AIAgent
        agent = AIAgent(model=model, provider='openai-codex', api_mode='codex_responses',
                        base_url=endpoint, api_key=grant, max_iterations=24,
                        enabled_toolsets=enabled_toolsets(), quiet_mode=True,
                        tool_start_callback=lambda call_id, name, args: (emit('tool.started', name), record('tool.call', args, tool=name, call_id=call_id)),
                        tool_complete_callback=lambda call_id, name, args, result: (emit('tool.completed', name), record('tool.result', result, tool=name, call_id=call_id)),
                        skip_background_review=True, fallback_model={}, cwd=os.getcwd(), run_budget_seconds=540)
        if agent.base_url.rstrip('/') != endpoint or agent.provider != 'openai-codex':
            raise RuntimeError('Unexpected provider route')
        result = agent.run_conversation(prompt)
    if result.get('failed') or result.get('error') or result.get('completed') is False:
        raise RuntimeError('Hermes did not complete the task')
    record('run.usage', input_tokens=int(result.get('input_tokens') or 0), output_tokens=int(result.get('output_tokens') or 0))
    response = result.get('final_response')
    if not isinstance(response, str) or not response:
        raise RuntimeError('Hermes returned no final response')
    print(json.dumps({'result': response, 'is_error': False}))

if __name__ == '__main__':
    try:
        main()
    except Exception:
        sys.exit(1)
