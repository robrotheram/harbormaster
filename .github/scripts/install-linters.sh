#!/usr/bin/env bash
set -euo pipefail

bin_dir=$1
mkdir -p "$bin_dir"
downloads=$(mktemp -d)
trap 'rm -rf "$downloads"' EXIT

fetch() {
  local url=$1 digest=$2 file=$3
  curl --fail --silent --show-error --location --retry 3 "$url" --output "$file"
  echo "$digest  $file" | sha256sum --check
}

fetch https://github.com/golangci/golangci-lint/releases/download/v2.15.0/golangci-lint-2.15.0-linux-amd64.tar.gz \
  9b7fe6a6bf6334b98decf8d86b11415ea7889afdabdd1959fdce23af2a9ebcea "$downloads/golangci-lint.tgz"
tar -xzf "$downloads/golangci-lint.tgz" -C "$bin_dir" --strip-components=1 golangci-lint-2.15.0-linux-amd64/golangci-lint

fetch https://github.com/rhysd/actionlint/releases/download/v1.7.12/actionlint_1.7.12_linux_amd64.tar.gz \
  8aca8db96f1b94770f1b0d72b6dddcb1ebb8123cb3712530b08cc387b349a3d8 "$downloads/actionlint.tgz"
tar -xzf "$downloads/actionlint.tgz" -C "$bin_dir" actionlint

fetch https://github.com/hadolint/hadolint/releases/download/v2.15.1/hadolint-linux-x86_64 \
  c7187db94eeeeca956519a6af171adc31453941a1e777961f6e680f697c8c507 "$downloads/hadolint"
install -m 0755 "$downloads/hadolint" "$bin_dir/hadolint"

fetch https://github.com/yannh/kubeconform/releases/download/v0.8.0/kubeconform-linux-amd64.tar.gz \
  9bc2bffbf71f261128533edaf912153948b7ff238f9a531ae6d34466ec287883 "$downloads/kubeconform.tgz"
tar -xzf "$downloads/kubeconform.tgz" -C "$bin_dir" kubeconform
