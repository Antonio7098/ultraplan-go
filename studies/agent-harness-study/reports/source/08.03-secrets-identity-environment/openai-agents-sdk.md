# Source Analysis: openai-agents-sdk

## 08.03 — Secrets, Identity, and Environment Handling

### Source Info

| Field | Value |
|-------|-------|
| Name | openai-agents-sdk |
| Path | `studies/agent-harness-study/sources/openai-agents-sdk` |
| Language / Stack | Python (pydantic, httpx, OpenAI Python SDK; Docker/Modal/Runloop/Vercel sandbox backends) |
| Analyzed | 2026-08-24 |

## Summary

The SDK has no unified secret store. Credentials arrive through three channels: host environment variables (`OPENAI_API_KEY` read by the model client and trace exporter, `src/agents/models/openai_provider.py:157-170`, `src/agents/tracing/processors.py:106-116`), explicit programmatic key/client setters (`src/agents/__init__.py:278-303`, `src/agents/_config.py:13-24`), and inline credentials on sandbox manifest mount entries (`src/agents/sandbox/entries/mounts/providers/s3.py:24-26`). An `EnvValue` extension point exists for resolving manifest environment values from arbitrary stores, but a built-in secret-store implementation is still a TODO (`src/agents/sandbox/manifest.py:84-110`).

What the SDK does have is an unusually strong *exposure-control* layer. Logging of model/tool payloads is redacted by default (`src/agents/_debug.py:12-27`); errors are rewritten to drop payload-bearing exception chains and traceback frames (`src/agents/exceptions.py:33-56`, `src/agents/run_internal/turn_resolution.py:436-478`); the tracing API key is never serialized by default — only a SHA-256 fingerprint is persisted so resumed runs can verify a re-supplied key (`src/agents/tracing/traces.py:171-192`, `src/agents/tracing/context.py:37-44`); and a ~2,300-line mount-security module plus 132 dedicated tests strip cloud-mount authority ("credentials") from durable state and require an exact-match trusted-manifest rebind on resume (`src/agents/sandbox/_mount_security.py:60-161`, `1723-1813`; `tests/sandbox/test_mount_security.py`).

The main weakness sits in the default for traces themselves: `trace_include_sensitive_data` defaults to **True** via `OPENAI_AGENTS_TRACE_INCLUDE_SENSITIVE_DATA` (`src/agents/run_config.py:53-56`, documented at `docs/tracing.md:146`). So while logs are safe by default, exported traces carry full LLM/tool inputs and outputs unless the application opts out — meaning a freshly captured trace cannot be shared without leaking risk until that flag is set. Environment isolation between runs is strong for the Docker backend (container env comes only from the manifest, `src/agents/sandbox/sandboxes/docker.py:1736-1744`) but weak for the Unix-local backend, which copies the entire host environment into every command (`src/agents/sandbox/sandboxes/unix_local.py:442-451`).

## Rating

**7 / 10** — Clear model with extensive tests, explicit interfaces, and real operational safeguards for log/error/state redaction and mount-authority hygiene; kept out of the top band because trace payloads include sensitive data by default, there is no built-in secret provider (extension point only), identity scoping is minimal, and the local-sandbox backend inherits the full host environment.

## Evidence Collected

Every entry cites a file path with line numbers relative to the source root.

| Area | Evidence | File:Line |
|------|----------|-----------|
| Env config (API key) | Model client lazily built from stored key or `OPENAI_API_KEY`; base URLs from `OPENAI_BASE_URL`/`OPENAI_WEBSOCKET_BASE_URL` | `src/agents/models/openai_provider.py:136-176` |
| Env config (setter APIs) | `set_default_openai_key(key, use_for_tracing=True)` also feeds the trace exporter | `src/agents/__init__.py:278-290`, `src/agents/_config.py:13-17` |
| Env config (trace exporter) | Exporter falls back to `OPENAI_API_KEY`, `OPENAI_ORG_ID`, `OPENAI_PROJECT_ID` | `src/agents/tracing/processors.py:69-116` |
| Secret storage (none built-in) | `EnvValue` extension point with `resolve()` coroutine "can reach a secret store"; TODO comment "env val from secret store" | `src/agents/sandbox/manifest.py:84-110`, `235-243` |
| Secret storage (mount credentials) | `S3Mount` carries `access_key_id`/`secret_access_key`/`session_token` inline; rendered as plaintext rclone config lines | `src/agents/sandbox/entries/mounts/providers/s3.py:21-31`, `125-130` |
| Secret storage (provider secrets) | Modal named secrets via `modal.Secret.from_name`/`from_dict` for cloud bucket mounts | `src/agents/extensions/sandbox/modal/mounts.py:23-31`, `src/agents/extensions/sandbox/modal/sandbox.py:2018-2030` |
| Secret storage (managed platform secrets) | Runloop `managed_secrets`: create/update/list/delete against account secret store; attach by ref to devbox | `src/agents/extensions/sandbox/runloop/sandbox.py:413-429`, `488`, `1495-1515`, `620-632` |
| Secret injection (hosted tools) | `ShellToolContainerNetworkPolicyDomainSecret`: value bound to one allowlisted domain of a hosted container | `src/agents/tool.py:1219-1232` |
| Secret injection (sandbox env) | Docker container env resolved only from manifest; unix-local copies full host env then overlays manifest env | `src/agents/sandbox/sandboxes/docker.py:1736-1744`, `src/agents/sandbox/sandboxes/unix_local.py:442-451` |
| Log redaction defaults | `DONT_LOG_MODEL_DATA`/`DONT_LOG_TOOL_DATA` default True via `OPENAI_AGENTS_DONT_LOG_*` flags | `src/agents/_debug.py:12-27` |
| Log redaction enforcement | Console exporter prints "data is redacted"; exporter HTTP error bodies gated; collision warnings gated | `src/agents/tracing/processors.py:30-41`, `186-197`, `src/agents/_tool_identity.py:531-537`, `src/agents/mcp/util.py:825-831` |
| Error-chain redaction | `_mark_error_data_redacted`, traceback detachment, detached re-raise boundary; handoff input replaced with `<redacted>` | `src/agents/exceptions.py:33-212`, `src/agents/handoffs/__init__.py:49-67`, `src/agents/run_internal/turn_resolution.py:436-478` |
| Trace sensitive-data switch | `RunConfig.trace_include_sensitive_data` default factory reads env var, default `"true"` | `src/agents/run_config.py:53-56`, `404-410` |
| Trace gating (spans) | Function-span input/output set only when flag on; tool error text swapped for `"Error details are redacted."` | `src/agents/run_internal/tool_execution.py:1818-1819`, `1840-1855`, `src/agents/util/_error_tracing.py:11`, `46-53` |
| Trace gating (models) | `ModelTracing.ENABLED_WITHOUT_DATA` when flag off; generation spans omit input/output | `src/agents/tracing/model_tracing.py:6-14`, `src/agents/models/openai_responses.py:605-621` |
| Trace gating (MCP output) | MCP tool output attached to span only when flag on; span keeps only server name | `src/agents/mcp/util.py:814-823` |
| Tracing key hygiene | `to_json(include_tracing_api_key=False)` default "to avoid persisting secrets unintentionally"; SHA-256 fingerprint only; resume verifies hash match | `src/agents/tracing/traces.py:171-192`, `245-277`, `src/agents/tracing/context.py:37-44`, `.agents/references/tracing-lifecycle.md:18` |
| RunState opt-in raw key | `RunState.to_json(include_tracing_api_key=False)` parameter threading | `src/agents/run_state.py:1709-1719`, `2047-2073` |
| Mount authority redaction | Authority-field tables per mount type; opaque strategy fields; URL inline-authority checks; rclone line-injection rejection | `src/agents/sandbox/_mount_security.py:54-161`, `1874-1905`, `1922-2029` |
| Durable-state sanitizer | `sanitize_manifest_mount_authority` returns credential-free manifest + `redacted` flag; session-state serializer always sanitizes and sets marker | `src/agents/sandbox/_mount_security.py:1723-1748`, `2121+`, `src/agents/sandbox/session/sandbox_session_state.py:349-399` |
| Rebind protocol | Resume requires current trusted manifest with exactly matching credential-free topology before restoring authority | `src/agents/sandbox/_mount_security.py:1751-1813`, `src/agents/sandbox/session/sandbox_session_state.py:300-347` |
| Credential-exposure acknowledgement | Runtime-only policy (never serialized); manifest input containing policy keys rejected | `src/agents/sandbox/manifest.py:55-78`, `261-271`, `296-351` |
| Per-server auth identity | MCP servers accept per-server `headers`/httpx auth; guidance to keep tokens in headers not URLs | `src/agents/mcp/server.py:306-324`, `1980-1981`, `docs/mcp.md:14` |
| Sandbox users/groups | Manifest-level POSIX-style `User`/`Group`/`Permissions` for in-sandbox file ownership | `src/agents/sandbox/types.py:9-55`, `src/agents/sandbox/manifest.py:250-253` |
| Context not visible to model | `RunContextWrapper.context` explicitly "not passed to the LLM"; app-supplied dependency channel | `src/agents/run_context.py:72-81` |
| Tests: log redaction | ~20 tests covering error/warning redaction under both data policies | `tests/test_error_logging_redaction.py:212-753` |
| Tests: mount security | 132 tests: acknowledgement scoping, provenance forgery, serialization redaction, rebind topology, injection rejection | `tests/sandbox/test_mount_security.py` (test list at lines incl. `def test_rejects_explicit_credentials_for_in_container_mounts`, `test_session_state_serialization_redacts_complete_opaque_authority_fields`) |
| Docs: behavior contract | Sensitive-data sections documenting both flags and their risks | `docs/config.md:179-197`, `230-248`, `docs/tracing.md:140-146`, `docs/sandbox/guide.md:679` |

## Answers to Dimension Questions

**1. Can the model see secrets?**
Not directly by design, but yes indirectly in practice. The run context object is documented as never sent to the LLM (`src/agents/run_context.py:76-78`), so app-injected secrets in context stay out of prompt input. However, anything a tool *returns* flows back as model input, and the Unix-local shell executes with a copy of the full host environment (`src/agents/sandbox/sandboxes/unix_local.py:443`), so a command like `env` or `cat ~/.aws/credentials` surfaces host secrets straight into conversation history. The Docker backend confines env to manifest-declared values (`src/agents/sandbox/sandboxes/docker.py:1736-1744`), and hosted-shell domain secrets are injected server-side per allowlisted domain (`src/agents/tool.py:1219-1232`).

**2. Can tools use secrets without exposing them?**
Partially. Three mechanisms avoid handing secrets to the model: (a) context-based dependency injection (`src/agents/run_context.py:76-81`); (b) external/provider-native mount strategies where the credential stays with the volume driver rather than in-container files, which the SDK prefers and enforces — in-container credentialed mounts are rejected unless trusted code acknowledges exposure (`src/agents/sandbox/manifest.py:296-323`, `docs/sandbox/clients.md:151-165`); (c) platform-managed secrets (Modal named secrets, Runloop managed secrets referenced by name, `src/agents/extensions/sandbox/runloop/sandbox.py:425-430`). But inline mount credentials are rendered as plaintext rclone config lines inside the sandbox when used (`src/agents/sandbox/entries/mounts/providers/s3.py:125-130`), and the docs concede acknowledged exposure means the helper "receives credentials without confining credential use to the mounted path" (`docs/sandbox/clients.md:165`).

**3. Are secrets redacted in traces?**
Only conditionally. Span *payload* fields (tool/model inputs and outputs, MCP outputs) are omitted when `trace_include_sensitive_data=False` (`src/agents/run_internal/tool_execution.py:1818-1855`, `src/agents/mcp/util.py:816-820`, `src/agents/tracing/model_tracing.py:12-13`), but the default is True via env fallback (`src/agents/run_config.py:53-56`). Error strings attached to spans swap to `"Error details are redacted."` when the flag is off (`src/agents/util/_error_tracing.py:46-53`), and the framework separately guarantees exception objects/chains don't retain payloads regardless of display (`src/agents/exceptions.py:140-212`, `.agents/references/tracing-lifecycle.md:31-32`). The tracing export API key itself is well protected: excluded from serialized trace JSON by default and stored only as a SHA-256 fingerprint (`src/agents/tracing/traces.py:171-192`). Answering the dimension's core question — *can a trace be shared without leaking credentials?* — **not by default**: sharing requires opting out via `RunConfig(trace_include_sensitive_data=False)` or the env var, and even then the reference doc warns the flag "does not automatically sanitize exception objects, chaining, tracebacks, logs, or telemetry created elsewhere" (`.agents/references/tracing-lifecycle.md:31`).

**4. Are identities scoped per user/task?**
Minimally. There is no first-class user/task identity concept; `TContext` is application-defined. What exists: per-trace tracing API keys grouped at export time so different traces can authenticate differently (`src/agents/tracing/processors.py:124-131`, `.agents/references/tracing-lifecycle.md:34`); per-server MCP auth headers (`src/agents/mcp/server.py:306-324`); per-domain secrets for hosted shells (`src/agents/tool.py:1219-1232`); POSIX-style users/groups/permissions inside sandboxes (`src/agents/sandbox/types.py:9-55`); and approval records keyed per tool call within a run (`src/agents/run_context.py:89-94`). No evidence found of built-in workload-identity federation, per-run credential scoping, or automatic short-lived credential minting — searches across `src/agents` for identity/sts/federation concepts returned only the mount-authority tables above.

## Architectural Decisions

1. **Redact at the boundaries, not at the source.** The SDK does not wrap or proxy secret values (no secret handle type exists). Instead it controls the four egress points — logs, exception chains/tracebacks, span payloads, and serialized state — each behind explicit flags or unconditional sanitizers (`src/agents/_debug.py:20-27`, `src/agents/exceptions.py:33-56`, `src/agents/run_config.py:404-410`, `src/agents/sandbox/session/sandbox_session_state.py:349-399`).

2. **Durable state never grants authority.** Session-state and `RunState` serialization strips mount credentials and sets `REDACTED_MOUNT_AUTHORITY_KEY`; resume requires re-binding from a *current* trusted manifest whose credential-free topology matches exactly (`src/agents/sandbox/_mount_security.py:1751-1813`, `docs/sandbox/guide.md:679`). A stolen serialized run can be inspected but cannot resurrect credentials.

3. **Explicit human acknowledgment for credential exposure.** In-container credentialed mounts fail closed unless trusted application code calls `with_in_container_mount_credential_exposure_acknowledged()` for exact mount paths; the acknowledgement is runtime-only, unserializable, and rejected if supplied via manifest input (`src/agents/sandbox/manifest.py:261-271`, `296-351`).

4. **Fail-safe defaults differ by channel.** Logs default to redaction ON (`_debug.py` defaults True); trace payloads default to inclusion OFF... i.e., sensitivity included by default. This asymmetry is deliberate but creates the sharpest safety cliff in the design (`src/agents/run_config.py:53-56` vs `src/agents/_debug.py:12-17`).

5. **Secret storage delegated to platforms and extension points.** Rather than shipping a secret provider, the SDK exposes `EnvValue.resolve()` coroutines (explicitly noted as able to reach a secret store) and lets backend extensions map to native secret services (`src/agents/sandbox/manifest.py:235-243`, `src/agents/extensions/sandbox/modal/sandbox.py:2018-2030`).

## Notable Patterns

- **Fingerprint-not-secret persistence:** SHA-256 hashing of the tracing API key with hash-match verification on resume, so stripped snapshots prove key identity without retaining the key (`src/agents/tracing/traces.py:187-192`, `src/agents/tracing/context.py:40-43`).
- **Error-object surgery over message masking:** redaction detaches tracebacks, clears `__cause__`/`__context__`, rebuilds replacement exceptions frame-by-frame, and deletes payload-bearing locals before raising — because "`raise ... from None` changes display, not object retention" (`src/agents/exceptions.py:140-178`, `src/agents/sandbox/_mount_security.py:487-548`, `.agents/references/tracing-lifecycle.md:32`).
- **Authority-vs-secret classification:** the mount tables distinguish plain secrets (`secret_access_key`) from *authority* fields that grant access without being secret values (e.g., Azure `identity_client_id`, Box config files), redacting both (`src/agents/sandbox/_mount_security.py:57-107`).
- **Provenance checks against forged trust:** custom mounts/subclasses cannot self-declare trusted credential boundaries; builtin-class provenance is validated before deepcopy or side effects (`tests/sandbox/test_mount_security.py::test_custom_mount_cannot_self_declare_a_trusted_credential_boundary`, `test_builtin_mount_subclass_is_rejected_by_execution_provenance`).
- **Injection defense on free-form config:** rclone config values are checked for line breaks and s3fs option delimiters so a bucket name cannot smuggle extra configuration directives (`src/agents/sandbox/_mount_security.py:112-145`, `1961-1995`).

## Tradeoffs

- **Default-on trace verbosity vs shareability.** Including sensitive data by default maximizes debuggability for new users but makes "just share this trace" dangerous; mitigation is entirely opt-in (`src/agents/run_config.py:53-56`, `docs/tracing.md:146`).
- **Inline mount credentials vs operational simplicity.** Plaintext credentials in manifests enable simple cross-backend setups but create plaintext-at-rest exposure in user code and (for in-container strategies) inside the sandbox; the counterweight is the acknowledgement gate and preference for external strategies (`docs/sandbox/clients.md:151-167`).
- **Local-sandbox convenience vs isolation.** Copying the whole host env (`unix_local.py:443`) maximizes tool compatibility (PATH, proxies, tokens) but collapses the host/agent secret boundary.
- **No secret abstraction vs zero lock-in.** Delegating to `EnvValue` subclasses/platform secrets avoids building a vault integration prematurely, but leaves every application to roll its own injection discipline, and the core TODO remains open (`src/agents/sandbox/manifest.py:84`).
- **Strictness vs extensibility in mount security.** Closed allowlists of known strategies and rejection of unknown discriminators make the sanitizer robust but mean third-party mounts need SDK-side registration to survive serialization safely (`src/agents/sandbox/_mount_security.py:158-180`, `2001-2010`).

## Failure Modes / Edge Cases

- **Trace leak by default:** a team that never touches `trace_include_sensitive_data` exports full prompts/tool outputs — including any secret echoed by a tool — to the configured processor/exporter.
- **Host-env bleed-through in unix-local sandbox:** any process-level secret is one `env` call away from the transcript; nothing warns at configuration time.
- **Serialization-failure path must also stay clean:** if the manifest itself fails to dump, the SDK raises a *redacted* serialization error instead of the original (payload-bearing) one, and scrubs traceback locals — a subtle case covered by tests (`src/agents/sandbox/_mount_security.py:1734-1744`, `test_serialization_failure_redacts_mount_authority_from_sdk_traceback_frames`).
- **Partial credential sets:** acknowledgements validate completeness (e.g., S3 requires access key + secret; empty scalars rejected) so a half-configured mount cannot silently fall back to ambient IAM (`tests/sandbox/test_mount_security.py::test_acknowledgement_rejects_incomplete_in_container_s3_credentials`, `test_s3_files_require_broad_acknowledgement_before_ambient_iam_can_be_used`).
- **Resume with mismatched trusted manifest fails closed** before sandbox start rather than partially rebinding (`src/agents/sandbox/session/sandbox_session_state.py:308-326`, `test_mount_authority_rebind_requires_exact_credential_free_topology`).
- **GeneratorExit during trace unwind** is tolerated without crashing but leaves reset ownership unresolved — documented as unfixable from that context (`src/agents/tracing/traces.py:18-48`).

## Future Considerations

- Ship a built-in secret-store `EnvValue` (the TODO at `src/agents/sandbox/manifest.py:84`) so manifests can reference secrets symbolically instead of carrying inline values.
- Flip or harden the default of `trace_include_sensitive_data` (e.g., warn loudly when exporting sensitive spans, or add scrubbing middleware to processors).
- Add an env-allowlist mode for the unix-local sandbox so host-environment inheritance is explicit rather than total.
- Introduce optional per-task/per-user identity propagation (e.g., a typed identity field on run options feeding trace metadata and export routing) to satisfy least-privilege auditing.
- Extend domain-scoped secret binding (currently hosted-shell-only, `src/agents/tool.py:1219-1224`) to local/sandboxed shell execution.

## Questions / Gaps

- No evidence found of automatic secret scanning in CI workflows (`.github/workflows/` contains docs/publish/release/tests pipelines only; no secret-scanning or gitleaks configuration located). Search boundary: `.github/workflows/*.yml` filenames and SECURITY.md.
- No evidence found of TTL/rotation handling for inline mount credentials — expiry is left to the underlying provider strategy; nothing in `src/agents/sandbox/` schedules refreshes.
- The interaction between `call_model_input_filter` (`src/agents/run_config.py:438-446`) and secret leakage was not analyzed exhaustively: a filter could deliberately inject secrets into model input with no SDK-side guard. This appears to be trusted-application territory by design, but no documentation states it.
- Voice pipeline audio-data tracing has its own flag (`VoicePipelineConfig.trace_include_sensitive_audio_data`, `docs/voice/tracing.md:10-11`) which also defaults to including data; whether realtime audio transcripts can contain secrets was not traced further.

---

Generated by dimension 08.03 (Secrets, Identity, and Environment Handling) against `openai-agents-sdk`.
