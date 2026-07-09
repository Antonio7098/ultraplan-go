#!/usr/bin/env bash
set -euo pipefail

repo="${1:-github.com/Antonio7098/ultraplan-go/cmd/ultraplan@main}"
gobin="${GOBIN:-$HOME/.local/bin}"

mkdir -p "$gobin"
GOBIN="$gobin" go install "$repo"

echo "installed ultraplan to $gobin/ultraplan"
