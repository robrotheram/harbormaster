#!/usr/bin/env bash
set -euo pipefail
# Target an isolated recovery cluster/database. Keep public ingress disabled.
: "${BACKUP_DIR:?}" "${RECOVERY_DATABASE_URL:?}" "${RESTORED_PVC_MANIFEST:?}"
(cd "$BACKUP_DIR" && sha256sum -c SHA256SUMS)
# Operator-reviewed PVC manifests must use snapshot dataSource and original
# namespace/name/ownership, the tested CSI class, and ReadWriteOncePod.
python3 - "$RESTORED_PVC_MANIFEST" <<'PY'
import json,sys
items=json.load(open(sys.argv[1]))['items']
assert items
for p in items:
 assert p['kind']=='PersistentVolumeClaim'
 assert p['spec']['accessModes']==['ReadWriteOncePod']
 assert p['spec']['dataSource']['kind']=='VolumeSnapshot'
 assert p['metadata']['labels']['workbench.home']
 assert p['metadata']['labels']['workbench.project']
PY
kubectl -n workbench-system scale deployment/controlplane --replicas=0
pg_restore --exit-on-error --no-owner --dbname="$RECOVERY_DATABASE_URL" "$BACKUP_DIR/metadata.dump"
kubectl apply -f "$RESTORED_PVC_MANIFEST"
echo 'Restore loaded. Restore reviewed grant secrets and suspended Sandbox definitions; verify home hashes and single-writer reattachment before enabling ingress/controlplane.'
