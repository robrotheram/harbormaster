#!/usr/bin/env bash
set -euo pipefail

repository=$1
revision=$2
runs="repos/$repository/actions/workflows/check.yml/runs"
if ! run_id=$(gh api "$runs" -X GET -f branch=main -f event=push -f status=success \
    -f head_sha="$revision" -F per_page=1 --jq '.workflow_runs[0].id // empty'); then
  echo 'Could not look up the main workflow run.' >&2
  exit 0
fi
if [[ -z "$run_id" ]]; then
  exit 0
fi
[[ "$run_id" =~ ^[0-9]+$ ]]

artifacts="repos/$repository/actions/runs/$run_id/artifacts"
if ! complete=$(gh api "$artifacts?per_page=100" --jq '
    [.artifacts[] | select(.expired == false and (.name | startswith("digest-"))) | .name]
    | sort == [
        "digest-controlplane", "digest-credential-broker", "digest-dashboard",
        "digest-egress-gateway", "digest-inference-gateway", "digest-mcp",
        "digest-runtime-controller", "digest-workspace", "digest-workspace-hermes"
      ]
  '); then
  echo 'Could not look up the main workflow artifacts.' >&2
  exit 0
fi
if [[ "$complete" == true ]]; then
  echo "$run_id"
fi
