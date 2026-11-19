#!/usr/bin/env bash
set -euo pipefail

namespace=$1
tag=$2
digest_dir=$3

for file in "$digest_dir"/*.json; do
  image=$(jq -er '.name | select(test("^[a-z-]+$"))' "$file")
  digest=$(jq -er '.digest | select(test("^sha256:[0-9a-f]{64}$"))' "$file")
  ref="$namespace/$image"
  docker buildx imagetools create --tag "$ref:$tag" "$ref@$digest"
done
