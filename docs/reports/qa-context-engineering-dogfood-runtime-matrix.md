# QA context-engineering runtime matrix

Source: `/home/antonioborgerees/coding/.ultraplan-qa-context-dogfood-20260831/repair-fixture-workspace/projects/ultraplan-go/sprints/38-bounded-repair/.runtime-metrics.json`

This matrix contains every retained QA and repair agent call in the final dogfood fixture ledger. Review, planning, smoke, and merge agents are excluded. `unknown` means the provider or failed runtime did not report the metric. A displayed `0` is a known zero. `Exact` states whether the retained tool-call count carries an explicit exactness marker.

| Seq | Stage | Agent | Operation | Task | Status | Input | Output | Cache read | Cache write | Total | Tool calls | Exact |
|---:|---|---|---|---|---|---:|---:|---:|---:|---:|---:|:---:|
| 1 | qa | semantic-mapper | qa-map | qa-v1-map-1521f43179b8db56d82f4e9e | completed | 106335 | 15290 | 1956 | 0 | 123581 | 0 | yes |
| 2 | qa | semantic-mapper | qa-map | qa-v1-map-3fd27eb5af3a296cfbc0bf7d | completed | 106375 | 2359 | 1956 | 0 | 110690 | 0 | yes |
| 3 | qa | investigator | qa-investigate | qa-v1-shard-0f52866c2f241c333c504353 | completed | 18125 | 2255 | 128 | 0 | 20508 | 0 | yes |
| 4 | qa | investigator | qa-investigate | qa-v1-shard-5c9606e6b0dc26218dc47ee6 | completed | 42621 | 1296 | 1924 | 0 | 45841 | 0 | yes |
| 5 | qa | investigator | qa-investigate-output-repair | qa-v1-shard-5c9606e6b0dc26218dc47ee6 | completed | 42762 | 1417 | 2042 | 0 | 46221 | 0 | yes |
| 6 | qa | investigator | qa-investigate-context-continuation | qa-v1-shard-0f52866c2f241c333c504353 | completed | 20514 | 2351 | 2042 | 0 | 24907 | 0 | yes |
| 7 | qa | investigator | qa-investigate-context-continuation | qa-v1-shard-5c9606e6b0dc26218dc47ee6 | completed | 6830 | 1454 | 44804 | 0 | 53088 | 0 | yes |
| 8 | qa | investigator | qa-investigate | qa-v1-shard-478c4c16fc20b78f1abb25b6 | completed | 33671 | 2210 | 1910 | 0 | 37791 | 0 | yes |
| 9 | qa | investigator | qa-investigate | qa-v1-shard-d80efa72c8ec0193d74274e1 | completed | 27398 | 3128 | 1958 | 0 | 32484 | 0 | yes |
| 10 | qa | investigator | qa-investigate-context-continuation | qa-v1-shard-478c4c16fc20b78f1abb25b6 | completed | 40206 | 2195 | 2042 | 0 | 44443 | 0 | yes |
| 11 | qa | investigator | qa-investigate | qa-v1-shard-cf1174e47aa6c2b65d6cc84b | completed | 21382 | 2839 | 1955 | 0 | 26176 | 0 | yes |
| 12 | qa | investigator | qa-investigate-context-continuation | qa-v1-shard-d80efa72c8ec0193d74274e1 | completed | 31985 | 2668 | 2042 | 0 | 36695 | 0 | yes |
| 13 | qa | investigator | qa-investigate-context-continuation | qa-v1-shard-cf1174e47aa6c2b65d6cc84b | completed | 28580 | 2355 | 2042 | 0 | 32977 | 0 | yes |
| 14 | qa | arbiter | qa-arbitrate | qa-v1-arbiter-group-e7de8cc8d8d83b251d990e47 | completed | 65396 | 3674 | 1956 | 0 | 71026 | 0 | yes |
| 15 | qa | arbiter | qa-arbitrate | qa-v1-arbiter-group-e7de8cc8d8d83b251d990e47 | completed | 3712 | 3675 | 67352 | 0 | 74739 | 0 | yes |
| 16 | qa | semantic-mapper | qa-map | qa-v1-map-3fd27eb5af3a296cfbc0bf7d | completed | 1 | 4084 | 108330 | 0 | 112415 | 0 | yes |
| 17 | qa | arbiter | qa-arbitrate | qa-v1-arbiter-group-e7de8cc8d8d83b251d990e47 | completed | 65394 | 2016 | 1958 | 0 | 69368 | 0 | yes |
| 18 | qa | arbiter | qa-arbitrate | qa-v1-arbiter-group-e7de8cc8d8d83b251d990e47 | completed | 2054 | 2008 | 67352 | 0 | 71414 | 0 | yes |
| 19 | qa | arbiter | qa-arbitrate | qa-v1-arbiter-group-e7de8cc8d8d83b251d990e47 | completed | 65515 | 1477 | 1920 | 0 | 68912 | 0 | yes |
| 20 | qa | arbiter | qa-arbitrate | qa-v1-arbiter-group-e7de8cc8d8d83b251d990e47 | completed | 1535 | 2303 | 67435 | 0 | 71273 | 0 | yes |
| 21 | qa | arbiter | qa-arbitrate | qa-v1-arbiter-group-e7de8cc8d8d83b251d990e47 | completed | 65561 | 2063 | 1910 | 0 | 69534 | 0 | yes |
| 22 | qa | arbiter | qa-arbitrate | qa-v1-arbiter-group-e7de8cc8d8d83b251d990e47 | completed | 2116 | 2047 | 67471 | 0 | 71634 | 0 | yes |
| 23 | qa | arbiter | qa-arbitrate | qa-v1-arbiter-group-e7de8cc8d8d83b251d990e47 | completed | 68709 | 3268 | 128 | 0 | 72105 | 0 | yes |
| 24 | qa | arbiter | qa-arbitrate | qa-v1-arbiter-group-e7de8cc8d8d83b251d990e47 | completed | 3326 | 3304 | 68837 | 0 | 75467 | 0 | yes |
| 25 | qa | arbiter | qa-arbitrate | qa-v1-arbiter-group-e7de8cc8d8d83b251d990e47 | completed | 1 | 2711 | 68836 | 0 | 71548 | 0 | yes |
| 26 | qa | issue_reconciler | qa-reconcile-issues | qa-v1-map-3fd27eb5af3a296cfbc0bf7d | completed | 751 | 461 | 1956 | 0 | 3168 | 0 | yes |
| 27 | qa | arbiter | qa-arbitrate | qa-v1-arbiter-group-e7de8cc8d8d83b251d990e47 | completed | 1 | 1968 | 68836 | 0 | 70805 | 0 | yes |
| 28 | qa | issue_reconciler | qa-reconcile-issues | qa-v1-map-3fd27eb5af3a296cfbc0bf7d | completed | 2473 | 358 | 128 | 0 | 2959 | 0 | yes |
| 29 | qa | semantic-mapper | qa-map | qa-v1-map-4f35f3e91842eba59f91d6bc | completed | 106347 | 4383 | 1943 | 0 | 112673 | 0 | yes |
| 30 | qa | semantic-mapper | qa-map | qa-v1-map-4f35f3e91842eba59f91d6bc | completed | 103 | 4286 | 112658 | 0 | 117047 | 0 | yes |
| 31 | repair | issue-repair | qa-repair-proposal | qa-v2-issue-6161cf7b352db4c23363500b | completed | 101 | 25 | 6999 | 0 | 7125 | 2 | yes |
| 32 | repair | issue-repair | qa-repair-proposal | qa-v2-issue-abe77ab75b026dbc9a4d49de | completed | 101 | 28 | 6961 | 0 | 7090 | 2 | yes |
| 33 | repair | issue-repair | qa-repair-proposal | qa-v2-issue-8d322738d3aeba31fd4021a8 | completed | 124 | 23 | 7315 | 0 | 7462 | 3 | yes |
| 34 | repair | issue-repair | qa-repair-proposal | qa-v2-issue-c66e8c8fdce4e7ba43a549f1 | completed | 102 | 34 | 7137 | 0 | 7273 | 2 | yes |
| 35 | repair | issue-repair | qa-repair-proposal | qa-v2-issue-15a2cdfbdaa707d394e9b29f | completed | 102 | 19 | 7275 | 0 | 7396 | 2 | yes |
| 36 | repair | issue-repair | qa-repair-proposal | qa-v2-issue-5d9f1a6ee43f04bcab2ff266 | completed | 123 | 28 | 7211 | 0 | 7362 | 2 | yes |
| 37 | repair | issue-repair | qa-repair-proposal | qa-v2-issue-f4ee37d16213392771126282 | completed | 101 | 33 | 7222 | 0 | 7356 | 2 | yes |
| 38 | repair | issue-repair | qa-repair-proposal | qa-v2-issue-2b79bbb142417213a193732e | completed | 101 | 26 | 10658 | 0 | 10785 | 2 | yes |

## Post-fix rerun with optional read tools

Run date: 2026-09-02. Source: `/home/antonioborgerees/coding/.ultraplan-qa-context-dogfood-20260831/evidence/lane-10-read-tools-rerun/final-runtime-metrics.json`

This is a fresh rerun of the same QA fixture after removing forced no-tool policies. UltraPlan requested bounded `read`, `list`, `search`, and `glob` access under the read-only sandbox. Agentwrap translated those permissions to OpenCode's native `read`, `list`, `grep`, and `glob` entries with action `allow`; retained calls report restricted enforcement, default deny, permission audits, and zero unsupported tools. Write, edit, patch, bash, and shell remained denied. All 32 calls used zero tools. Because no call exercised an allowed tool, this run proves policy translation but does not independently prove a successful end-to-end read invocation. The run completed with assessment `pass`, eight of eight shards complete, ten isolated evidence records, and no issues.

The first live command failed closed on malformed arbiter output. Two resumes then reached evidence admission but exposed an ignored 39,099,697-byte fixture binary above the 32 MiB isolation file limit. The binary was moved, not deleted, to the persistent lane evidence with its SHA-256. The next resume completed. Every failed and successful envelope remains in the lane evidence.

| Seq | Agent | Operation | Task | Status | Input | Output | Cache read | Cache write | Total | Tool calls | Exact |
|---:|---|---|---|---|---:|---:|---:|---:|---:|---:|:---:|
| 1 | semantic-mapper | qa-map | qa-v1-map-840b3616841900535f6fedab | completed | 109439 | 3765 | 1942 | 0 | 115146 | 0 | yes |
| 2 | investigator | qa-investigate | qa-v1-shard-31779ce812cda3e66d953d8b | completed | 32150 | 1632 | 128 | 0 | 33910 | 0 | yes |
| 3 | investigator | qa-investigate | qa-v1-shard-5f30bfc60c94c5d85fed2b2b | completed | 20835 | 2114 | 1956 | 0 | 24905 | 0 | yes |
| 4 | investigator | qa-investigate | qa-v1-shard-4d826b1c2dd2e0e4c4bfbd51 | completed | 22203 | 3492 | 128 | 0 | 25823 | 0 | yes |
| 5 | investigator | qa-investigate-output-repair | qa-v1-shard-31779ce812cda3e66d953d8b | completed | 1746 | 1534 | 32278 | 0 | 35558 | 0 | yes |
| 6 | investigator | qa-investigate-output-repair | qa-v1-shard-4d826b1c2dd2e0e4c4bfbd51 | completed | 3622 | 3494 | 22331 | 0 | 29447 | 0 | yes |
| 7 | investigator | qa-investigate-context-continuation | qa-v1-shard-5f30bfc60c94c5d85fed2b2b | completed | 7617 | 2260 | 22791 | 0 | 32668 | 0 | yes |
| 8 | investigator | qa-investigate-context-continuation | qa-v1-shard-31779ce812cda3e66d953d8b | completed | 6985 | 1795 | 34024 | 0 | 42804 | 0 | yes |
| 9 | investigator | qa-investigate | qa-v1-shard-6df850ca7af23f7cab4d33b6 | completed | 34576 | 3116 | 6019 | 0 | 43711 | 0 | yes |
| 10 | investigator | qa-investigate | qa-v1-shard-6f5a34c05ec8de39c2479cea | completed | 21618 | 3536 | 1924 | 0 | 27078 | 0 | yes |
| 11 | investigator | qa-investigate | qa-v1-shard-882541f5838debfb1158965c | completed | 55080 | 3016 | 1910 | 0 | 60006 | 0 | yes |
| 12 | investigator | qa-investigate-output-repair | qa-v1-shard-6f5a34c05ec8de39c2479cea | completed | 3649 | 2724 | 23542 | 0 | 29915 | 0 | yes |
| 13 | investigator | qa-investigate-context-continuation | qa-v1-shard-6df850ca7af23f7cab4d33b6 | completed | 11444 | 3851 | 40595 | 0 | 55890 | 0 | yes |
| 14 | investigator | qa-investigate-output-repair | qa-v1-shard-882541f5838debfb1158965c | completed | 3131 | 2885 | 56990 | 0 | 63006 | 0 | yes |
| 15 | investigator | qa-investigate-context-continuation-output-repair | qa-v1-shard-6df850ca7af23f7cab4d33b6 | completed | 3961 | 1254 | 52039 | 0 | 57254 | 0 | yes |
| 16 | investigator | qa-investigate | qa-v1-shard-bfeed0b5ed1420904acbe635 | completed | 32013 | 5190 | 1956 | 0 | 39159 | 0 | yes |
| 17 | investigator | qa-investigate | qa-v1-shard-8fd037a63e668eee41c524f5 | completed | 33116 | 621 | 128 | 0 | 33865 | 0 | yes |
| 18 | investigator | qa-investigate-context-continuation | qa-v1-shard-882541f5838debfb1158965c | completed | 12638 | 3441 | 60121 | 0 | 76200 | 0 | yes |
| 19 | investigator | qa-investigate-output-repair | qa-v1-shard-8fd037a63e668eee41c524f5 | completed | 752 | 529 | 33244 | 0 | 34525 | 0 | yes |
| 20 | investigator | qa-investigate-output-repair | qa-v1-shard-bfeed0b5ed1420904acbe635 | completed | 5298 | 4629 | 33969 | 0 | 43896 | 0 | yes |
| 21 | investigator | qa-investigate-context-continuation | qa-v1-shard-8fd037a63e668eee41c524f5 | completed | 3312 | 759 | 33996 | 0 | 38067 | 0 | yes |
| 22 | arbiter | qa-arbitrate | qa-v1-arbiter-group-7ff49f8e7a405ce35bc86269 | completed | 88942 | 2749 | 1956 | 0 | 93647 | 0 | yes |
| 23 | arbiter | qa-arbitrate | qa-v1-arbiter-group-7ff49f8e7a405ce35bc86269 | completed | 3148 | 2370 | 90898 | 0 | 96416 | 0 | yes |
| 24 | arbiter | qa-arbitrate | qa-v1-arbiter-group-7ff49f8e7a405ce35bc86269 | completed | 1 | 2263 | 90897 | 0 | 93161 | 0 | yes |
| 25 | arbiter | qa-arbitrate | qa-v1-arbiter-group-7ff49f8e7a405ce35bc86269 | completed | 2615 | 2175 | 90943 | 0 | 95733 | 0 | yes |
| 26 | issue_reconciler | qa-reconcile-issues | qa-v1-map-840b3616841900535f6fedab | completed | 3561 | 267 | 128 | 0 | 3956 | 0 | yes |
| 27 | arbiter | qa-arbitrate | qa-v1-arbiter-group-7ff49f8e7a405ce35bc86269 | completed | 1 | 2382 | 90897 | 0 | 93280 | 0 | yes |
| 28 | arbiter | qa-arbitrate | qa-v1-arbiter-group-7ff49f8e7a405ce35bc86269 | completed | 2740 | 2339 | 90914 | 0 | 95993 | 0 | yes |
| 29 | issue_reconciler | qa-reconcile-issues | qa-v1-map-840b3616841900535f6fedab | completed | 1774 | 280 | 1924 | 0 | 3978 | 0 | yes |
| 30 | issue_reconciler | qa-reconcile-issues | qa-v1-map-840b3616841900535f6fedab | completed | 315 | 276 | 3698 | 0 | 4289 | 0 | yes |
| 31 | arbiter | qa-arbitrate | qa-v1-arbiter-group-7ff49f8e7a405ce35bc86269 | completed | 1 | 2585 | 90897 | 0 | 93483 | 0 | yes |
| 32 | issue_reconciler | qa-reconcile-issues | qa-v1-map-840b3616841900535f6fedab | completed | 3547 | 251 | 128 | 0 | 3926 | 0 | yes |

Totals: 531,830 input tokens, 73,574 output tokens, 1,015,291 cache-read tokens, zero cache-write tokens, 1,620,695 total reported tokens, and zero tool calls across 32 retained QA calls.
