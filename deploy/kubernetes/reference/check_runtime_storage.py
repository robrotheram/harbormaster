#!/usr/bin/env python3
"""Disposable Kata/RWOP acceptance for the reference cluster.
Creates and removes only uniquely named probe Pods/PVCs. Requires the approved
Kata installation and the Longhorn scratch class with Delete reclaim policy.
"""
import json,subprocess,time,uuid
from pathlib import Path
name='reference-probe-'+uuid.uuid4().hex[:8]
base=['kubectl','--context','default','--request-timeout=15s','-n','workbench-system']
def run(args, obj=None):
 p=subprocess.run(base+args,input=json.dumps(obj) if obj else None,text=True,capture_output=True,check=True)
 return p.stdout

def get(kind,key): return json.loads(run(['get',kind,key,'-o','json']))
def wait(predicate,seconds=120):
 deadline=time.monotonic()+seconds
 while time.monotonic()<deadline:
  value=predicate()
  if value:return value
  time.sleep(2)
 raise RuntimeError('Acceptance probe timed out')

def pod(suffix,command):
 return {'apiVersion':'v1','kind':'Pod','metadata':{'name':name+suffix},'spec':{
 'runtimeClassName':'kata','restartPolicy':'Never','automountServiceAccountToken':False,
 'securityContext':{'runAsUser':10001,'runAsGroup':10001,'fsGroup':10001,'runAsNonRoot':True,'seccompProfile':{'type':'RuntimeDefault'}},
 'containers':[{'name':'probe','image':'alpine:3.23@sha256:469c9ea410b9215eaee7976ba6c0639039b444be8e5d638f7abbada189971eb2',
 'command':['sh','-ec',command], 'resources':{'requests':{'cpu':'100m','memory':'128Mi'},'limits':{'cpu':'1','memory':'512Mi'}},
 'securityContext':{'readOnlyRootFilesystem':True,'allowPrivilegeEscalation':False,'capabilities':{'drop':['ALL']}},
 'volumeMounts':[{'name':'data','mountPath':'/data'}]}],
 'volumes':[{'name':'data','persistentVolumeClaim':{'claimName':name}}]}}
Path('.local/k3s-reference').mkdir(parents=True, exist_ok=True)
try:
 run(['create','-f','-'],{'apiVersion':'v1','kind':'PersistentVolumeClaim','metadata':{'name':name},'spec':{'storageClassName':'longhorn','accessModes':['ReadWriteOncePod'],'resources':{'requests':{'storage':'128Mi'}}}})
 run(['create','-f','-'],pod('-writer','uname -r; printf reference-state > /data/marker; sleep 300'))
 wait(lambda:get('pod',name+'-writer').get('status',{}).get('phase')=='Running')
 kernel=run(['logs',name+'-writer']).strip()
 print('Kata guest kernel:',kernel,flush=True)
 host = json.loads(run(['get', 'node', get('pod', name+'-writer')['spec']['nodeName'], '-o', 'json']))
 assert kernel != host['status']['nodeInfo']['kernelVersion'], 'guest kernel matches host'
 assert kernel, 'guest did not report a kernel'
 run(['create','-f','-'],pod('-reader','test "$(cat /data/marker)" = reference-state; uname -r'))
 def fenced():
  status=get('pod',name+'-reader').get('status',{})
  if status.get('phase') in ['Running','Succeeded']:raise RuntimeError('Second writer was not fenced')
  for c in status.get('conditions',[]):
   if c.get('type')=='PodScheduled' and c.get('status')=='False' and 'ReadWriteOncePod' in c.get('message',''):return c['message']
 message=wait(fenced,60)
 print('RWOP second-pod fence:',message,flush=True)
 run(['delete','pod',name+'-writer','--wait=true','--timeout=60s'])
 wait(lambda:get('pod',name+'-reader').get('status',{}).get('phase')=='Succeeded')
 evidence={'guest_kernel':kernel,'rwop_fence':message,'retained_marker_after_reattach':True}
 Path('.local/k3s-reference/runtime-storage-evidence.json').write_text(json.dumps(evidence,indent=2)+'\n')
 print('Kata/RWOP/reattach acceptance passed',flush=True)
finally:
 for kind,keys in [('pod',[name+'-writer',name+'-reader']),('pvc',[name])]:
  subprocess.run(base+['delete',kind,*keys,'--ignore-not-found','--wait=false'],capture_output=True)
