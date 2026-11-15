#!/usr/bin/env bash
set -euo pipefail
# Read-only infrastructure inventory. Connectivity probes run inside an existing
# disposable, operator-provisioned Kata workspace; never accept readiness as VM proof.
: "${PROJECT_NAMESPACE:?}" "${WORKSPACE_POD:?}" "${OTHER_PROJECT_ENDPOINT:?}" "${APPROVED_ENDPOINT:?}"
mkdir -p "${EVIDENCE_DIR:=./k3s-evidence}"
kubectl version -o json > "$EVIDENCE_DIR/version.json"
kubectl get runtimeclass kata -o yaml > "$EVIDENCE_DIR/runtimeclass.yaml"
kubectl -n "$PROJECT_NAMESPACE" get networkpolicy,resourcequota,pvc -o yaml > "$EVIDENCE_DIR/project.yaml"
kubectl -n "$PROJECT_NAMESPACE" get pod "$WORKSPACE_POD" -o json > "$EVIDENCE_DIR/pod.json"
jq -e '.spec | .runtimeClassName == "kata" and
  .automountServiceAccountToken == false and
  (.hostNetwork // false) == false and (.hostPID // false) == false and
  (.hostIPC // false) == false' "$EVIDENCE_DIR/pod.json" >/dev/null
# Operator credential may exec solely for these acceptance probes. Runtime RBAC cannot.
probe() { kubectl -n "$PROJECT_NAMESPACE" exec "$WORKSPACE_POD" -- curl --max-time 5 --fail --silent "$1" >/dev/null; }
if probe https://kubernetes.default.svc; then echo 'API reachable' >&2; exit 1; fi
if probe "$OTHER_PROJECT_ENDPOINT"; then echo 'cross-project traffic reachable' >&2; exit 1; fi
if probe https://example.com; then echo 'direct Internet reachable' >&2; exit 1; fi
probe "$APPROVED_ENDPOINT"
kubectl auth can-i create pods --as=system:serviceaccount:workbench-system:runtime-controller -n "$PROJECT_NAMESPACE" | { read -r result; test "$result" = no; }
echo 'Configuration and connectivity probes passed; NOT Kata attestation or full M5 acceptance.'
