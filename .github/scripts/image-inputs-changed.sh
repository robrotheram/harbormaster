#!/usr/bin/env bash
set -euo pipefail

# A new ref, manual run, or unavailable comparison must build.
if [[ -z ${1:-} ]] || ! git cat-file -e "$1^{commit}" 2>/dev/null ||
    ! git cat-file -e "$2^{commit}" 2>/dev/null; then
  echo 'images=true'
  exit
fi

if git diff --quiet "$1" "$2" -- \
    .dockerignore Dockerfile go.mod go.sum cmd/ internal/ tools/ frontend/ integrations/hermes/ \
    deploy/hermes/runtime.json .github/workflows/check.yml .github/scripts/ \
    ':(exclude,glob)**/*.md'; then
  echo 'images=false'
else
  echo 'images=true'
fi
