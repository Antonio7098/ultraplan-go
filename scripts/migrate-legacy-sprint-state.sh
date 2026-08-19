#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
  echo "usage: $0 <workspace> <project> [--apply]" >&2
  exit 2
fi

workspace=$(cd "$1" && pwd)
project=$2
mode=${3:---dry-run}
if [[ "$mode" != "--dry-run" && "$mode" != "--apply" ]]; then
  echo "mode must be --dry-run or --apply" >&2
  exit 2
fi

project_root="$workspace/projects/$project"
if [[ ! -d "$project_root/sprints" ]]; then
  echo "missing project sprints directory: $project_root/sprints" >&2
  exit 4
fi

stage_status() {
  local sprint_root=$1
  local stage=$2
  case "$stage" in
    area-reasoning)
      if find "$sprint_root/reasoning" -maxdepth 1 -type f -name '*.md' -print -quit 2>/dev/null | grep -q .; then
        echo complete
      else
        echo skipped
      fi
      ;;
    *)
      local filename=$stage.md
      [[ "$stage" == "sprint-index" ]] && filename=sprint-index.md
      [[ "$stage" == "technical-handbook" ]] && filename=technical-handbook.md
      if [[ -f "$sprint_root/$filename" ]]; then echo complete; else echo missing; fi
      ;;
  esac
}

migrated=0
while IFS= read -r state; do
  if [[ $(jq -r '(.schemaVersion == null) and (.version == 1) and (.stages | type == "object")' "$state") != true ]]; then
    continue
  fi
  sprint_root=$(dirname "$state")
  slug=$(basename "$sprint_root")
  backup="$sprint_root/flow-state.legacy-v1.json"
  if [[ -e "$backup" ]]; then
    echo "refusing to overwrite backup: $backup" >&2
    exit 1
  fi
  echo "$mode $project/$slug"
  migrated=$((migrated + 1))
  [[ "$mode" == "--dry-run" ]] && continue

  updated_at=$(jq -r '.updatedAt // empty' "$state")
  [[ -z "$updated_at" ]] && updated_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  stages=()
  for stage in requirements sprint-index technical-handbook area-reasoning reasoning plan; do
    status=$(stage_status "$sprint_root" "$stage")
    path="projects/$project/sprints/$slug/$stage.md"
    [[ "$stage" == "area-reasoning" ]] && path="projects/$project/sprints/$slug/reasoning"
    stages+=("$(jq -cn --arg stage "$stage" --arg status "$status" --arg path "$path" '{stage:$stage,status:$status,path:$path}')")
  done
  stages_json=$(printf '%s\n' "${stages[@]}" | jq -s '.')
  temp=$(mktemp "$sprint_root/.flow-state.migration.XXXXXX")
  jq -n --arg project "$project" --arg sprint "$slug" --arg updatedAt "$updated_at" --argjson stages "$stages_json" \
    '{schemaVersion:2,project:$project,sprint:$sprint,updatedAt:$updatedAt,stages:$stages}' > "$temp"
  cp -p "$state" "$backup"
  mv "$temp" "$state"
done < <(find "$project_root/sprints" -mindepth 2 -maxdepth 2 -name flow-state.json -type f | sort)

echo "legacy flow states matched: $migrated"
