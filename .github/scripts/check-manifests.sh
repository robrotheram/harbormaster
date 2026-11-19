#!/usr/bin/env bash
set -euo pipefail

rendered=$(mktemp -d)
trap 'rm -rf "$rendered"' EXIT

.local/tools/kustomize build deploy/kubernetes/base > "$rendered/base.yaml"
.local/tools/kustomize build deploy/kubernetes/overlays/example > "$rendered/example.yaml"
# The Kubernetes schema catalog excludes declarations of custom resource types.
.local/tools/kubeconform -strict -summary -kubernetes-version 1.35.0 \
  -schema-location 'https://raw.githubusercontent.com/yannh/kubernetes-json-schema/88b0235585da4efb0e4bc6cc00baccf0e327e54d/{{.NormalizedKubernetesVersion}}-standalone{{.StrictSuffix}}/{{.ResourceKind}}{{.KindSuffix}}.json' \
  -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/784ec895547a36e548e6810b41f27baade3a0890/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' \
  -skip CustomResourceDefinition "$rendered"/*.yaml
