#!/usr/bin/env bash
#
# Drive opencode non-interactively on the current checkout, commit whatever
# it produces, push to main, then reinstall the ultraplan binary.
#
# Deterministic: the commit subject and body are built from the actual
# diff and the opencode model list. No LLM is asked to write the message.
#
# Configuration (override via environment):
#   PICKLE_PROMPT     prompt passed to `opencode run`     (default: "big pickle")
#   PICKLE_MODEL      opencode --model value              (default: empty = provider default)
#   PICKLE_AGENT      opencode --agent value              (default: empty = default agent)
#   PICKLE_DIR        directory opencode runs in          (default: repo root)
#   PICKLE_RG         ripgrep term against `opencode models` (default: "pickle")
#   PICKLE_BRANCH     branch to push to                   (default: main)
#   PICKLE_INSTALL    install script path                 (default: scripts/install-ultraplan.sh)

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"

PICKLE_PROMPT="${PICKLE_PROMPT:-big pickle}"
PICKLE_RG="${PICKLE_RG:-pickle}"
PICKLE_BRANCH="${PICKLE_BRANCH:-main}"
PICKLE_DIR="${PICKLE_DIR:-$repo_root}"
PICKLE_INSTALL="${PICKLE_INSTALL:-$script_dir/install-ultraplan.sh}"

log() { printf '[pickle] %s\n' "$*" >&2; }
die() { printf '[pickle] error: %s\n' "$*" >&2; exit 1; }

[[ -d "$PICKLE_DIR" ]] || die "PICKLE_DIR does not exist: $PICKLE_DIR"
[[ -x "$PICKLE_INSTALL" ]] || die "install script missing or not executable: $PICKLE_INSTALL"

run_opencode() {
  local -a cmd=(opencode run --dir "$PICKLE_DIR")
  [[ -n "${PICKLE_MODEL:-}" ]] && cmd+=(--model "$PICKLE_MODEL")
  [[ -n "${PICKLE_AGENT:-}" ]] && cmd+=(--agent "$PICKLE_AGENT")
  cmd+=("$PICKLE_PROMPT")
  log "running: ${cmd[*]}"
  "${cmd[@]}"
}

collect_models() {
  if ! command -v opencode >/dev/null 2>&1; then
    printf '(opencode not on PATH)\n'
    return 0
  fi
  if ! command -v rg >/dev/null 2>&1; then
    log "ripgrep (rg) not found; skipping model filter"
    opencode models || true
    return 0
  fi
  local matches
  if matches="$(opencode models | rg --ignore-case --no-heading "$PICKLE_RG" || true)"; then
    if [[ -n "$matches" ]]; then
      printf '%s\n' "$matches"
    else
      printf '(no models matched /%s/i)\n' "$PICKLE_RG"
    fi
  fi
}

build_commit_message() {
  local stamp title body
  stamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  title="pickle: ${PICKLE_PROMPT} (${stamp})"

  {
    printf '%s\n\n' "$title"
    printf 'prompt: %s\n' "$PICKLE_PROMPT"
    printf 'opencode models matching /%s/i:\n' "$PICKLE_RG"
    printf -- '----\n'
    collect_models
    printf -- '----\n\n'
    printf 'git diff --stat:\n'
    printf -- '----\n'
    if git -C "$PICKLE_DIR" diff --stat --no-color; then
      :
    else
      printf '(stat unavailable)\n'
    fi
    printf -- '----\n'
  } >"$msg_file"

  printf '%s' "$title"
}

current_branch() {
  git -C "$PICKLE_DIR" symbolic-ref --quiet --short HEAD 2>/dev/null \
    || die "not on a branch (detached HEAD)"
}

main() {
  cd "$PICKLE_DIR"
  local branch
  branch="$(current_branch)"
  [[ "$branch" == "$PICKLE_BRANCH" ]] || die "on branch '$branch', expected '$PICKLE_BRANCH'"

  run_opencode

  if [[ -z "$(git -C "$PICKLE_DIR" status --porcelain)" ]]; then
    log "no changes produced; nothing to commit"
    exit 0
  fi

  local msg_file
  msg_file="$(mktemp -t pickle-commit-XXXXXX.txt)"
  trap 'rm -f "$msg_file"' EXIT

  local subject
  subject="$(build_commit_message)"

  git -C "$PICKLE_DIR" add -A
  log "staged $(git -C "$PICKLE_DIR" diff --cached --name-only | wc -l | tr -d ' ') file(s)"

  git -C "$PICKLE_DIR" commit -F "$msg_file"
  log "created commit: $(git -C "$PICKLE_DIR" rev-parse --short HEAD)"

  log "pushing to origin/$PICKLE_BRANCH"
  git -C "$PICKLE_DIR" push origin "$PICKLE_BRANCH"

  log "running installer: $PICKLE_INSTALL"
  "$PICKLE_INSTALL"
  log "pickle pipeline complete"
}

main "$@"
