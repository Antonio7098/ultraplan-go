# Server Shutdown and Active Run Cancellation Contract

**Status:** Proposed, normative addendum  
**Applies to:** `docs/plans/integrated-roadmap.md` and `docs/plans/ultraplan-local-server-experiment-plan.md`  
**Scope:** Filesystem-backed local server and all later server-backed UltraPlan modes

## 1. Core rule

> Gracefully stopping the UltraPlan server cancels every active run owned by that server.

The initial local server must not leave agent processes, study run loops, sprint flows, verification attempts, repairs, shell commands, or other server-started operations running after the server that owns their lifecycle, locks, progress streams, and cancellation controls has stopped.

This rule applies whether shutdown begins through:

- a browser or administrative stop action;
- `SIGINT`;
- `SIGTERM`;
- normal process shutdown initiated by the application.

A browser tab closing, navigating away, losing its SSE connection, or temporarily disconnecting does **not** cancel a run. Runs are owned by the server operation lifecycle, not by an individual HTTP request or browser connection.

## 2. Ownership boundary

For the filesystem-backed local product, an operation started through CLI, TUI, or HTTP is owned by the process executing that operation.

A server-owned operation includes:

- its root `context.Context` and cancellation function;
- runtime and provider calls;
- child processes and process groups;
- retries and rate-limit waits;
- task or shard workers;
- scope locks and leases;
- progress publication;
- durable attempt and stage state;
- cleanup and reconciliation responsibility.

The initial server does not support detached background execution that intentionally survives server termination.

A future durable worker architecture may change ownership so runs survive control-plane restart, but that must be introduced as an explicit later contract with durable workers, leases, heartbeats, and reconciliation. It must not emerge accidentally from orphaned local processes.

## 3. Graceful shutdown sequence

When graceful shutdown begins, the server must perform this sequence:

```text
running
  -> draining
  -> cancellation requested for all active server-owned operations
  -> bounded cleanup and reconciliation
  -> durable terminal or interrupted state
  -> HTTP/SSE shutdown
  -> process exit
```

### 3.1 Enter draining state

The server must first:

- mark itself as draining;
- stop accepting new mutating operations;
- reject new run starts with a stable `server_draining` or equivalent error;
- prevent queued work that has not begun from starting;
- allow only the bounded reads needed to display shutdown progress where practical.

### 3.2 Request cancellation

For every active operation, the server must:

- persist or publish `cancellation_requested` where durable operation state exists;
- record `reason: server_shutdown`;
- record the request timestamp;
- invoke the operation's canonical cancellation function exactly once;
- propagate cancellation to nested tasks, runtime adapters, waits, and subprocess execution.

Server shutdown must use the same cancellation and cleanup path as an explicit user cancellation. It must not invent a weaker process-only termination path.

### 3.3 Stop runtime and process trees

Cancellation must reach the runtime boundary and terminate the complete owned process tree according to the existing AgentWrap and runtime cleanup guarantees.

The server must not consider an operation stopped merely because:

- the top-level goroutine returned;
- the immediate child process exited;
- the SSE client disconnected;
- the provider call stopped producing events.

Owned descendants, pipes, temporary resources, locks, and workspaces must be reconciled or reported as uncertain.

### 3.4 Wait for bounded cleanup

The server must wait for a configured, bounded shutdown grace period.

During that period it should:

- continue collecting terminal runtime events;
- allow operation state to reach a durable terminal result;
- wait for process-tree cleanup;
- persist final diagnostics and artifacts that can be captured safely;
- release locks only after the owning operation has stopped or reconciliation has established that it no longer owns live work.

The server must never wait indefinitely.

### 3.5 Persist the correct outcome

After cancellation and cleanup, each operation must end as one of:

- `cancelled`: cancellation was accepted and owned work was stopped cleanly;
- `interrupted`: shutdown prevented normal completion and the full final outcome could not be established;
- `cleanup_uncertain`: cancellation was requested, but descendant-process, workspace, lock, or resource cleanup could not be proven;
- `failed`: a separate failure was already authoritative before shutdown cancellation took effect.

Where the state model supports structured reasons, preserve:

```text
terminal_status: cancelled
cancellation_reason: server_shutdown
```

Do not represent shutdown cancellation as:

- successful completion;
- ordinary user cancellation without the shutdown reason;
- a missing or silently abandoned run;
- a completed stage merely because an artifact happens to exist.

Artifact validation and canonical stage rules remain authoritative after cancellation.

### 3.6 Finish progress streams and stop HTTP

Where possible, publish a final bounded event for each affected operation, such as:

```text
operation_cancelled reason=server_shutdown
operation_interrupted reason=server_shutdown
cleanup_uncertain reason=server_shutdown
```

Then:

- close operation event subscriptions;
- close SSE responses;
- gracefully stop the HTTP server;
- exit after all operations have reached a durable outcome or the grace period has expired and uncertainty has been recorded.

## 4. Browser and administrative UX

### 4.1 Stopping from the browser

If a browser-accessible stop-server action is introduced and active runs exist, the UI must show a confirmation that states:

- how many operations are active;
- their kinds and scopes;
- that stopping the server will cancel them;
- that partial work will be preserved only where current recovery guarantees allow;
- that cleanup uncertainty may require reconciliation on restart.

After confirmation, shutdown proceeds without requiring separate cancellation confirmation for each run.

If no runs are active, the server may stop without the additional active-run warning.

### 4.2 Signals and non-interactive shutdown

`SIGINT` and `SIGTERM` cannot depend on an interactive confirmation. They must begin the graceful shutdown sequence immediately.

A second termination signal may shorten the remaining grace period, but it should still attempt final state persistence before forced exit where possible.

### 4.3 Browser disconnection

The following must not cancel a run:

- closing the browser tab;
- refreshing the page;
- losing network connectivity;
- an SSE timeout;
- reconnecting from another browser session.

The user must be able to reconnect and observe the current durable operation state while the server remains alive.

## 5. Forced termination and crash recovery

`SIGKILL`, host failure, power loss, runtime crash, or an unrecoverable server panic cannot guarantee graceful cancellation.

On the next startup, UltraPlan must reconcile any operation recorded as active but lacking a valid live owner.

Reconciliation should:

1. inspect durable operation, attempt, lock, and runtime state;
2. determine whether an owned process is still alive where this can be checked safely;
3. terminate or quarantine confirmed leftover owned processes where supported;
4. preserve valid partial artifacts and diagnostics without promoting them to canonical success;
5. mark the operation `interrupted` or `cleanup_uncertain`;
6. clear or repair stale locks only after ownership has been checked;
7. expose a recovery recommendation in CLI, TUI, and browser status.

A stale `running` record must never remain indefinitely, and restart reconciliation must never infer success merely from process absence.

## 6. Concurrency and state rules

- Shutdown cancellation is idempotent.
- Each active operation has one authoritative cancellation owner.
- New operations are rejected after draining begins.
- An operation completing concurrently with shutdown uses the existing terminal-outcome arbitration rules; shutdown cancellation must not overwrite an already committed authoritative completion, and late completion must not overwrite an accepted cancellation.
- Lock release follows terminal ownership and cleanup, not HTTP request lifetime.
- The server process must not exit while it still claims active operations without first recording interruption or cleanup uncertainty.
- `flow-state.json` and other summary state must not become a detailed shutdown database; they should retain canonical outcome, freshness, reason, and pointers to detailed attempt evidence.

## 7. Configuration

The server may expose a bounded shutdown grace period, for example:

```yaml
server:
  shutdown_grace_period: 30s
```

Requirements:

- provide a safe default;
- reject invalid or unbounded values;
- include the effective value in diagnostics;
- do not allow configuration to disable cancellation of server-owned runs;
- distinguish the HTTP shutdown timeout from operation cleanup budgets if they need different values.

## 8. Required observability

The browser, CLI, logs, and structured events should make shutdown behavior visible through fields such as:

```text
server_state
drain_started_at
active_operation_count
operation_id
operation_kind
scope
cancellation_requested_at
cancellation_reason
cleanup_status
terminal_status
recovery_required
```

Secrets, full prompts, unsafe environment values, and unbounded provider payloads remain excluded.

## 9. Required tests

### 9.1 Graceful shutdown

Cover:

- stopping an idle server;
- `SIGINT` with one active run;
- `SIGTERM` with several active operations;
- cancellation propagation to nested runtime work;
- process-tree cleanup;
- bounded grace-period expiry;
- final state persistence before exit;
- final SSE event and stream closure;
- lock release after confirmed cleanup;
- rejection of new operations while draining.

### 9.2 Terminal races

Cover:

- operation completes immediately before shutdown cancellation;
- cancellation is accepted before a late successful completion publishes;
- failure becomes authoritative while shutdown is beginning;
- repeated shutdown or cancellation requests;
- second termination signal during cleanup.

Exactly one authoritative terminal outcome must be committed.

### 9.3 Browser behavior

Cover:

- closing a tab does not cancel a run;
- SSE disconnect does not cancel a run;
- reconnect shows current state;
- stop-server confirmation lists active operations;
- confirmed browser shutdown cancels all active runs.

### 9.4 Crash and restart

Cover:

- process death before final state write;
- stale active-operation records;
- stale locks;
- leftover child processes where the platform permits testing;
- valid partial artifacts after interruption;
- startup reconciliation producing `interrupted` or `cleanup_uncertain`, never success.

### 9.5 Parity

The same underlying cancellation, cleanup, terminal arbitration, and recovery behavior must be exercised whether the operation was started from CLI, TUI, or HTTP.

## 10. Exit criteria for the web foundation

The web foundation is not complete until:

- graceful server shutdown cancels every active server-owned run;
- cancellation reaches runtime and owned process trees;
- shutdown waits for bounded cleanup;
- each affected operation receives a durable, truthful outcome;
- browser disconnection is independent from run cancellation;
- restart reconciles abruptly interrupted work;
- active-run shutdown behavior is visible and tested;
- no orphaned local execution is treated as supported detached work.

## 11. Final decision

For the initial filesystem-backed server:

```text
browser disconnect -> run continues
explicit run cancellation -> that run is cancelled
server graceful shutdown -> all server-owned active runs are cancelled
server crash or forced kill -> next startup reconciles them as interrupted or cleanup-uncertain
```

This contract remains in force until UltraPlan deliberately introduces a durable execution service whose workers, leases, and ownership semantics allow runs to survive control-plane restart.