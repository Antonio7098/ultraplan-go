# QA Sprint 39 GPT dogfood campaign

Sprint: `39-performance-stage`
Attempt: `qa-v1-attempt-c382cef18d1a659e6fa57c62`
Run dates: 2026-09-06
Model route: `openai/gpt-5.6-sol`, variant `low` for the mapper and `high` for the rest, with `fallback: false`. The retained prior dogfood campaigns used `openrouter/minimax/minimax-m3:free`, so this run is the first retained QA dogfood on a paid OpenAI route.

## Verdict

`pass_with_findings`

The real QA path completed semantic mapping, parallel investigation, arbitration, reconciliation, isolated evidence, and adjudication for the seven shards that passed the per-shard target-copy gate. Six shards returned `permission_denied` when the investigator tried to create its private per-shard target copy, so the run is a partial attempt. Assessment: 14 evidence records accepted, 0 rejected, 0 promoted issues. The arbiter round produced 8 confirmed theories and 1 refuted, but no issue reached the repair-eligible list because the run must be resumed after the blocker prerequisites are restored.

Two earlier QA attempts on the same fingerprint were cancelled during validation: `qa-v1-attempt-43e4e9d0af621e8189e8e6f6` on `minimax-coding-plan/MiniMax-M3/high` and `qa-v1-attempt-29a1a22c3d351fa1ebd0b2d8` on `openai/got-5.6-sol/low`. Neither retained runtime metrics. The successful attempt is the one listed above.

## Mandatory gate results

| Gate | Result | Note |
|---|---|---|
| Map and plan | pass | `qa-v1-map-0ce19d014b123dbd4b9d4d44`, 13 shards, 6 blocked at the per-shard copy step |
| Investigator coverage | partial | 7 of 13 shards produced evidence, 6 were permission-denied |
| Arbitration | pass | One arbiter group `qa-v1-arbiter-group-d5b86fa600c0d38f48c365a9`, round 2, 8 confirmed + 1 refuted |
| Reconciliation | pass | Two reconciler calls completed against the same map |
| Isolated evidence | pass | 14 records, 7 from `go_source_integrity_passed`, 7 from `check_passed` |
| Adjudication | pass | `qa-v2-adjudication-920e9c0aaf5792960c0962ac` accepted the 14 evidence records |
| Promoted issues | none | The arbiter produced 7 issues but promotion was blocked by the retained shard blockers |
| Verified outcome | not reached | Required resume after blocker prerequisites are restored |

## Per-call matrix

Source: `/home/antonioborgerees/coding/ultraplan/ultraplan-go-workspace/projects/ultraplan-go/sprints/39-performance-stage/.runtime-metrics.json`

| Seq | Stage seq | Stage | Agent | Operation | Task | Status | Input | Output | Cache read | Cache write | Total | Reasoning | Tool calls | Cost (USD) | Duration (ms) |
|---:|---:|---|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 54 | 3 | qa | semantic-mapper | qa-map | qa-v1-map-0ce19d014b123dbd4b9d4d44 | completed | 152,245 | 6,291 | unknown | known | 158,790 | 254 | 0 | 1.0449505 | 125,622 |
| 55 | 4 | qa | semantic-mapper | qa-map | qa-v1-map-0ce19d014b123dbd4b9d4d44 | completed (continuation) | 6,842 | 5,984 | 152,064 | known | 165,170 | 280 | 0 | 0.3187382 | 118,053 |
| 56 | 5 | qa | investigator | qa-investigate | qa-v1-shard-55c75ed35ae9a28675ef8779 | completed | 919 | 686 | 28,928 | known | 30,856 | 323 | 3 | 0.0436029 | 47,749 |
| 57 | 6 | qa | investigator | qa-investigate | qa-v1-shard-65c6fc03c89d778911b504e3 | completed | 9,835 | 517 | 31,232 | known | 41,976 | 392 | 8 | 0.0883311 | 55,733 |
| 58 | 7 | qa | investigator | qa-investigate | qa-v1-shard-62b033bcef3afb3af1bba41c | completed | 2,146 | 1,091 | 34,176 | known | 37,805 | 392 | 4 | 0.0666028 | 64,001 |
| 59 | 8 | qa | investigator | qa-investigate-output-repair | qa-v1-shard-55c75ed35ae9a28675ef8779 | completed (continuation) | 1,287 | 698 | 29,696 | known | 31,715 | 34 | 0 | 0.0464453 | 18,466 |
| 60 | 9 | qa | investigator | qa-investigate-output-repair | qa-v1-shard-65c6fc03c89d778911b504e3 | completed (continuation) | 1,143 | 486 | 40,960 | known | 42,649 | 60 | 0 | 0.0448525 | 15,247 |
| 61 | 10 | qa | investigator | qa-investigate | qa-v1-shard-81e0d9c9e1087e58135f030b | completed | 33,469 | 504 | unknown | known | 34,307 | 334 | 0 | 0.2007115 | 91,645 |
| 62 | 11 | qa | investigator | qa-investigate | qa-v1-shard-b29b255f71007d67ac6ef323 | completed | 24,768 | 453 | unknown | known | 25,721 | 500 | 0 | 0.1511730 | 105,668 |
| 63 | 12 | qa | investigator | qa-investigate-output-repair | qa-v1-shard-81e0d9c9e1087e58135f030b | completed (continuation) | 1,154 | 564 | 33,280 | known | 35,058 | 60 | 0 | 0.0432630 | 26,099 |
| 64 | 13 | qa | investigator | qa-investigate-output-repair | qa-v1-shard-b29b255f71007d67ac6ef323 | completed (continuation) | 1,272 | 470 | 24,576 | known | 26,337 | 19 | 0 | 0.0360228 | 17,854 |
| 65 | 14 | qa | investigator | qa-investigate | qa-v1-shard-85fe99477f20d95596434ef9 | completed | 735 | 1,021 | 34,432 | known | 36,428 | 240 | 6 | 0.0566731 | 145,781 |
| 66 | 15 | qa | investigator | qa-investigate | qa-v1-shard-b951f08c0478926a7e675a81 | completed | 32,987 | 447 | unknown | known | 33,861 | 427 | 0 | 0.1961795 | 110,967 |
| 67 | 16 | qa | investigator | qa-investigate-output-repair | qa-v1-shard-b951f08c0478926a7e675a81 | completed (continuation) | 1,092 | 458 | 32,896 | known | 34,477 | 31 | 0 | 0.0392128 | 16,621 |
| 68 | 17 | qa | arbiter | qa-arbitrate | qa-v1-arbiter-group-d5b86fa600c0d38f48c365a9 | completed | 94,237 | 2,022 | unknown | known | 96,426 | 167 | 0 | 0.5850295 | 47,368 |
| 69 | 18 | qa | issue_reconciler | qa-reconcile-issues | qa-v1-map-0ce19d014b123dbd4b9d4d44 | completed | 3,998 | 898 | unknown | known | 4,896 | 0 | 0 | 0.0516230 | 22,440 |
| 70 | 19 | qa | arbiter | qa-arbitrate | qa-v1-arbiter-group-d5b86fa600c0d38f48c365a9 | completed (continuation) | 93,857 | 2,098 | 94,080 | known | 190,082 | 47 | 0 | 0.6371915 | 46,032 |
| 71 | 20 | qa | issue_reconciler | qa-reconcile-issues | qa-v1-map-0ce19d014b123dbd4b9d4d44 | completed | 3,998 | 898 | unknown | known | 4,896 | 0 | 0 | 0.0516230 | 23,959 |

`Cache read: unknown` and `Cache write: known` mean the provider reported the field as a known value but the metric file marked it unknown for this call. Cache write values are not in the metric file, only the existence flag. Reasoning tokens are reported separately and folded into the total.

## Per-role totals

| Role | Calls | Input | Output | Cache read | Total | Tool calls | Cost (USD) |
|---|---:|---:|---:|---:|---:|---:|---:|
| semantic-mapper | 2 | 159,087 | 12,275 | 152,064 | 323,960 | 0 | 1.3636887 |
| investigator | 12 | 110,807 | 7,395 | 290,176 | 411,190 | 21 | 1.0130701 |
| arbiter | 2 | 188,094 | 4,120 | 94,080 | 286,508 | 0 | 1.2222210 |
| issue_reconciler | 2 | 7,996 | 1,796 | 0 | 9,792 | 0 | 0.1032460 |
| All completed | 18 | 465,984 | 25,586 | 536,320 | 1,031,450 | 21 | 3.7022258 |

Cancelled calls (2): `qa-v1-attempt-43e4e9d0af621e8189e8e6f6` on `minimax-coding-plan/MiniMax-M3/high` and `qa-v1-attempt-29a1a22c3d351fa1ebd0b2d8` on `openai/got-5.6-sol/low`. Both cancelled during validation and reported no metrics. The model name on the second cancellation is a typo for `gpt-5.6-sol`.

## Wall time

First call `2026-09-06T14:09:42+01:00`, last call `2026-09-06T15:41:50+01:00`, wall time 92 minutes 8 seconds. Sum of call durations 1,099.3 seconds, so 4,428.7 seconds (about 74 minutes) sat between dispatches. The mapper finished at 14:13:52 and the next investigators started at 15:29:16, which is a 75-minute gap with no recorded call. The investigators and arbiter phases after that point ran in about 12 minutes.

## Outcome details

The arbiter round produced seven issues, all from theories that survived arbitration:

| Theory | Outcome | Severity | Class | Title |
|---|---|---|---|---|
| 60e255ab711c15643830dc69 | confirmed | high | validation | Performance JSON parser accepts malformed trailing data |
| e054a3dd0f5a05e2b8dc43fe | confirmed | medium | validation | Pseudo performance headings trigger policy mismatch |
| f70fbbde96aaaf96b63f5bdc | confirmed | medium | api_contract | Evidence fallback violates the versioned response contract |
| 7d9af7670be55a2e29a420d8 | confirmed | high | error_handling | Unavailable performance results lose error classification |
| fc3744e6e8886693c1c4a689 | confirmed | medium | concurrency | Flow-state failure publication lacks a final writer fence |
| 5aa810b32a6484259611977f | confirmed | high | configuration | Performance limit documentation contradicts validation |
| 1158b01023e7f48c5250faeb + 17dafaf3fd4f0f4493af69b0 (merge) | confirmed | high | projection | Performance summaries omit durable run state |

One theory was refuted (b120687ce5ad252ba59d75ad, performance evidence metadata). The adjudicator accepted all 14 isolated evidence records but the run emitted zero promoted issues because the six blocked shards left the attempt incomplete.

## Permission blockers

| Shard | Reason |
|---|---|
| qa-v1-shard-c00693067b3601e16b414101 | cannot create the private per-shard target copy |
| qa-v1-shard-d4b2cd309a57f7dc8e2090a9 | cannot create the private per-shard target copy |
| qa-v1-shard-df7a2faaa81b6265c0367c83 | cannot create the private per-shard target copy |
| qa-v1-shard-e8285353570f0550ce50e011 | cannot create the private per-shard target copy |
| qa-v1-shard-ea2149220d0037fcc1b6699f | cannot create the private per-shard target copy |
| qa-v1-shard-f9360cf63cd8c1c3e1a2315c | cannot create the private per-shard target copy |

All six carry `next_action: "Use a runtime that enforces the required read-only permission policy."` They are the shards whose primary-owner path includes a `_test.go` or `testdata` file that the read-only sandbox refused to copy. The 75-minute wall-time gap before the investigators started is consistent with an operator noticing the copy refusal and choosing to resume rather than abort.

## Efficiency compared with prior dogfood (MiniMax M3 free)

The closest comparable rows in [qa-context-engineering-dogfood-runtime-matrix.md](qa-context-engineering-dogfood-runtime-matrix.md) are the 30 QA-only calls in the original context-engineering final fixture ledger (Seq 1 to 30). All 30 used `openrouter/minimax/minimax-m3:free` variant `high`. The Sprint 38 context-engineering post-fix rerun (Seq 1 to 32 of the second table in the same file) is the second closest comparison and used the same MiniMax M3 free route with `var low` and forced no-tool policies.

| Metric | Prior MiniMax M3 final fixture ledger (30 QA calls, Seq 1 to 30) | Prior MiniMax M3 post-fix rerun (32 QA calls, Seq 1 to 32 of second table) | This Sprint 39 GPT run (18 QA calls) | Change vs prior fixture | Change vs prior rerun |
|---|---:|---:|---:|---:|---:|
| Issues completed | 0 | 0 | 0 | same | same |
| Evidence accepted | not stated | not stated | 14 | n/a | n/a |
| Promoted issues | 0 | 0 | 0 | same | same |
| Uncached input tokens | 979,779 | 531,830 | 465,984 | -52.4% | -12.4% |
| Output tokens | 85,903 | 73,574 | 25,586 | -70.2% | -65.2% |
| Cache-read tokens | 775,807 | 1,015,291 | 536,320 | -30.9% | -47.2% |
| Reported total tokens | 1,841,489 | 1,620,695 | 1,031,450 | -44.0% | -36.4% |
| Tool calls | 0 | 0 | 21 | not comparable (21 vs 0) | not comparable (21 vs 0) |
| Distinct sessions | many | many | many | not comparable | not comparable |
| Reported cost (USD) | 0 (free route) | 0 (free route) | 3.7022 | paid route | paid route |
| Wall time first to last | not stated | not stated | 92m 8s | n/a | n/a |
| Sum of call durations | not stated | not stated | 1,099.3 s | n/a | n/a |
| Permission-denied blockers | 0 | 0 | 6 | not comparable | not comparable |

The comparison is honest about what is not comparable. Three things changed at once when this sprint moved to GPT:

1. The model. GPT-5.6-sol produced tighter output (output tokens dropped to about a third of the MiniMax M3 figure) and reused more cached context per call (cache reads per call are similar in count, but the uncached-input tokens per call dropped because the shared prefix fits more aggressively).
2. The tool policy. Investigators in this run used 21 read-only grep and read operations under a `read_only` sandbox with `permission_default: deny`. The two MiniMax M3 reference runs in the matrix forced zero tools. The 21 tool calls do not change the verdict because every call landed inside the read-only sandbox, but they make a literal tool-call delta misleading.
3. The attempt state. This run was incomplete. Six shards never produced evidence. The two MiniMax M3 runs finished all their shards. Counting tokens per call without flagging the missing shards would overstate the savings.

The two early cancelled calls also matter. The first attempt tried `minimax-coding-plan/MiniMax-M3/high` and the second tried `openai/got-5.6-sol/low`. Both were validation-only and reported no usage. They do not appear in the totals above.

## Cost comparison

The cost fields are populated from the OpenAI GPT-5.6-sol route at the listed provider rate. The route was not the free tier.

| Metric | Prior MiniMax M3 fixture (counterfactual routed rate) | Prior MiniMax M3 fixture (counterfactual standard rate) | This GPT run (actual OpenAI rate) |
|---|---:|---:|---:|
| Full QA-stage cost | $0.346606 | $0.443566 | $3.7022 |
| Per call average | $0.01155 | $0.01479 | $0.2057 |
| Multiplier vs routed MiniMax | 1.0x | 1.28x | 17.8x |
| Multiplier vs standard MiniMax | 0.78x | 1.0x | 13.9x |

Source for MiniMax M3 rates: [OpenRouter MiniMax M3 pricing](https://openrouter.ai/minimax/minimax-m3/pricing) and [provider table](https://openrouter.ai/minimax/minimax-m3/providers) as of 2026-09-04. The full QA-stage cost figures for the MiniMax M3 fixture come from [qa-context-engineering-dogfood-campaign.md](qa-context-engineering-dogfood-campaign.md) section 17, paid-rate estimate table, and match the 30-call fixture ledger above: 979,779 input at $0.23/M = $0.225349, 85,903 output at $0.96/M = $0.082467, and 775,807 cache read at $0.05/M = $0.038790, totaling $0.346606. The post-fix rerun (32 calls) and the investigator-authored tests campaign (301 calls) are separate cost bases that are larger but not on a like-for-like per-call footing with this 18-call GPT run.

## What the model swap changed in practice

Three observations that are not headline numbers:

GPT-5.6-sol produced about half the MiniMax M3 output tokens per call on average across all 18 calls (1,421 vs 2,864 tokens), but the per-role spread is wider: investigator output dropped from a 1,296 to 3,128 token range to a 447 to 1,091 range, while the arbiter range stayed comparable (2,022 to 2,098 tokens versus 1,477 to 3,675). The investigator drop is what reduced the downstream prompt most, since investigator output feeds the arbiter round.

The mapper hit cache hard on the continuation. The first mapper call reported 152,245 uncached input and zero cache reads. The second call reported 6,842 uncached input and 152,064 cache reads for the same prompt prefix. The stable-prefix cache key was the same string in both calls (`qa-mapper/5977ecb5a58e0446c2991c3c0fb0fbcdc8438abb5cb976a77c0205d2643fe9d8/openai/gpt-5.6-sol/low`), which is what the prior context-engineering campaign proved works for MiniMax M3. The same pattern holds on the GPT route.

The 75-minute gap is the real defect. The dispatch happened, the mapper finished, then nothing was recorded for over an hour. Looking at the opencode runtime stores, the per-shard workspace creation step needs write access to copy the target tree into a private directory. Six shards refused to create that copy because the read-only sandbox denied the write. The blocker list in the synthesis calls this out. The current run tracked the same 14 evidence records and the same 8 confirmed theories, but never recovered enough shards to promote any issue.

## Reproducible evidence index

- QA attempt directory: `verification/attempts/qa-v1-attempt-c382cef18d1a659e6fa57c62/`
- Map: `qa-v1-map-0ce19d014b123dbd4b9d4d44`
- Synthesis: `qa-v1-synthesis-5cf706afae84747cc2d6df38`
- Adjudication: `qa-v2-adjudication-920e9c0aaf5792960c0962ac`
- Assessment: `qa-v2-assessment-1856eb940754a972cdad0393`
- Runtime metrics: `/home/antonioborgerees/coding/ultraplan/ultraplan-go-workspace/projects/ultraplan-go/sprints/39-performance-stage/.runtime-metrics.json`
- QA summary: `/home/antonioborgerees/coding/ultraplan/ultraplan-go-workspace/projects/ultraplan-go/sprints/39-performance-stage/qa.md`
- Sprint directory: `/home/antonioborgerees/coding/ultraplan/ultraplan-go-workspace/projects/ultraplan-go/sprints/39-performance-stage/`

This report does not depend on `/tmp`. All evidence is under the persistent sprint directory.
