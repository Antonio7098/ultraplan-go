# QA storage and evidence recovery

QA stores disposable workspaces, authoring snapshots, reproductions and reruns in
the operating system's user cache directory under `ultraplan/qa-runtime`. On a
normal Linux desktop this is `$HOME/.cache/ultraplan/qa-runtime`, outside `/tmp`.
Set `ULTRAPLAN_QA_RUNTIME_DIR` to an absolute directory on a disk with sufficient
space to choose another location. Keep it outside the implementation and the
governed workspace. All UltraPlan processes sharing a host should use the same
runtime directory so their reservations coordinate.

Before copying or executing, QA reserves additional disk capacity. Model workers
and reproductions also reserve memory. Reservations use a process-shared lock;
dead processes do not retain reservations. Admission waits up to 30 seconds for
capacity, then records a retryable capacity failure. It does not spend an
authoring turn. The defaults leave 256 MiB of disk and 1 GiB of host memory free,
reserve 1 GiB for reproduction build output and 2 GiB for dependency preparation,
plus the source copy. These are conservative estimates, not filesystem quotas.
Capacity diagnostics name the constrained resource and report available,
reserved, requested and headroom bytes in MiB, including the remaining shortfall.

The default memory reservation per worker is 1536 MiB.
`ULTRAPLAN_QA_WORKER_MEMORY_MB` permits a positive integer override based on
measured worker memory. Lowering it increases concurrency and host pressure.
Disposable scratch directories carry owner records; subsequent scratch creation
removes abandoned directories belonging to dead processes. Retained investigator
workspaces follow the existing attempt cleanup and restoration rules.

Go dependencies are downloaded and verified once in a contained preparation copy.
QA identifies the published cache by implementation, module directory and local
Go toolchain. Packages in the same module share it. Reproduction processes see
the cache read-only through native isolation and run with `GOPROXY=off` and
`GOTOOLCHAIN=local`. Build caches, home directories and temporary output remain
private. The required toolchain must already be installed. Published dependency
caches remain available across runs; remove unused caches only while QA is idle.

## Evidence accounting

New requests use version 2 accounting. Authoring, preparation and execution have
separate durable counters. A model turn is reserved immediately before calling
the model, after workspace and snapshot preparation. The resulting immutable
bundle is checkpointed before execution. An interrupted execution or a known
infrastructure failure reuses that bundle.

Each bundle gets at most three execution reservations, including interrupted
reservations. Known infrastructure failures retry with bounded backoff. Fixtures,
ordinary compiler errors, unknown failures and test timeouts do not automatically
qualify as infrastructure. Requests retain the failure phase, cause, sanitized
diagnostic, execution history and any separate cleanup failure. Structured test
events and assertion markers must agree with the retained sanitized output.

Scheduling gives uncovered theories an initial opportunity before revisions.
`tests_per_theory` bounds authoring across replacement requests. The attempt's
normal authoring ceiling is the larger of its theory count and its shard count
multiplied by `evidence_rounds_per_shard`. Existing wall-clock and model-turn
limits still apply. Legacy requests keep their old accounting until an explicit
recovery records an additional allowance.

## Retry infrastructure-blocked evidence

After restoring capacity or another failed prerequisite, run:

```sh
ultraplan --workspace /path/to/workspace sprint PROJECT SPRINT qa retry-infrastructure
```

The same operation is available in the web interface and TUI. It uses durable run
ownership and the QA writer fence. It requires a retained adjudicated attempt and
unchanged governed inputs and implementation. It does not remap or rerun
investigators or accepted checks. Valid retained bundles run directly; missing
bundles may require their original investigator to author a test. For sessions
created before the storage migration, a temporary checked symlink exposes the
managed copy at its original workspace path. It is removed when continuation
ends; source data remains on disk. Occupied paths and redirected parents block
restoration rather than being replaced. Only affected
arbitration groups receive new observations.

Recovery records one additional allowance per eligible request, its original
reason and the grant time. Repeating the command does not replenish that
allowance. Older workspace failures without an underlying errno can receive one
prerequisite recovery without being labelled as proven disk failures. Unstarted
requests blocked by an exhausted shard budget qualify when that shard has retained
infrastructure failure evidence. Legacy preparation counters do not exclude a
request that never produced a bundle. Session failures during directory migration
can retry within the existing allowance; they do not receive reset counters.
Ordinary fixture or product failures do not.

Recovery progress distinguishes reused accepted checks from newly executed ones
and reports unresolved request reasons. Completing retained checks does not mean
that the requested product reproductions succeeded.

Reports and interfaces show unresolved request endpoints first. Predecessors and
answered requests remain available as history. The Markdown report also lists
the accepted failing bundle and test assertion for each candidate theory; missing
sibling coverage remains explicit.

For Sprint 39, use `ultraplan-go` and `39-performance-stage` as the project and
sprint arguments. Recovery still requires the original implementation fingerprint;
it cannot admit old evidence against a changed implementation.
