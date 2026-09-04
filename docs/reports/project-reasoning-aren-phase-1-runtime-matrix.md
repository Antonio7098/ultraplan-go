# Aren Phase 1 project-reasoning runtime matrix

This report covers every OpenCode agent session created while dogfooding the Aren Phase 1 project-reasoning lifecycle on 2026-09-04. All runs used `openrouter/minimax/minimax-m3:free` in `/home/antonioborgerees/coding/ultraplan/ultraplan-go-workspace`.

The figures come from OpenCode's `session` rows. Tool calls count `part` rows whose type is `tool`. Times are UTC. OpenRouter reported zero cost because the selected route was free.

## Summary

| Set | Runs | Input tokens | Output tokens | Cache-read tokens | Cache-write tokens | Tool calls | Reported cost |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| All attempts | 15 | 3,171,577 | 135,852 | 149,981 | 0 | 6 | $0.00 |
| Current accepted artifact producers | 9 | 1,008,341 | 119,115 | 140,088 | 0 | 6 | $0.00 |

The first four evidence-assessment attempts consumed 1,968,843 input tokens. The successful bounded attempt used 137,653, down 77.3% from the preceding 605,979-token attempt. Fourteen of the 15 sessions made no tool calls because UltraPlan injected their governed inputs directly. The initial index session made all six observed tool calls before the terminal-only stage contract was added.

## Per-run matrix

| Run | Started | Stage or area | Result | Seconds | Input | Output | Cache read | Cache write | Tools |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `f94074938f` | 10:30:56 | index | Current accepted index | 32 | 26,009 | 925 | 129,920 | 0 | 6 |
| `f9406a5fcf` | 10:31:38 | Evidence assessment | Failed: output file absent | 22 | 612,917 | 78 | 1,941 | 0 | 0 |
| `f94049055f` | 10:33:55 | Evidence assessment | Failed: required sections absent | 344 | 612,938 | 314 | 1,920 | 0 | 0 |
| `f93fe49ccf` | 10:40:46 | Evidence assessment | Failed: context overflow caused topic drift | 432 | 605,979 | 9,862 | 1,942 | 0 | 0 |
| `f93f483acf` | 10:51:27 | Evidence assessment | Current accepted area | 252 | 137,653 | 20,961 | 128 | 0 | 0 |
| `f93f090c7f` | 10:55:45 | Lifecycle authority | Current accepted area | 113 | 127,927 | 12,966 | 2,155 | 0 | 0 |
| `f93eebf23f` | 10:57:45 | Outcomes and terminal resolution | Current accepted area | 150 | 127,541 | 18,642 | 1,941 | 0 | 0 |
| `f93ec5d12f` | 11:00:21 | Cancellation and cleanup | Current accepted area | 176 | 129,215 | 15,539 | 128 | 0 | 0 |
| `f93e9931bf` | 11:03:24 | Events and observation | Current accepted area | 217 | 126,440 | 15,611 | 128 | 0 | 0 |
| `f93e62a13f` | 11:07:07 | Verification and Go correctness | Current accepted area | 207 | 125,467 | 21,199 | 1,792 | 0 | 0 |
| `f93e2e7dcf` | 11:10:41 | Final reasoning | Current accepted synthesis | 109 | 98,148 | 11,570 | 1,955 | 0 | 0 |
| `f93e125a9f` | 11:12:36 | Review | Superseded: non-actionable observations returned `pass_with_findings` | 29 | 111,642 | 2,389 | 128 | 0 | 0 |
| `f93df2971f` | 11:14:46 | Review | Superseded: stale candidate reuse reproduced | 55 | 109,920 | 2,324 | 1,920 | 0 | 0 |
| `f93dcbee5f` | 11:17:24 | Review | Failed: old candidate lacked actionable-finding count | 26 | 109,840 | 1,770 | 2,042 | 0 | 0 |
| `f93db58a4f` | 11:18:56 | Review | Current accepted review, `Actionable Findings: 0`, `Verdict: pass` | 19 | 109,941 | 1,702 | 1,941 | 0 | 0 |

## Notes

- Token counts are provider-reported usage, not estimates derived from prompt bytes.
- Cache-read tokens can exceed input tokens for a run because OpenRouter reports cached prefix usage separately.
- The accepted-producer row includes the current index, six areas, final synthesis, and final review.
- Superseded and failed runs remain in the all-attempt total because they were real paid or free provider executions and exposed the defects fixed by this campaign.
