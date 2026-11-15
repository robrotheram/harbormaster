#!/usr/bin/env python3
"""Render a single-owner, single-project production deployment as Kubernetes JSON.
Input is an operator-owned JSON file; secrets are pre-created, never rendered.
"""
import argparse
import json
import re
from pathlib import Path

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('config', type=Path)
p.add_argument('--output', type=Path, required=True)
a = p.parse_args()
c = json.loads(a.config.read_text())
for key in ('project_id', 'project_namespace', 'storage_class', 'host', 'tls_secret'):
    if not re.fullmatch(r'[a-zA-Z0-9._-]+', c[key]):
        p.error('invalid ' + key)
services = ['controlplane', 'runtime-controller', 'dashboard', 'mcp', 'inference-gateway', 'credential-broker', 'egress-gateway']
for name in services + ['workspace']:
    if not re.fullmatch(r'.+@sha256:[a-f0-9]{64}', c['images'][name]):
        p.error('digest image required: ' + name)
items = []
def add(kind, name, ns, spec=None, **extra):
    obj = dict(apiVersion='v1', kind=kind, metadata=dict(name=name, namespace=ns), **extra)
    if spec is not None:
        obj['spec'] = spec
    items.append(obj)
for ns in ('workbench-system', 'workbench-egress'):
    add('Namespace', ns, ns)
add('ServiceAccount', 'runtime-controller', 'workbench-system')
add('ConfigMap', 'runtime-bindings', 'workbench-system', data={
    'projects.json': json.dumps({c['project_id']: c['project_namespace']}),
    'allowlists.json': json.dumps({c['project_id']: c.get('allowlist', [])})})
secret_env = {
 'controlplane': ['CONTROL_TOKEN', 'CONTROLLER_TOKEN', 'DATABASE_URL'],
 'runtime-controller': ['CONTROLLER_TOKEN', 'GATEWAY_SERVICE_TOKEN', 'GRANT_SECRET'],
 'dashboard': ['CONTROL_TOKEN'], 'mcp': ['CONTROL_TOKEN', 'MCP_TOKEN'],
 'inference-gateway': ['GATEWAY_SERVICE_TOKEN', 'SUBSCRIPTION_VAULT_TOKEN'],
 'credential-broker': ['GATEWAY_SERVICE_TOKEN'], 'egress-gateway': ['GATEWAY_SERVICE_TOKEN']}
base = {'CONTROLLER_URL': 'http://runtime-controller.workbench-system.svc:8080',
        'CONTROLPLANE_URL': 'http://controlplane:8080', 'CONTROLPLANE_READ_URL': 'http://controlplane:8081'}
for name in services:
    gateway = name in ('inference-gateway', 'credential-broker', 'egress-gateway')
    ns = 'workbench-egress' if gateway else 'workbench-system'
    labels = {'app': name}
    if gateway:
        labels.update({'workbench.project': c['project_id'], 'workbench.role': 'gateway'})
    env = dict(base)
    if gateway:
        env.update(GATEWAY_MODE='upstream', GATEWAY_CONFIG='/etc/workbench/' + name + '.json')
    if name == 'runtime-controller':
        env.update(RUNTIME_BACKEND='kubernetes', REQUIRE_VERIFIED_ISOLATION='true',
                   WORKSPACE_IMAGE=c['images']['workspace'], KUBERNETES_STORAGE_CLASS=c['storage_class'],
                   KUBERNETES_PROJECTS_FILE='/etc/runtime/projects.json', KUBERNETES_ALLOWLISTS_FILE='/etc/runtime/allowlists.json')
    if name == 'mcp':
        env['MCP_ALLOWED_HOSTS'] = c['host']
    env.update(c.get('environment', {}).get(name, {}))
    # An explicitly unverified test install must be selected in reviewed config.
    secret_dir = '/run/workbench-secrets' if name == 'runtime-controller' else '/run/secrets'
    container = dict(name=name, image=c['images'][name], ports=[dict(containerPort=8080)],
        env=[dict(name=k, value=str(v)) for k,v in env.items()] + [dict(name=k+'_FILE', value=secret_dir+'/'+k.lower()) for k in secret_env[name]],
        securityContext=dict(allowPrivilegeEscalation=False, readOnlyRootFilesystem=True, capabilities={'drop':['ALL']}),
        resources={'requests':{'cpu':'100m','memory':'128Mi'},'limits':{'cpu':'1','memory':'1Gi'}},
        readinessProbe={'exec':{'command':['/usr/local/bin/healthcheck']}, 'timeoutSeconds':3},
        volumeMounts=[dict(name='secrets', mountPath=secret_dir, readOnly=True),dict(name='tmp',mountPath='/tmp')])
    volumes = [dict(name='secrets',secret={'secretName':name+'-secrets'}),dict(name='tmp',emptyDir={'sizeLimit':'64Mi'})]
    if name == 'runtime-controller':
        volumes.append(dict(name='bindings', configMap={'name':'runtime-bindings'}))
        container['volumeMounts'].append(dict(name='bindings',mountPath='/etc/runtime',readOnly=True))
    if gateway:
        volumes.append(dict(name='config',secret={'secretName':name+'-config'}))
        container['volumeMounts'].append(dict(name='config',mountPath='/etc/workbench',readOnly=True))
    pod = dict(automountServiceAccountToken=name=='runtime-controller', containers=[container],volumes=volumes,
               securityContext=dict(runAsNonRoot=True,runAsUser=10001,runAsGroup=10001,seccompProfile={'type':'RuntimeDefault'}))
    if c.get('image_pull_secrets'):
        pod['imagePullSecrets'] = [dict(name=name) for name in c['image_pull_secrets']]
    if name=='runtime-controller':
        pod['serviceAccountName']='runtime-controller'
    add('Deployment',name,ns,dict(replicas=1,strategy={'type':'Recreate'},selector={'matchLabels':{'app':name}},template={'metadata':{'labels':labels},'spec':pod}))
    items[-1]['apiVersion']='apps/v1'
    ports=[dict(name='http',port=8080,targetPort=8080)]
    if name=='controlplane':
        ports.append(dict(name='read',port=8081,targetPort=8081))
    add('Service',name,ns,dict(selector={'app':name},ports=ports))
add('Ingress','workbench','workbench-system',dict(ingressClassName=c.get('ingress_class','traefik'),
 tls=[dict(hosts=[c['host']],secretName=c['tls_secret'])],rules=[dict(host=c['host'],http={'paths':[
 {'path':'/','pathType':'Prefix','backend':{'service':{'name':'dashboard','port':{'number':8080}}}}]})]))
items[-1]['apiVersion']='networking.k8s.io/v1'
# Remote access must pass an operator-configured authenticating ingress. No raw
# controlplane/MCP ingress is exposed by this package.
items[-1]['metadata']['annotations']=c['auth_annotations']
if not c['auth_annotations']:
    p.error('auth_annotations must configure an authenticating ingress')
a.output.write_text(json.dumps(dict(apiVersion='v1',kind='List',items=items),indent=2)+'\n')
