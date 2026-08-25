# Source Analysis: agent-framework

## Dimension 17.01: Sandbox Boundary

### Source Info

| Field | Value |
|-------|-------|
| Name | agent-framework |
| Path | `studies/agent-harness-study/sources/agent-framework` |
| Language / Stack | C# (.NET), Python, (Go stub only); Microsoft Agent Framework monorepo |
| Analyzed | 2026-08-24 |

## Summary

The repository does not funnel all tool execution through a single sandbox. Instead it ships a **portfolio of execution environments**, each with an explicitly documented threat model, and lets the developer choose the boundary strength:

1. **VM/WASM-isolated code interpreter (Hyperlight)** — `execute_code` tools in both Python (`python/packages/hyperlight`) and .NET (`dotnet/src/Microsoft.Agents.AI.Hyperlight`) run model-generated Python (or JS) inside a Hyperlight micro-VM/WASM guest, with host-mediated file mounts, an outbound network domain allow-list, and snapshot/restore lifecycle.
2. **Language-restricted in-process interpreter (Monty)** — `python/packages/monty` runs generated code in the Rust-based Monty interpreter where OS/filesystem/network calls are denied by default at the interpreter bridge level and mounts/resource-limits are opt-in.
3. **Container-isolated shell** — `dotnet/src/Microsoft.Agents.AI.Tools.Shell/DockerShellExecutor.cs` runs shell commands in a Docker container with restrictive-by-default flags; `LocalShellExecutor.cs` runs shells directly on the host and declares *approval-in-the-loop* as the security boundary instead of isolation.
4. **Local child-process CodeAct** — `dotnet/src/Microsoft.Agents.AI.LocalCodeAct` executes Python in a child process with AST allow-list validation and resource caps while explicitly stating it is **not** a sandbox and requires external isolation.
5. **Provider-hosted remote execution** — hosted `code_interpreter` tools (Foundry/OpenAI) delegate execution to the service's container ("auto" container), moving the boundary out of process entirely.

Boundary configurability is a first-class concern: tools, file mounts, network allow-lists, backend choice, heap/stack sizes, resource limits, and approval modes are all constructor/runtime options, and sandboxes are rebuilt or re-snapshotted when configuration fingerprints change.

## Rating

**8 / 10** — Clear, layered model with explicit per-environment threat models, extensive tests for boundary logic (symlink escape rejection, restrictive Docker defaults, config fingerprints, approval gating), and honest documentation of what is *not* a sandbox. Not 9–10 because: the local executors deliberately have no enforced isolation (approval-only), Docker isolation depends on caller-supplied `ExtraRunArgs`, the .NET Hyperlight package wires only a single input directory rather than per-mount mounts (`dotnet/src/Microsoft.Agents.AI.Hyperlight/Internal/SandboxExecutor.cs:200-207`), and Monty/Hyperlight are pre-stable (beta/pre-release).

## Evidence Collected

Every entry includes a workspace-relative file path with line numbers. Paths are relative to the workspace root; all files live under the selected source directory.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Hyperlight = VM-isolated sandbox | README states Hyperlight VM-isolated sandbox; `SandboxBackend.Wasm`/`JavaScript` factories | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Hyperlight/README.md:5`; `.../HyperlightCodeActProviderOptions.cs:37-52` |
| Default WASM backend (Python) | `DEFAULT_HYPERLIGHT_BACKEND = "wasm"`, module `python_guest.path` | `studies/agent-harness-study/sources/agent-framework/python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:29-30` |
| Thread-confined sandbox actor | `_SandboxWorker` keeps unsendable PyO3 sandbox on one thread; sanitized exceptions prevent cross-thread Drop | `studies/agent-harness-study/sources/agent-framework/python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:101-126` |
| Input staging: symlink/junction rejection | `_copy_path` rejects links/reparse points; containment check under configured root | `studies/agent-harness-study/sources/agent-framework/python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:639-671`, `391-417` |
| Output read: TOCTOU hardening | `O_NOFOLLOW` + lstat/fstat inode identity check before reading `/output` files | `studies/agent-harness-study/sources/agent-framework/python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:685-715` |
| Output safety validation | `_is_safe_output_file` walks components with lstat, rejects `..` and links | `studies/agent-harness-study/sources/agent-framework/python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:745-790` |
| Mount path confinement | mount paths must stay under `/input`; `..` rejected | `studies/agent-harness-study/sources/agent-framework/python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:540-557` |
| Network egress allow-list | `sandbox.allow_domain(target, methods)` registration; scheme-expansion retry | `studies/agent-harness-study/sources/agent-framework/python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:1040-1066`, `523-537` |
| Per-config sandbox cache | `_RunConfig.cache_key()` drives cached sandbox entries keyed by backend/tools/mounts/domains | `studies/agent-harness-study/sources/agent-framework/python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:76-87`, `990-997` |
| Host-tool bridge into sandbox | sync FFI callback wraps async `FunctionTool.invoke` on dedicated thread | `studies/agent-harness-study/sources/agent-framework/python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:910-942` |
| Symlink-escape tests (Python Hyperlight) | tests: reject symlink/junction input staging, output listing skips symlinked dirs/files, parent traversal rejected | `studies/agent-harness-study/sources/agent-framework/python/packages/hyperlight/tests/hyperlight/test_hyperlight_codeact.py:632-1002` |
| Monty default-deny OS access | OS functions resumed with `PermissionError("OS and filesystem calls are not available.")` | `studies/agent-harness-study/sources/agent-framework/python/packages/monty/agent_framework_monty/_monty_bridge.py:262-267`; docstring `.../_execute_code_tool.py:8` |
| Monty unknown-function denial | unregistered names resume as `NameError` | `studies/agent-harness-study/sources/agent-framework/python/packages/monty/agent_framework_monty/_monty_bridge.py:276-279` |
| Monty opt-in mounts + limits | `mount=`/`limits=` forwarded to `Monty.start`; modes `read-only`/`read-write`/`overlay` with `write_bytes_limit` | `studies/agent-harness-study/sources/agent-framework/python/packages/monty/agent_framework_monty/_monty_bridge.py:238-243`; `.../README.md:108-149` |
| Monty resource limits API | `resource_limits` → `pydantic_monty.ResourceLimits` (duration, memory, recursion, allocations) | `studies/agent-harness-study/sources/agent-framework/python/packages/monty/README.md:146-149`; `.../tests/monty/test_monty_codeact.py:363` |
| Monty OS-rejection test | `test_run_code_os_function_is_rejected_with_permissionerror` | `studies/agent-harness-study/sources/agent-framework/python/packages/monty/tests/monty/test_monty_codeact.py:523` |
| .NET Hyperlight sandbox lifecycle | `SandboxExecutor` builds `SandboxBuilder`, registers tools, `AllowDomain`, warm snapshot restore per run | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Hyperlight/Internal/SandboxExecutor.cs:174-228`, `125-155` |
| Config-fingerprint rebuild | `EnsureInitialized` disposes and rebuilds sandbox when fingerprint changes | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Hyperlight/Internal/SandboxExecutor.cs:157-172` |
| Guest resource sizing options | `HeapSize`/`StackSize` configurable (e.g. `"50Mi"`) | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Hyperlight/HyperlightCodeActProviderOptions.cs:60-70` |
| Single-input-dir limitation (documented) | SDK exposes only single input+output surface; `FileMount`s advertised in description but not wired | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Hyperlight/Internal/SandboxExecutor.cs:200-207` |
| Approval computation for execute_code | `ComputeApprovalRequired(mode, tools)`; `AlwaysRequire` when guest can reach tools | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Hyperlight/HyperlightCodeActProvider.cs:313-314`; tests `.../tests/Microsoft.Agents.AI.Hyperlight.UnitTests/ApprovalComputationTests.cs` |
| Fingerprint unit tests | different tools/order/mounts/domains/hostInput produce different fingerprints | `studies/agent-harness-study/sources/agent-framework/dotnet/tests/Microsoft.Agents.AI.Hyperlight.UnitTests/SandboxExecutorTests.cs:12-103` |
| Docker shell default image & flags | `mcr.microsoft.com/azurelinux/base/core:3.0`, network none, 512 MiB, 256 pids | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Tools.Shell/DockerShellExecutor.cs:62-83` |
| Docker argv hardening | `--cap-drop ALL --security-opt no-new-privileges --tmpfs /tmp:rw,nosuid,nodev,size=64m`, optional `--read-only`, non-root user, ro/rw volume mount | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Tools.Shell/DockerShellExecutor.cs:360-418` (persistent), `538-568` (stateless) |
| "Best-effort, not a guarantee" caveat | container intended as boundary but depends on kernel/runtime/image/ExtraRunArgs; approval is primary control | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Tools.Shell/DockerShellExecutor.cs:20-37`, `253-268` |
| Docker defaults tested | `BuildRunArgv_EmitsRestrictiveDefaults` asserts cap-drop/no-new-privileges/read-only/memory/pids | `studies/agent-harness-study/sources/agent-framework/dotnet/tests/Microsoft.Agents.AI.Tools.Shell.UnitTests/DockerShellExecutorTests.cs:17-45` |
| Local shell threat model | deny list is guardrail not boundary; real isolation = human approval or container; `acknowledgeUnsafe:true` required to skip approval | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Tools.Shell/LocalShellExecutor.cs:43-50`, `339-347` |
| Shell policy evaluation | deny-first regex lists, exclusive allow list, custom callback last | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.Tools.Shell/ShellPolicy.cs:187-214` |
| Local CodeAct: not a sandbox | XML docs state NOT a sandbox; requires external process/fs/net isolation | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.LocalCodeAct/LocalCodeActProvider.cs:19-30`; csproj description `.../Microsoft.Agents.AI.LocalCodeAct.csproj:25` |
| Resource limits ≠ sandbox | `ProcessExecutionLimits` timeouts/output caps are defense-in-depth only | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.LocalCodeAct/ProcessExecutionLimits.cs:8-34` |
| Child-process execution env | `ProcessBridge` spawns configured python with `-I` (isolated mode) runner script | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.LocalCodeAct/Internal/ProcessBridge.cs:64-74` |
| AST validation subprocess | `CodeValidator` runs embedded Python AST validator with strict timeout; allowed/blocked imports & builtins | `studies/agent-harness-study/sources/agent-framework/dotnet/src/Microsoft.Agents.AI.LocalCodeAct/Internal/CodeValidator.cs:16`; `.../LocalCodeActProvider.cs:55-67` |
| Hosted remote execution | `get_code_interpreter_tool()` returns SDK `CodeInterpreterTool(container={"type":"auto"})`; sanitizer injects default container | `studies/agent-harness-study/sources/agent-framework/python/packages/foundry/agent_framework_foundry/_chat_client.py:372-392`; `.../_tools.py:45-48,67-68` |
| Run-scoped tool snapshots | `create_run_tool()` snapshots tools/mounts/domains per run (Python Hyperlight & Monty) | `studies/agent-harness-study/sources/agent-framework/python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:1256-1271`; `.../monty/agent_framework_monty/_execute_code_tool.py:313` |
| Devcontainer boundary (dev environment) | devcontainer uses docker-in-docker feature for test isolation | `studies/agent-harness-study/sources/agent-framework/.devcontainer/devcontainer.json:6-8` |
| Go implementation absent from source | `go/README.md:1-3` points to separate repo; no Go harness code to assess | `studies/agent-harness-study/sources/agent-framework/go/README.md:1-3` |

## Answers to Dimension Questions

**1. Where does code execute?**
Five distinct places, chosen per-package by the developer:
- Hyperlight guest (WASM or JavaScript backend) via `hyperlight_sandbox.Sandbox(backend=..., module=...)` (`studies/agent-harness-study/sources/agent-framework/python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:1025-1038`) and .NET `SandboxBuilder.WithBackend(...)` (`.../dotnet/src/Microsoft.Agents.AI.Hyperlight/Internal/SandboxExecutor.cs:176-209`).
- In-process restricted Monty interpreter (`studies/agent-harness-study/sources/agent-framework/python/packages/monty/agent_framework_monty/_monty_bridge.py:232-243`) — same process as the host app, but a non-CPython interpreter with mediated calls.
- Host-local child processes: .NET LocalCodeAct spawns `python -I <runner>` (`.../Internal/ProcessBridge.cs:64-74`); `LocalShellExecutor` spawns bash/sh/pwsh/cmd on the host (`.../Tools.Shell/LocalShellExecutor.cs:190-201`).
- Docker containers via the `docker` CLI (`.../Tools.Shell/DockerShellExecutor.cs:360-444`).
- Provider-side hosted containers for `code_interpreter` tools, where the framework only serializes a tool descriptor with `{"type": "auto"}` container (`.../python/packages/foundry/agent_framework_foundry/_tools.py:45-48`) — execution happens in the cloud service.

**2. What boundaries exist between agents and the host?**
- Hyperlight: hypervisor/WASM memory isolation around guest code; host exposure limited to staged `/input` (copy, link-free), captured `/output` files, registered host tools over FFI, and allow-listed HTTP domains. Host-side staging/reading code adds its own symlink/junction/TOCTOU defenses (`.../python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:639-790`).
- Monty: language-level capability boundary — OS calls intercepted and refused with `PermissionError`; filesystem only via explicit mounts with `read-only`/`read-write`/`overlay` modes and byte caps; CPU/memory via `ResourceLimits` (`.../python/packages/monty/agent_framework_monty/_monty_bridge.py:262-267`; README `.../python/packages/monty/README.md:140-149`).
- Docker shell: namespace/cgroup boundary via runtime flags (no network, non-root, cap-drop ALL, no-new-privileges, memory/pids caps, tmpfs, optional read-only root) (`.../Tools.Shell/DockerShellExecutor.cs:25-27, 376-401`).
- Local shell / LocalCodeAct: **no isolation boundary**; compensating controls are human approval (`ApprovalRequiredAIFunction`, refusal without `acknowledgeUnsafe`), regex policy deny/allow lists, AST import/builtin allowlists, timeouts, and output-size caps (`.../Tools.Shell/LocalShellExecutor.cs:43-50`; `.../LocalCodeAct/ProcessExecutionLimits.cs:8-11`).

**3. Are boundaries enforced?**
Yes where they claim to be, with evidence:
- Enforcement lives mostly in external runtimes (Hyperlight guest, Docker daemon, Monty interpreter), which the framework configures but does not itself implement — appropriate layering.
- The framework's own enforcement code is tested: symlink/junction escape rejection tests (`.../python/packages/hyperlight/tests/hyperlight/test_hyperlight_codeact.py:632-1002`), Monty OS-rejection test (`.../python/packages/monty/tests/monty/test_monty_codeact.py:523`), Docker restrictive-defaults assertions (`.../dotnet/tests/Microsoft.Agents.AI.Tools.Shell.UnitTests/DockerShellExecutorTests.cs:17-45`), and approval-gating tests (`.../DockerShellExecutorTests.cs:162-192`).
- Honest negative claims are also enforced: `LocalShellExecutor.AsAIFunction` throws unless `requireApproval` or `acknowledgeUnsafe` (`.../Tools.Shell/LocalShellExecutor.cs:341-347`). One residual risk called out in docs: auto-approval rules that match tool names can bypass approval if names collide (`.../DockerShellExecutor.cs:262-268`).

**4. Can sandbox configuration be changed per-run?**
Yes, extensively:
- Dynamic mutation APIs: `add_tools/remove_tool/clear_tools`, `add_file_mounts/remove_file_mount/clear_file_mounts`, `add_allowed_domains/remove_allowed_domain/clear_allowed_domains` (`.../python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:1148-1245`).
- Run scoping: `create_run_tool()` produces an immutable per-run snapshot (`.../_execute_code_tool.py:1256-1271`).
- Rebuild-on-change: Python caches sandboxes per `_RunConfig.cache_key()` (`.../_execute_code_tool.py:76-87`); .NET rebuilds when the `ConfigFingerprint` changes, including fingerprint tests (`.../dotnet/src/Microsoft.Agents.AI.Hyperlight/Internal/SandboxExecutor.cs:157-172`; tests `SandboxExecutorTests.cs:12-103`).
- Backend/sizing choices: WASM vs JavaScript backend, custom guest module path, heap/stack size (`.../HyperlightCodeActProviderOptions.cs:26-70`); Monty resource limits dict (`.../python/packages/monty/README.md:128-149`).

## Architectural Decisions

1. **Portfolio over monoculture.** Rather than one sandbox abstraction, the framework ships parallel implementations with a shared conceptual shape (`execute_code` + provider/tool wiring + instructions) so developers pick isolation strength: Hyperlight (strong), Monty (medium, in-process), Docker shell (container), local+approval (weakest). Design docs describe this as a portable capability model across backends (`studies/agent-harness-study/sources/agent-framework/docs/features/code_act/python-implementation.md:31,286`; dotnet counterpart `.../docs/features/code_act/dotnet-implementation.md:371-385`).
2. **Explicit non-boundaries.** Packages that cannot isolate say so in type-adjacent docs and even in assembly descriptions (`.../dotnet/src/Microsoft.Agents.AI.LocalCodeAct/Microsoft.Agents.AI.LocalCodeAct.csproj:25`; `ProcessExecutionLimits.cs:9-11`). This prevents the false sense of safety that plagues local-execution tools.
3. **Approval as orthogonal control.** Approval gating (`ApprovalMode`/`CodeActApprovalMode`, `ApprovalRequiredAIFunction`) is layered on top of every executor, including sandboxed ones (`.../HyperlightCodeActProvider.cs:313-314`; `.../DockerShellExecutor.cs:277-305`).
4. **Host-mediated I/O.** Guests never touch the host FS directly: inputs are copied into temp dirs after link-rejection; outputs are validated then returned as `Content.from_data` attachments (`.../python/packages/hyperlight/agent_framework_hyperlight/_execute_code_tool.py:674-682, 817-848`).
5. **Snapshot/restore lifecycle.** Sandboxes are warmed once and reset to a clean snapshot between invocations, giving deterministic state per call (`.../dotnet/.../SandboxExecutor.cs:132-135, 219-227`; `.../python/.../_execute_code_tool.py:210-229`).
6. **Thread-confinement for FFI safety.** The Python worker-actor pattern treats the PyO3 `unsendable` constraint as an architectural invariant, keeping sandbox objects worker-local and returning only sendable data (`.../_execute_code_tool.py:101-126`).

## Notable Patterns

- **Defense-in-depth against symlink attacks**: separate validators for staging (`_copy_path`), walking (`_iter_real_entries`), reading (`_read_output_file_bytes` with `O_NOFOLLOW` + fstat identity), and listing (`_is_safe_output_file`) — each independently tested (`.../python/packages/hyperlight/...` lines above).
- **Restrictive-baseline container defaults** with escape hatch: secure flags are default-on (`--network none`, cap-drop ALL), while `ExtraRunArgs` allows overrides — with an explicit warning that overrides weaken the boundary (`.../DockerShellExecutor.cs:27-30`).
- **Configuration fingerprints** to decide sandbox reuse vs rebuild, keeping per-run customization cheap (`SandboxExecutorTests.cs:12-103`).
- **Capability advertisement through prompts**: the sandbox tool description dynamically enumerates mounted paths, callable tools, and allowed domains so the model knows the boundary contents (`InstructionBuilder.cs:44-62` in Hyperlight Internal; `build_execute_code_description` in Python).
- **Graceful degradation probes**: `IsAvailableAsync` checks docker reachability before use (`.../DockerShellExecutor.cs:312-353`); Python reports missing hypervisor/backend as structured skip reasons (`test_hyperlight_codeact.py:1250`).

## Tradeoffs

- **Strong isolation vs operational weight**: Hyperlight requires hypervisor/WASM backend availability and platform support; failures degrade to runtime errors with install guidance (`.../_execute_code_tool.py:307-316, 1034-1038`).
- **In-process speed vs weaker boundary (Monty)**: Monty avoids VM overhead and works everywhere, but shares the host process; protection is interpreter-mediated, and exotic Python features are unsupported (`.../python/packages/monty/README.md:163-176`).
- **Docker convenience vs kernel-shared isolation**: docs admit container flags are best-effort and recommend gVisor/Kata/VMs for high stakes (`.../DockerShellExecutor.cs:28-36`).
- **Permissiveness of defaults varies**: Hyperlight `execute_code` defaults to `never_require` approval (isolation replaces approval), while local shell defaults to `always_require` — coherent but easy to misconfigure when mixing packages (`.../HyperlightExecuteCodeTool` ctor `.../_execute_code_tool.py:1101-1110` vs `LocalShellExecutor.cs:339-347`).
- **Portability vs completeness in .NET mounts**: per-mount `FileMount` config exists in the object model but the SDK gap means only a single input dir is actually wired (`.../Internal/SandboxExecutor.cs:200-207`).

## Failure Modes / Edge Cases

- **Cross-thread guest drop panic**: PyO3 unsendable sandbox dropped off-thread panics unrecoverably; mitigated by the worker-actor plus traceback sanitization, with fallback "leak rather than panic" during teardown (`.../_execute_code_tool.py:117-126, 272-275`).
- **TOCTOU swaps on `/output`**: a malicious payload replacing output files with symlinks mid-read is refused via inode identity checks (`.../_execute_code_tool.py:685-715`).
- **Timeout/cancellation container leaks**: stateless docker runs kill the container explicitly on timeout or caller cancellation because `--rm` alone would leak it (`.../DockerShellExecutor.cs:506-523`).
- **Persistent-session state bleed**: shared persistent executors across users leak state; docs mandate per-session ownership or stateless mode (`.../DockerShellExecutor.cs:46-57`; `.../LocalShellExecutor.cs:32-41`).
- **Name-collision approval bypass**: auto-approval matching by tool name can silently approve shell commands if names collide across features (`.../DockerShellExecutor.cs:262-268`).
- **Mid-stage workspace mutation**: `_copy_path` documents that concurrent modification of the source tree is not atomic; callers needing guarantees must supply immutable snapshots (`.../_execute_code_tool.py:649-653`).

## Future Considerations

- Wire the richer Hyperlight mount API when the .NET SDK exposes it, closing the gap between the `FileMount` model and actual sandbox wiring (`.../Internal/SandboxExecutor.cs:200-207`).
- Promote Monty beyond beta and expose virtual-filesystem (`OSAccess`) and URL allow-list primitives currently marked "Not exposed" (`.../python/packages/monty/AGENTS.md` capability table).
- Add end-to-end (daemon-present) CI coverage for Docker boundary flags; current unit tests validate argv construction only (`.../tests/Microsoft.Agents.AI.Tools.Shell.UnitTests/DockerShellExecutorTests.cs:355-357` comment notes pure builders so tests don't need Docker).
- Consider making the portable CodeAct capability model (workspace_root/file_mounts/allowed_domains semantics) contractually identical across Hyperlight/Monty/local backends, as flagged in design docs (`.../docs/features/code_act/python-implementation.md:286`).

## Questions / Gaps

- **No evidence found** for a Windows/macOS-native sandbox primitive (e.g., Job Objects, Seatbelt) anywhere in the tree; searches for `sandbox` outside the Hyperlight/Monty/Docker packages surfaced only docs and hosted-tool plumbing. Boundary options are limited to the five environments listed.
- **No evidence found** for per-run network egress enforcement in .NET Hyperlight beyond `AllowDomain` pass-through; whether the underlying hyperlight-sandbox proxy enforces method-level restrictions could not be verified from this source (the SDK is an external NuGet dependency, `.../Microsoft.Agents.AI.Hyperlight.csproj:15`).
- **No evidence found** of resource-limit configuration for the Python Hyperlight tool (no heap/CPU knobs equivalent to .NET `HeapSize`/`StackSize` were present in `HyperlightExecuteCodeTool.__init__`, `.../_execute_code_tool.py:1088-1126`); searched `resource`, `limit`, `heap`, `timeout` within the package.
- Whether the hosted `code_interpreter` container persists state across calls is decided entirely service-side and is invisible in this repository (`.../python/packages/foundry/agent_framework_foundry/_tools.py:36-53` only shapes the request payload).
- The Go implementation is out of scope here by absence — the directory contains only a pointer to a separate repository (`studies/agent-harness-study/sources/agent-framework/go/README.md:1-3`), so no Go boundary mechanisms could be assessed.

---

Generated by `Dimension 17.01: Sandbox Boundary` against `agent-framework`.
