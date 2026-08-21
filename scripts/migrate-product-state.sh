#!/usr/bin/env bash
set -euo pipefail

workspace_path=${1:-.}
if [[ $# -gt 0 ]]; then
  shift
fi

ultraplan_binary=${ULTRAPLAN_BIN:-ultraplan}
exec "$ultraplan_binary" --workspace "$workspace_path" storage migrate "$@"
