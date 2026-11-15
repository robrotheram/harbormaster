#!/usr/bin/env bash
set -euo pipefail
# Requires jq, pg_restore and kubectl. Target an isolated recovery cluster/database.
# Keep public ingress disabled.
: "${BACKUP_DIR:?}" "${RECOVERY_DATABASE_URL:?}" "${RESTORED_PVC_MANIFEST:?}"
(cd "$BACKUP_DIR" && sha256sum -c SHA256SUMS)
# Operator-reviewed PVC manifests must use snapshot dataSource and original
# namespace/name/ownership, the tested CSI class, and ReadWriteOncePod.
jq -e '.items | length > 0 and all(.[];
  .kind == "PersistentVolumeClaim" and .spec.accessModes == ["ReadWriteOncePod"] and
  .spec.dataSource.kind == "VolumeSnapshot" and
  (.metadata.labels["workbench.home"] | type == "string" and length > 0) and
  (.metadata.labels["workbench.project"] | type == "string" and length > 0))' "$RESTORED_PVC_MANIFEST" >/dev/null
kubectl -n workbench-system scale deployment/controlplane --replicas=0
pg_restore --exit-on-error --no-owner --dbname="$RECOVERY_DATABASE_URL" "$BACKUP_DIR/metadata.dump"
kubectl apply -f "$RESTORED_PVC_MANIFEST"
echo 'Restore loaded. Restore reviewed grant secrets and suspended Sandbox definitions; verify home hashes and single-writer reattachment before enabling ingress/controlplane.'
