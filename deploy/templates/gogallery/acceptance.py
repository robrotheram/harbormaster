"""Disposable noNIC acceptance: python3 deploy/templates/gogallery/acceptance.py.
Only trusted image assets enter guests. Probe processes require disabling process
inventory; recipe process supervision and resource budgets remain enabled.
"""
import base64
import json
import subprocess
import time

containers = []
probe = '''import urllib.request,urllib.error,sys
r=urllib.request.Request('http://127.0.0.1:8080'+sys.argv[2],data=sys.stdin.buffer.read() if sys.argv[1]=='POST' else None,method=sys.argv[1],headers={'Authorization':'Bearer acceptance-only','Content-Type':'application/json'})
try:
 with urllib.request.urlopen(r,timeout=30) as f: print(f.status);sys.stdout.flush();sys.stdout.buffer.write(f.read())
except urllib.error.HTTPError as e:
 print(e.code);sys.stdout.flush();sys.stdout.buffer.write(e.read())
'''


def request(cid, method, path, body=None):
    raw = subprocess.check_output(['docker', 'exec', '-i', cid, 'python3', '-c', probe, method, path], input=json.dumps(body or {}).encode())
    status, data = raw.split(b'\n', 1)
    return int(status), data


def call(cid, method, path, body=None, expected=200):
    status, data = request(cid, method, path, body)
    assert status == expected, (status, data)
    return json.loads(data)


try:
    for name in ('coding', 'retained'):
        cid = subprocess.check_output(['docker', 'run', '-d', '--network', 'none', '--memory', '1g', '--cpus', '1', '--no-healthcheck', '-e', 'WORKSPACE_TOKEN=acceptance-only', '-e', 'MONITOR_PROCESSES=false', '-e', f'WORKSPACE_ID=ws_{name}', 'agent-workbench-workspace:gogallery-v1'], text=True).strip()
        containers.append(cid)
        for _ in range(60):
            try:
                call(cid, 'GET', '/v1/activity')
                break
            except subprocess.CalledProcessError:
                time.sleep(1)
        else:
            raise RuntimeError('supervisor did not become ready')
    coding, retained = containers
    job = call(coding, 'POST', '/v1/recipe-jobs', {'template': 'gogallery-v1', 'idempotency_key': 'acceptance-gallery', 'timeout_seconds': 1800}, 202)
    for _ in range(1800):
        job = call(coding, 'GET', '/v1/recipe-jobs/' + job['id'])
        if job['state'] != 'running':
            break
        time.sleep(1)
    if job['state'] != 'succeeded':
        for artifact in call(coding, 'GET', '/v1/artifacts')['items']:
            if artifact['source_path'].endswith('/execution.log'):
                print(request(coding, 'GET', '/v1/artifacts/' + artifact['id'] + '/content')[1].decode(errors='replace'))
        raise AssertionError(job)
    artifacts = call(coding, 'GET', '/v1/artifacts')['items']
    archive = next(a for a in artifacts if a['source_path'].endswith('/gallery.tar'))
    status, snapshot = request(coding, 'GET', '/v1/artifacts/' + archive['id'] + '/content')
    assert status == 200
    payload = {'snapshot': base64.b64encode(snapshot).decode(), 'ttl_seconds': 60}
    service = call(retained, 'POST', '/v1/services', payload, 201)
    subprocess.check_call(['docker', 'stop', coding], stdout=subprocess.DEVNULL)
    path = '/v1/services/' + service['id']
    content = call(retained, 'POST', path + '/fetch', {'path': 'index.html'})
    assert base64.b64decode(content['content'])
    call(retained, 'POST', path + '/revoke')
    assert request(retained, 'POST', path + '/fetch', {'path': 'index.html'})[0] == 410
    payload['ttl_seconds'] = 1
    expiring = call(retained, 'POST', '/v1/services', payload, 201)
    time.sleep(2)
    call(retained, 'POST', '/v1/services/reconcile')
    assert request(retained, 'POST', '/v1/services/' + expiring['id'] + '/fetch', {'path': 'index.html'})[0] == 410
    print(json.dumps({'job': job, 'archived_artifacts': artifacts, 'coding_stopped_preview_available': True, 'revocation': True, 'expiry': True}))
finally:
    for cid in containers:
        subprocess.run(['docker', 'stop', cid], check=False, stdout=subprocess.DEVNULL)
        subprocess.run(['docker', 'container', 'rm', cid], check=False, stdout=subprocess.DEVNULL)
