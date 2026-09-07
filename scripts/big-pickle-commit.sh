#!/usr/bin/env bash
# Inspect, stage, commit, and push every change in the current repository.
# Big Pickle writes the commit message and resolves rebase conflicts. The
# script never asks the user for input.

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"

PICKLE_DIR="${PICKLE_DIR:-$repo_root}"
PICKLE_MODEL="${PICKLE_MODEL:-opencode/big-pickle}"
PICKLE_REMOTE="${PICKLE_REMOTE:-origin}"
PICKLE_PUSH_ATTEMPTS="${PICKLE_PUSH_ATTEMPTS:-3}"

log() { printf '[pickle] %s\n' "$*" >&2; }
die() { printf '[pickle] error: %s\n' "$*" >&2; exit 1; }

[[ -d "$PICKLE_DIR" ]] || die "PICKLE_DIR does not exist: $PICKLE_DIR"
command -v git >/dev/null 2>&1 || die "git is not on PATH"
command -v opencode >/dev/null 2>&1 || die "opencode is not on PATH"
[[ "$PICKLE_PUSH_ATTEMPTS" =~ ^[1-9][0-9]*$ ]] \
  || die "PICKLE_PUSH_ATTEMPTS must be a positive integer"

current_branch() {
  git -C "$PICKLE_DIR" symbolic-ref --quiet --short HEAD 2>/dev/null \
    || die "not on a branch (detached HEAD)"
}

show_changes() {
  log "working-tree status"
  git -C "$PICKLE_DIR" status --short
  log "unstaged diff"
  git -C "$PICKLE_DIR" --no-pager diff --no-ext-diff --no-color
  log "staged diff"
  git -C "$PICKLE_DIR" --no-pager diff --cached --no-ext-diff --no-color
}

write_commit_message() {
  local diff_file="$1"
  local output_file="$2"
  local raw_file="$3"
  local prompt
  prompt="$(printf '%s\n' \
    'Write the Git commit message for the attached staged diff.' \
    'Output only the commit message. Do not use Markdown fences or commentary.' \
    'Use an imperative subject of at most 72 characters.' \
    'Add a concise body only when it explains important context not obvious from the subject.' \
    'Do not edit files, run Git commands, commit, or push.')"

  log "asking $PICKLE_MODEL for the commit message"
  opencode run \
    "$prompt" \
    --dir "$PICKLE_DIR" \
    --model "$PICKLE_MODEL" \
    --file "$diff_file" >"$raw_file"

  sed -e 's/\r$//' -e '/^```[[:alnum:]_-]*$/d' "$raw_file" \
    | awk '
        BEGIN { pending = ""; seen = 0 }
        /^[[:space:]]*$/ {
          if (seen) pending = pending "\n"
          next
        }
        {
          if (seen && pending != "") printf "%s", pending
          print
          pending = ""
          seen = 1
        }
      ' >"$output_file"

  [[ -s "$output_file" ]] || die "$PICKLE_MODEL returned an empty commit message"
  [[ "$(sed -n '1p' "$output_file" | wc -c | tr -d ' ')" -le 73 ]] \
    || die "$PICKLE_MODEL returned a commit subject longer than 72 characters"
}

resolve_conflicts() {
  local conflicted
  local -a conflicted_paths
  conflicted="$(git -C "$PICKLE_DIR" diff --name-only --diff-filter=U)"
  [[ -n "$conflicted" ]] || return 1
  mapfile -t conflicted_paths <<<"$conflicted"

  log "rebase conflicts detected; asking $PICKLE_MODEL to resolve them"
  opencode run \
    --dir "$PICKLE_DIR" \
    --model "$PICKLE_MODEL" \
    --auto \
    "Resolve every current Git rebase conflict in this repository. Inspect both sides and preserve their intent. Edit only files needed to resolve the conflicts. Remove all conflict markers. Do not stage, commit, continue the rebase, abort the rebase, or push. Do not ask questions."

  if git -C "$PICKLE_DIR" grep -n -E '^(<<<<<<<|=======|>>>>>>>)' -- "${conflicted_paths[@]}"; then
    die "conflict markers remain after model resolution"
  fi
  git -C "$PICKLE_DIR" add -A
  conflicted="$(git -C "$PICKLE_DIR" diff --name-only --diff-filter=U)"
  [[ -z "$conflicted" ]] || die "unresolved conflicts remain: $conflicted"
}

rebase_onto_remote() {
  local branch="$1"

  git -C "$PICKLE_DIR" fetch "$PICKLE_REMOTE" "$branch"
  if git -C "$PICKLE_DIR" merge-base --is-ancestor "$PICKLE_REMOTE/$branch" HEAD; then
    return 0
  fi

  log "rebasing onto $PICKLE_REMOTE/$branch"
  if ! GIT_EDITOR=true git -C "$PICKLE_DIR" rebase "$PICKLE_REMOTE/$branch"; then
    resolve_conflicts || die "rebase failed without resolvable file conflicts"
    while ! GIT_EDITOR=true git -C "$PICKLE_DIR" rebase --continue; do
      if resolve_conflicts; then
        continue
      fi
      if git -C "$PICKLE_DIR" diff --cached --quiet; then
        log "skipping an empty commit after conflict resolution"
        git -C "$PICKLE_DIR" rebase --skip
        if [[ ! -d "$(git -C "$PICKLE_DIR" rev-parse --git-path rebase-merge)" \
          && ! -d "$(git -C "$PICKLE_DIR" rev-parse --git-path rebase-apply)" ]]; then
          break
        fi
        continue
      fi
      die "rebase could not continue"
    done
  fi
}

main() {
  local branch attempt
  local temp_dir diff_file message_file raw_message_file

  git -C "$PICKLE_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1 \
    || die "PICKLE_DIR is not a Git working tree"
  branch="$(current_branch)"

  temp_dir="$(mktemp -d -t big-pickle-commit-XXXXXX)"
  trap 'rm -rf -- "${temp_dir:-}"' EXIT
  diff_file="$temp_dir/staged.diff"
  message_file="$temp_dir/commit-message.txt"
  raw_message_file="$temp_dir/model-output.txt"

  show_changes
  git -C "$PICKLE_DIR" add -A

  if git -C "$PICKLE_DIR" diff --cached --quiet; then
    log "no changes to commit"
  else
    git -C "$PICKLE_DIR" --no-pager diff --cached --no-ext-diff --binary --no-color \
      >"$diff_file"
    log "staged $(git -C "$PICKLE_DIR" diff --cached --name-only | wc -l | tr -d ' ') file(s)"
    write_commit_message "$diff_file" "$message_file" "$raw_message_file"
    git -C "$PICKLE_DIR" commit -F "$message_file"
    log "created commit $(git -C "$PICKLE_DIR" rev-parse --short HEAD): $(sed -n '1p' "$message_file")"
  fi

  for ((attempt = 1; attempt <= PICKLE_PUSH_ATTEMPTS; attempt++)); do
    rebase_onto_remote "$branch"
    log "pushing HEAD to $PICKLE_REMOTE/$branch (attempt $attempt/$PICKLE_PUSH_ATTEMPTS)"
    if git -C "$PICKLE_DIR" push "$PICKLE_REMOTE" "HEAD:$branch"; then
      log "all repository changes are committed and pushed"
      return 0
    fi
  done

  die "push failed after $PICKLE_PUSH_ATTEMPTS attempts"
}

main "$@"
