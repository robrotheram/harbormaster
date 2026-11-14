#!/usr/bin/python3
"""Launch persistent application processes through the private workspace supervisor."""
import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='action', required=True)
    start = commands.add_parser('start')
    start.add_argument('--directory', default='.')
    start.add_argument('--port', type=int, required=True)
    start.add_argument('--command', required=True)
    start.add_argument('--key', default=None, help='Reuse this key when retrying a start')
    commands.add_parser('status')
    stop = commands.add_parser('stop')
    stop.add_argument('--id', required=True)
    args = parser.parse_args()
    token = os.environ.get('WORKBENCH_WORKSPACE_GRANT')
    if not token:
        parser.error('Run this command inside an agent workspace.')
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def call(path, body=None):
        request = urllib.request.Request('http://127.0.0.1:8080/v1/' + path,
            data=None if body is None else json.dumps(body).encode(),
            headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
        with opener.open(request, timeout=20) as response:
            return json.load(response)

    try:
        if args.action == 'stop':
            if not args.id.startswith('service_') or not args.id.replace('_', '').isalnum():
                parser.error('Invalid service ID')
            print(json.dumps(call('services/' + args.id + '/revoke', {})))
            return
        if args.action == 'start':
            app = call('services', {'application': {'directory': args.directory,
                'command': args.command, 'port': args.port}, 'ttl_seconds': 3600,
                'idempotency_key': args.key or 'agent-preview-' + uuid.uuid4().hex})
            for _ in range(30):
                items = call('services')['items']
                app = next((item for item in items if item['id'] == app['id']), app)
                if app.get('ready') or app['state'] != 'active':
                    break
                time.sleep(.5)
            print(json.dumps({k: v for k, v in app.items() if k != 'logs'}))
            if not app.get('ready'):
                print('Application is not ready. Inspect application logs in the workbench.', file=sys.stderr)
                sys.exit(1)
            print('Application is supervised inside this workspace. Open Web preview in the work conversation.')
        else:
            print(json.dumps({'items': [{k: v for k, v in app.items() if k != 'logs'} for app in call('services')['items']]}))
    except urllib.error.HTTPError as error:
        try:
            code = json.load(error).get('code', 'REQUEST_FAILED')
        except (ValueError, AttributeError):
            code = 'REQUEST_FAILED'
        print('Preview request failed: ' + str(code), file=sys.stderr)
        sys.exit(1)
    except (urllib.error.URLError, TimeoutError):
        print('Workspace supervisor is unavailable.', file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()
