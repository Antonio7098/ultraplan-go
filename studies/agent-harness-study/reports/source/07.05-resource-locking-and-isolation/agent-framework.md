# Source Analysis: agent-framework

## Dimension 07.05: Resource Locking and Isolation

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework (Microsoft Agent Framework) |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | Python 3.10+ (`asyncio`) and .NET/C# (`async`/`Task`); dual-language monorepo |
| Analyzed | 2026-08-23 |

## Summary

Agent Framework protects shared resources through a layered but decentralized model rather than a single central lock manager. There is no `LockManager` abstraction; instead, each resource owner embeds its own synchronization: (1) **files** are protected by 64-way striped `threading.Lock`s plus per-event-loop `asyncio.Lock` striping in the session/history stores (`python/packages/core/agent_framework/_sessions.py:1902-1905`, `python/packages/core/agent_framework/_sessions.py:2438-2455`) and a coarse per-store write lock in the file-access harness (`python/packages/core/agent_framework/_harness/_file_access.py:1343`); (2) **shell sessions** are explicitly declared "single-owner" with an internal run lock that orders commands on one stdin/stdout pipe but deliberately does not provide multi-tenant isolation (`python/packages/tools/agent_framework_tools/shell/_session.py:11-19`, mirrored in `dotnet/src/Microsoft.Agents.AI.Tools.Shell/ShellSession.cs:74-80`); (3) **isolation from untrusted execution** is delegated to sandbox boundaries — Docker containers with hardened defaults (`--network none`, non-root UID, read-only rootfs, capability drop; `python/packages/tools/agent_framework_tools/shell/_docker.py:68-73`, `dotnet/src/Microsoft.Agents.AI.Tools.Shell/DockerShellExecutor.cs:384-394`) and Hyperlight micro-VM sandboxes guarded by a thread-confined actor pattern (`python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:101-131`). Deadlock prevention is handled through consistent lock-ordering discipline (sorted batch acquisition in AG-UI approvals, `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:753-756`) and a dedicated lifecycle-owner task that serializes MCP connect/close without nested locks (`python/packages/core/agent_framework/_mcp.py:1163-1193`). The answer to the dimension's guiding question — *can two tools edit the same file safely?* — is **yes within one process/event loop** (serialized writes + atomic replace), and **no across processes**, where the framework explicitly documents last-writer-wins semantics (`python/packages/core/agent_framework/_sessions.py:1893-1896`, `python/packages/core/agent_framework/_harness/_file_access.py:1340-1342`).

## Rating

**7 / 10** — Clear model with tests, explicit interfaces, and operational safeguards.

Rationale:
- Locks are explicit, named for their purpose, and documented with scope statements ("ordering primitive… NOT a multi-tenant isolation mechanism", `dotnet/src/Microsoft.Agents.AI.Tools.Shell/ShellSession.cs:74-78`).
- Concurrency behavior is proven by targeted tests: concurrent history writes are serialized (`python/packages/core/tests/core/test_sessions.py:1736-1793`), corrupt-snapshot quarantine does not clobber a concurrent replacement (`python/packages/core/tests/core/test_sessions.py:1183`), MCP header-provider calls serialize under concurrency (`python/packages/core/tests/core/test_mcp.py:6532`), and Docker tool rejects isolation-breaking extra args (`python/packages/tools/tests/test_docker_shell_tool.py:231`).
- Sandbox boundaries have hardened defaults that fail closed.
- It falls short of 8+ because: there is no observability of lock contention (no metrics/logs around lock waits anywhere in core), no lock-acquisition timeouts exist (a stuck shell command queues all later commands indefinitely), at least one security-relevant resource (`ContentVariableStore`, `python/packages/core/agent_framework/security.py:345-347`) has no synchronization at all, and cross-process coordination is explicitly out of scope (documented LWW), leaving multi-host deployments to their own devices.

## Evidence Collected

| Area | Evidence | File:Line |
|------|----------|-----------|
| Striped file-lock manager | `_FILE_LOCK_STRIPE_COUNT = 64`; class-level tuple of `threading.Lock`s shared process-wide for session snapshots | `python/packages/core/agent_framework/_sessions.py:1902-1905` |
| Stripe selection | `_session_file_lock()` maps a file path to a stripe via `hash(file_path) % 64` | `python/packages/core/agent_framework/_sessions.py:2067-2070` |
| Atomic snapshot write + LWW disclosure | Docstring states plaintext storage, no cross-process coordination, "last-writer-wins"; write uses temp file + `os.replace` | `python/packages/core/agent_framework/_sessions.py:1878-1898`, `python/packages/core/agent_framework/_sessions.py:2022` |
| Corrupt-snapshot quarantine | Reader re-reads file before quarantining so it never removes a concurrently replaced snapshot | `python/packages/core/agent_framework/_sessions.py:2055-2065` |
| History-file dual locking | Per-loop `asyncio.Lock` striping (`_session_async_write_lock`) + process-local thread locks (`_session_write_lock`) | `python/packages/core/agent_framework/_sessions.py:2438-2455` |
| File-access store coarse lock | `FileAccessProvider._write_lock` serializes ALL mutating tools provider-wide because the store "is intentionally shared across sessions and agents" | `python/packages/core/agent_framework/_harness/_file_access.py:1338-1343` |
| In-memory store check-and-write atomicity | `overwrite=False` create happens under the store lock so two callers cannot both observe a missing file | `python/packages/core/agent_framework/_harness/_file_access.py:646-658` |
| Disk store race hardening | `O_CREAT\|O_EXCL\|O_NOFOLLOW` open closes probe-then-open leaf-symlink race; symlink/reparse-point segment rejection | `python/packages/core/agent_framework/_harness/_file_access.py:891-930` |
| Shell single-owner contract | Module docstring: one session per conversation/user; internal `asyncio.Lock` only serializes onto the pipe; "There is no per-caller isolation" | `python/packages/tools/agent_framework_tools/shell/_session.py:11-19` |
| Shell run/lifecycle locks | `_run_lock` serializes commands; `_lifecycle_lock` prevents double subprocess spawn | `python/packages/tools/agent_framework_tools/shell/_session.py:83-92` |
| .NET shell equivalent | `SemaphoreSlim _runLock(1,1)` + `_lifecycleLock(1,1)` + `_bufferGate` object lock for stdout/stderr buffers | `dotnet/src/Microsoft.Agents.AI.Tools.Shell/ShellSession.cs:74-90` |
| Lazy session creation gate | `lock (this._sessionGate)` ensures one `ShellSession` per executor | `dotnet/src/Microsoft.Agents.AI.Tools.Shell/LocalShellExecutor.cs:148-163` |
| Command policy gate | `ShellPolicy.Evaluate` rejects denied commands before any resource touch | `dotnet/src/Microsoft.Agents.AI.Tools.Shell/LocalShellExecutor.cs:135-140` |
| Docker sandbox defaults | `DEFAULT_NETWORK="none"`, user `65534:65534`, `memory="512m"`, `pids_limit=256`, read-only root | `python/packages/tools/agent_framework_tools/shell/_docker.py:68-73` |
| Docker launch flags | `--cap-drop ALL`, `--security-opt no-new-privileges`, `--read-only`, `--tmpfs /tmp` | `dotnet/src/Microsoft.Agents.AI.Tools.Shell/DockerShellExecutor.cs:384-394` |
| Isolation-breaking arg rejection | Test proves extra docker args cannot weaken network/mount/user flags | `python/packages/tools/tests/test_docker_shell_tool.py:231-235` |
| Docker single-session ownership | Persistent-mode container state visible to every command; sharing across tenants leaks state; stateless mode gives each call a throwaway `docker run --rm` | `python/packages/tools/agent_framework_tools/shell/_docker.py:27-39` |
| Hyperlight thread-confined actor | `_SandboxWorker`: sandbox stored only as worker-local state on a 1-thread executor; exception tracebacks sanitized on worker to avoid cross-thread PyO3 panic | `python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:101-131` |
| Sandbox registry | Per-config cached entries behind `threading.RLock`; all sandbox access routed through entry's single-threaded worker | `python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:969-997` |
| MCP lifecycle locks | Three distinct locks: `_lifecycle_lock`, `_lifecycle_request_lock`, `_function_load_lock` | `python/packages/core/agent_framework/_mcp.py:554-556` |
| Lifecycle-owner task (deadlock avoidance) | connect/close marshalled via queue to a single owner task; futures resolve results; drain loop fails pending waiters on stop | `python/packages/core/agent_framework/_mcp.py:1152-1207` |
| Function-load serialization | `load_prompts` wraps pagination in `_function_load_lock` (`_load_prompts_locked`) | `python/packages/core/agent_framework/_mcp.py:1729-1732` |
| Hosting session-id locks | `_target_lock` guards target resolution; `dict[str, asyncio.Lock]` keyed by session id guards get-or-create | `python/packages/hosting/agent_framework_hosting/_state.py:112-116`, `python/packages/hosting/agent_framework_hosting/_state.py:173-181` |
| Workflow executor serialization | Per-executor `asyncio.Lock` held for entire handler invocation; created lazily per running loop to survive `asyncio.run` reuse | `python/packages/core/agent_framework/_workflows/_executor.py:211-244`, `python/packages/core/agent_framework/_workflows/_executor.py:271` |
| Fan-in edge sync + deadlock note | `lock (_syncLock)` serializes parallel superstep messages; comment mandates replacing lock if method becomes async "to avoid deadlocks" | `dotnet/src/Microsoft.Agents.AI.Workflows/Execution/FanInEdgeState.cs:35-52` |
| Sorted multi-lock acquisition | Batch approval locks sorted by `occurrence_id` before taking them — classic ABBA-deadlock prevention | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:742-756` |
| Approval transition visibility | Every approval lifecycle transition logged with occurrence id/status/owner | `python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:762-778` |
| Session-state value guard | .NET `AgentSessionStateBagValue` guards JSON cache/read-modify-write with a private object lock | `dotnet/src/Microsoft.Agents.AI.Abstractions/AgentSessionStateBagValue.cs:42-66` |
| Deferred persistence gate | `RunPersistenceGate.Collect/FlushAsync/Drop` move side effects after verdict under a short lock (collect outside lock, execute after swap) | `dotnet/src/Microsoft.Agents.AI.AgentHooks/Core/RunPersistenceGate.cs:27-61` |
| Unprotected variable store (gap) | `ContentVariableStore._storage` is a plain dict mutated by `store`/`clear`/`list_variables` with no lock | `python/packages/core/agent_framework/security.py:345-347`, `python/packages/core/agent_framework/security.py:360-403` |
| Concurrency test: history writes | Injected slow write proves second save waits; asserts no overlap and ordered append | `python/packages/core/tests/core/test_sessions.py:1736-1793` |
| Concurrency test: todo mutations | `test_todo_provider_serializes_concurrent_mutations` | `python/packages/core/tests/core/test_harness_todo.py:322` |
| Regression test: loop-bound locks | Executor locks recreated when running loop changes (guards against asyncio.Lock cross-loop errors) | `python/packages/core/tests/workflow/test_workflow.py:1199-1224` |

## Answers to Dimension Questions

**1. Which resources are shared?**
Six classes of shared resources exist: (a) session snapshot files and append-only history files (`python/packages/core/agent_framework/_sessions.py:1795-2084`); (b) harness workspace files via `FileSystemAgentFileStore`/`InMemoryAgentFileStore`, explicitly "shared across sessions and agents" (`python/packages/core/agent_framework/_harness/_file_access.py:1339-1343`); (c) shell sessions and containers (`python/packages/tools/agent_framework_tools/shell/_docker.py:27-39`); (d) code-execution sandboxes cached by config key (`python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:974-997`); (e) long-lived MCP client connections shared across tool calls (`python/packages/core/agent_framework/_mcp.py:554-556`); (f) in-memory registries: hosting session stores, workflow executor state, AG-UI approval occurrences, and the security variable store. Browser and database resources are absent from this source (no browser tool found; DB access appears only in optional stores such as the Azure Blob container-init lock, `dotnet/src/Microsoft.Agents.AI.Hosting.AzureStorage/Blob/AzureBlobAgentSessionStore.cs:154-165`).

**2. What protects them?**
Per-resource embedded primitives, chosen per runtime: `threading.Lock` stripes for cross-thread file safety, `asyncio.Lock`s for coroutine-level ordering, `SemaphoreSlim(1,1)` in .NET, plain C# `lock` for short critical sections, and actor/thread-confinement for foreign-runtime objects. Access control is layered separately through approval middleware (file-access write tools default to `approval_mode="always_require"`, `python/packages/core/agent_framework/_harness/_file_access.py:1325-1337`) and command policies (`dotnet/src/Microsoft.Agents.AI.Tools.Shell/LocalShellExecutor.cs:135-140`). The framework is careful to separate these: the shell docstring stresses the run lock is an ordering primitive, while tenant isolation comes from the ownership contract and the container boundary (`python/packages/tools/agent_framework_tools/shell/_session.py:16-19`).

**3. Are locks coarse or fine-grained?**
Mixed, deliberately matched to contention risk. Fine-to-medium: 64-stripe path-keyed locks let different session files proceed in parallel while serializing collisions (`python/packages/core/agent_framework/_sessions.py:1902-1905`), per-session-id locks in hosting (`python/packages/hosting/agent_framework_hosting/_state.py:116`), per-executor locks in workflows (`python/packages/core/agent_framework/_workflows/_executor.py:232-244`), and per-identity locks in AG-UI approvals (`python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:261-262`). Coarse: the entire `FileAccessProvider` write surface shares ONE lock regardless of which file is touched (`python/packages/core/agent_framework/_harness/_file_access.py:1338-1343`) — correct but a throughput bottleneck since two agents editing *different* files still queue.

**4. Can deadlocks occur?**
No structural ABBA deadlock was found, and several mechanisms actively prevent one: batch lock acquisition is sorted by identity (`python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:753-756`); MCP lifecycle operations are funneled through a single owner task instead of nested locks (`python/packages/core/agent_framework/_mcp.py:1163-1193`); .NET fan-in code carries an explicit maintenance rule to never make the locked section async (`dotnet/src/Microsoft.Agents.AI.Workflows/Execution/FanInEdgeState.cs:39-41`); event streams disable synchronous continuations to prevent deadlocks (`dotnet/src/Microsoft.Agents.AI.Workflows/Execution/StreamingRunEventStream.cs:43`). However, **no lock acquisition uses a timeout**: a hung shell command holding `_run_lock` blocks all subsequent commands forever (the timeout applies to the command, then close/interrupt proceeds via `_lifecycle_lock`, `dotnet/src/Microsoft.Agents.AI.Tools.Shell/ShellSession.cs:63-66` — a mitigation, but queueing behind a wedged pipe is still possible). Risk is bounded because lock depth is shallow (rarely more than index-guard → item-lock).

**5. Are resource conflicts visible?**
Partially. Conflicts surface as exceptions or logs at specific layers: corrupt snapshots raise actionable errors naming quarantine paths (`python/packages/core/agent_framework/_sessions.py:1959-1973`); approval transitions log occurrence ids/statuses (`python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:762-778`); shell tools support audit hooks (`on_command`, `python/packages/tools/agent_framework_tools/shell/_docker.py:350`). But there is **no generic lock-contention telemetry** — searching core for contention/wait metrics found nothing — and last-writer-wins overwrites across processes happen silently by design (`python/packages/core/agent_framework/_sessions.py:1893-1896`).

## Architectural Decisions

1. **No central lock manager; locks live with the resource owner.** Each subsystem owns its synchronization and documents its scope inline (e.g., `dotnet/src/Microsoft.Agents.AI.Tools.Shell/ShellSession.cs:74-80`). This keeps coupling low but means guarantees vary per package and must be re-learned each time.
2. **Isolation via ownership contracts + OS/container boundaries, not shared-resource mediation.** Rather than arbitrating concurrent access to one shared shell/file tree, the framework mandates one session/tool instance per conversation and provides stateless modes (`docker run --rm`) for genuinely shared use (`python/packages/tools/agent_framework_tools/shell/_docker.py:32-39`).
3. **Striped class-level locks for filesystem durability paths.** A fixed 64-stripe pool shared by all instances trades a small false-sharing probability for bounded memory and no per-instance setup (`python/packages/core/agent_framework/_sessions.py:1902-1905`).
4. **Atomicity at the OS layer wherever possible.** Temp-file + `os.replace` for snapshots/todos/memory files (`python/packages/core/agent_framework/_sessions.py:2022`, `python/packages/core/agent_framework/_harness/_todo.py:433-440`, `python/packages/core/agent_framework/_harness/_memory.py:123-133`) and `O_EXCL|O_NOFOLLOW` for create-if-absent semantics (`python/packages/core/agent_framework/_harness/_file_access.py:908-930`) mean crash-consistency does not depend on lock correctness alone.
5. **Actor confinement for foreign-runtime resources.** The Hyperlight sandbox cannot be touched off its creating thread (PyO3 `unsendable`), so the design wraps it in a single-thread executor actor and sanitizes exceptions on the worker (`python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:110-125`) — turning a runtime constraint into the concurrency strategy itself.

## Notable Patterns

- **Double-checked lazy locking**: target resolution acquires the lock only when uncached (`python/packages/hosting/agent_framework_hosting/_state.py:126-137`); executor locks rebind per event loop (`python/packages/core/agent_framework/_workflows/_executor.py:240-244`).
- **Guard lock → per-item lock two-level scheme**: AG-UI approvals use an index `RLock` to look up per-occurrence locks, then release the guard before acquiring items (`python/packages/ag-ui/agent_framework_ag_ui/_approval_lifecycle.py:758-760`); skills caching mirrors it with `_locks_guard` (`python/packages/core/agent_framework/_skills.py:3912-3946`).
- **Collect-under-lock, flush-outside-lock**: `RunPersistenceGate` swaps the pending list inside the lock but executes persistence callbacks outside it (`dotnet/src/Microsoft.Agents.AI.AgentHooks/Core/RunPersistenceGate.cs:36-51`) — avoids holding a lock across awaitable I/O.
- **Sentinel protocol with cryptographic tagging** to attribute stdout to exactly one command even against hostile output (`dotnet/src/Microsoft.Agents.AI.Tools.Shell/ShellSession.cs:107-119`), complemented by persistent reader tasks with buffer offsets to prevent stderr bleeding into the next command (`python/packages/tools/agent_framework_tools/shell/_session.py:32-37`).
- **Quarantine-on-corruption** instead of delete-on-corruption, with a compare-before-move to avoid racing a concurrent replacement (`python/packages/core/agent_framework/_sessions.py:2055-2065`).

## Tradeoffs

- **Correctness vs. throughput in the file harness**: one provider-wide write lock makes read-modify-write tools safe across agents but serializes unrelated file edits (`python/packages/core/agent_framework/_harness/_file_access.py:1338-1342`).
- **Simplicity vs. multi-process support**: process-local locks plus atomic replace give crash safety but silently accept LWW data loss across processes/hosts — an accepted, documented limitation (`python/packages/core/agent_framework/_sessions.py:1893-1898`), pushing burden onto hosts (e.g., Foundry-hosted stores).
- **Hardened-by-default vs. flexibility**: Docker defaults (no network, nobody user, RO rootfs) are safe but restrictive; escape hatches exist yet are validated — isolation-weakening `extra_run_args` are rejected (`python/packages/tools/tests/test_docker_shell_tool.py:231-235`), though `_validate_extra_run_args` remains a denylist-style control that must track new dangerous flags.
- **Single-owner shells vs. multi-user hosting**: strong isolation story per conversation, but hosts that naively share a tool instance get silent cross-tenant state leakage; the docs warn loudly but nothing enforces it at runtime (`python/packages/tools/agent_framework_tools/shell/_session.py:16-18`).

## Failure Modes / Edge Cases

- **Wedged pipe stalls the session**: commands serialize on `_run_lock`; a command whose process ignores sentinel/interrupt handling blocks successors until the interrupt grace expires and a hard respawn occurs (`dotnet/src/Microsoft.Agents.AI.Tools.Shell/ShellSession.cs:63-66`). No general acquisition timeout exists.
- **Cross-process LWW overwrite**: two processes writing the same session snapshot lose one writer's data without error (`python/packages/core/agent_framework/_sessions.py:1893-1896`).
- **Stripe collision**: two hot session files hashing to the same stripe contend spuriously (probability-bounded by the 64-stripe pool, `python/packages/core/agent_framework/_sessions.py:1902-1905`); benign but unobservable.
- **Windows symlink gap**: `O_NOFOLLOW` is POSIX-only, so leaf-segment symlink protection on Windows relies solely on lstat probes with an inherent TOCTOU window acknowledged in comments (`python/packages/core/agent_framework/_harness/_file_access.py:913-915`).
- **Event-loop affinity bugs**: `asyncio.Lock` bound to a dead loop raises on reuse; mitigated by per-loop lock recreation with regression tests (`python/packages/core/tests/workflow/test_workflow.py:1199-1224`).
- **Unsynchronized variable store**: concurrent `store`/`clear` on `ContentVariableStore` can lose entries or raise during iteration (`list_variables` iterates a live dict's keys view copy — safe-ish, but mutation during `clear` is racy) (`python/packages/core/agent_framework/security.py:345-403`).
- **Corrupt snapshot handling is version-sensitive**: schema/version failures intentionally preserve the original file (only syntactic decode failures quarantine), so a bad-version file loops until manually fixed (docstring, `python/packages/core/AGENTS.md` summary of `FileSessionStore`; implementation at `python/packages/core/agent_framework/_sessions.py:1974-1978`).

## Future Considerations

- Add optional cross-process file locking (e.g., advisory `flock`/lock files) or lease-based coordination for `FileSessionStore`/`FileHistoryProvider` to upgrade the documented LWW posture.
- Instrument lock acquisition (wait time, hold time) via the existing OTel pipeline so contention becomes observable; today nothing distinguishes "slow because contended" from "slow because work".
- Introduce acquisition timeouts or cancellation-aware waits for shell run locks, surfacing "queued behind command X" diagnostics.
- Protect `ContentVariableStore` with a lock (it handles untrusted-content indirection, a security feature) or document single-thread assumption explicitly (`python/packages/core/agent_framework/security.py:345-347`).
- Consider per-path (or sharded) write locks in `FileAccessProvider` to recover parallelism for independent files while keeping read-modify-write atomicity per path.
- Runtime enforcement (not just docs) of the shell single-owner contract, e.g., opt-in ownership tokens that reject cross-session calls (`python/packages/tools/agent_framework_tools/shell/_session.py:11-19`).

## Questions / Gaps

- **Database transactions**: No evidence of DB transaction management in the studied source. Searched `dotnet/src` for transaction/isolation-level patterns and found only a container-initialization lock in Azure Blob storage (`dotnet/src/Microsoft.Agents.AI.Hosting.AzureStorage/Blob/AzureBlobAgentSessionStore.cs:154-165`); Cosmos NoSQL and Redis packages were not exhaustively inspected within the allowed boundary.
- **Browser isolation**: No browser automation tool exists in this source; searched `python/packages` for browser/playwright/puppeteer symbols with no relevant hits.
- **Secrets management**: Secrets are addressed indirectly (environment sanitization, `dotnet/src/Microsoft.Agents.AI.Tools.Shell/EnvironmentSanitizer.cs:20`; warnings that session files are "not a secret store", `python/packages/core/agent_framework/_sessions.py:1887-1898`), but no dedicated secret-store resource with locking was found.
- **Contention behavior under load**: No benchmarks or soak tests for striped-lock distribution were found; actual stripe-collision impact is unquantified.
- **.NET-side parity tests**: Python has explicit concurrency tests for file stores; the equivalent .NET `ShellSession` unit tests directory contains no concurrency-named tests (searched `dotnet/tests/Microsoft.Agents.AI.Tools.Shell.UnitTests` for concurren*/parallel* — no matches), so .NET serialization guarantees are asserted by design rather than demonstrated by test.

---

Generated by dimension 07.05 (Resource Locking and Isolation) against `agent-framework`.
