#!/usr/bin/env bash
set -euo pipefail
# Run on a trusted backup host with pg_dump, kubectl and the CSI snapshot CRDs.
# DATABASE_URL must address the same database as the sole controlplane owner.
: "${DATABASE_URL:?}" "${PROJECT_NAMESPACE:?}" "${SNAPSHOT_CLASS:?}" "${BACKUP_DIR:?}"
umask 077
mkdir -p "$BACKUP_DIR"
kubectl -n workbench-system scale deployment/controlplane --replicas=0
trap 'echo "Controlplane remains stopped; verify all workspace suspensions before resuming" >&2' EXIT
# Operator must suspend every Sandbox first; abort if any writer is active.
kubectl -n "$PROJECT_NAMESPACE" get sandboxes -o json > "$BACKUP_DIR/sandboxes.json"
python3 - "$BACKUP_DIR/sandboxes.json" <<'PY'
import json,sys
for s in json.load(open(sys.argv[1]))['items']:
 assert s['spec']['operatingMode']=='Suspended'
 assert any(c['type']=='Suspended' and c['status']=='True' and c.get('observedGeneration')==s['metadata']['generation'] for c in s['status']['conditions'])
PY
pg_dump --format=custom --file="$BACKUP_DIR/metadata.dump" "$DATABASE_URL"
kubectl -n "$PROJECT_NAMESPACE" get pvc,secrets,networkpolicies -o json > "$BACKUP_DIR/project.json"
kubectl -n "$PROJECT_NAMESPACE" get pvc -o name | while read -r pvc; do
 name="backup-${pvc#*/}-$(date +%s)"
 cat <<JSON | kubectl create -f -
{"apiVersion":"snapshot.storage.k8s.io/v1","kind":"VolumeSnapshot","metadata":{"name":"$name","namespace":"$PROJECT_NAMESPACE"},"spec":{"volumeSnapshotClassName":"$SNAPSHOT_CLASS","source":{"persistentVolumeClaimName":"${pvc#*/}"}}}
JSON
 kubectl -n "$PROJECT_NAMESPACE" wait "volumesnapshot/$name" --for=jsonpath='{.status.readyToUse}'=true --timeout=10m
 kubectl -n "$PROJECT_NAMESPACE" get "volumesnapshot/$name" -o json > "$BACKUP_DIR/$name.json"
done
(cd "$BACKUP_DIR" && sha256sum ./*.json ./metadata.dump > SHA256SUMS)
echo 'Backup complete. Export CSI snapshots off-cluster and encrypt this directory before resuming controlplane.'
