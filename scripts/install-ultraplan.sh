#!/usr/bin/env bash
set -euo pipefail

gobin="${GOBIN:-$HOME/.local/bin}"
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"

mkdir -p "$gobin"
if (( $# > 0 )); then
  GOBIN="$gobin" go install "$1"
else
  (
    cd "$repo_root"
    GOBIN="$gobin" go install ./cmd/ultraplan
  )
fi

echo "installed ultraplan to $gobin/ultraplan"
