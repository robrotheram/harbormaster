#!/usr/bin/env python3
"""Disposable live network-policy acceptance for the reference cluster.
Proves endpoints are reachable from the gateway namespace before asserting that
the Kata project Pod cannot reach them. Cleans up only uniquely named probes.
"""
import json,subprocess,time,uuid
from pathlib import Path
prefix='reference-network-'+uuid.uuid4().hex[:8]
image='python:3.12-alpine@sha256:c5ba3deef00c7758561a0f03c26f44f10589263d40e732b8748e0d1f38f6dd89'
base=['kubectl','--context','default','--request-timeout=15s']
created=[]
def run(args,obj=None):
 return subprocess.run(base+args,input=json.dumps(obj) if obj else None,text=True,capture_output=True,check=True).stdout

def get(ns,name):return json.loads(run(['-n',ns,'get','pod',name,'-o','json']))
def create(ns,name,command,labels,kata=False):
 spec={'automountServiceAccountToken':False,'restartPolicy':'Never','securityContext':{'runAsNonRoot':True,'runAsUser':10001,'runAsGroup':10001,'seccompProfile':{'type':'RuntimeDefault'}},'containers':[{'name':'probe','image':image,'command':command,'securityContext':{'allowPrivilegeEscalation':False,'readOnlyRootFilesystem':True,'capabilities':{'drop':['ALL']}},'resources':{'requests':{'cpu':'100m','memory':'128Mi'},'limits':{'cpu':'1','memory':'512Mi'}}}]}
 if kata:spec['runtimeClassName']='kata'
 run(['create','-f','-'],{'apiVersion':'v1','kind':'Pod','metadata':{'namespace':ns,'name':name,'labels':labels},'spec':spec})
 created.append((ns,name))
def wait(ns,name,phase):
 end=time.monotonic()+120
 while time.monotonic()<end:
  p=get(ns,name)
  if p.get('status',{}).get('phase')==phase:return p
  if p.get('status',{}).get('phase')=='Failed':raise RuntimeError(run(['-n',ns,'logs',name]))
  time.sleep(2)
 raise RuntimeError('network acceptance timed out')
Path('.local/k3s-reference').mkdir(parents=True, exist_ok=True)
try:
 create('workbench-egress',prefix+'-allowed',['python','-m','http.server','8080'],{'workbench.project':'default','workbench.role':'gateway'})
 create('workbench-system',prefix+'-other',['python','-m','http.server','8080'],{})
 gateway=wait('workbench-egress',prefix+'-allowed','Running')['status']['podIP']
 other=wait('workbench-system',prefix+'-other','Running')['status']['podIP']
 api=json.loads(run(['-n','default','get','service','kubernetes','-o','json']))['spec']['clusterIP']
 targets={'gateway':[gateway,8080],'other_namespace':[other,8080],'api':[api,443],'internet':['1.1.1.1',443]}
 code='''import socket,json,os
checks={}
for name,(host,port) in TARGETS.items():
 try:
  s=socket.create_connection((host,port),3);s.close();checks[name]=True
 except OSError:checks[name]=False
print(json.dumps(checks),flush=True)
assert checks['gateway'] and all(not value for name,value in checks.items() if name!='gateway')
assert not os.path.exists('/var/run/secrets/kubernetes.io/serviceaccount/token')
'''.replace('TARGETS',repr(targets))
 # Verify the denied destinations are reachable from the trusted namespace first.
 baseline='import socket,json; targets='+repr(targets)+'; results={};\nfor n,(h,p) in targets.items():\n s=socket.create_connection((h,p),5);s.close();results[n]=True\nprint(json.dumps(results))'
 run(['-n','workbench-egress','exec',prefix+'-allowed','--','python','-c',baseline])
 create('workbench-reference',prefix+'-workspace',['python','-c',code],{'workbench.project':'default'},True)
 wait('workbench-reference',prefix+'-workspace','Succeeded')
 evidence=json.loads(run(['-n','workbench-reference','logs',prefix+'-workspace']))
 Path('.local/k3s-reference/network-evidence.json').write_text(json.dumps(evidence,indent=2)+'\n')
 print('Network policy verified:',json.dumps(evidence),flush=True)
finally:
 for ns,name in created:subprocess.run(base+['-n',ns,'delete','pod',name,'--ignore-not-found','--wait=false'],capture_output=True)
