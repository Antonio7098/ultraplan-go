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


## Investigator-authored tests dogfood campaign

Run dates: 2026-09-02 through 2026-09-04. This table includes every retained QA and repair model call created by this campaign. Conformance Review and other non-QA agents are excluded. The immutable source ledger is `/home/antonioborgerees/coding/.ultraplan-qa-authored-tests-dogfood-20260902/evidence/final-runtime-metrics.json`.

| Seq | Attempt | Agent | Operation | Task | Status | Input | Output | Cache read | Cache write | Total | Tool calls | Exact |
|---:|---|---|---|---|---|---:|---:|---:|---:|---:|---:|:---:|
| 300 | qa-v1-attempt-ce01034f088c62730d9c9baa | semantic-mapper | qa-map | qa-v1-map-06f0fa0cbf1f72965954ec4d | completed | 109505 | 3673 | 1910 | 0 | 115088 | 0 | yes |
| 301 | qa-v1-attempt-ce01034f088c62730d9c9baa | investigator | qa-investigate | qa-v1-shard-0996e447beced8fcf2c5fda6 | completed | 20435 | 2586 | 1897 | 0 | 24918 | 0 | yes |
| 302 | qa-v1-attempt-ce01034f088c62730d9c9baa | investigator | qa-investigate | qa-v1-shard-5e0bfe70390df3e3bb0608b0 | completed | 58413 | 3264 | 1941 | 0 | 63618 | 0 | yes |
| 303 | qa-v1-attempt-ce01034f088c62730d9c9baa | investigator | qa-investigate | qa-v1-shard-094881bad022ff682cb6bdd3 | completed | 30852 | 5698 | 1941 | 0 | 38491 | 0 | yes |
| 304 | qa-v1-attempt-ce01034f088c62730d9c9baa | investigator | qa-investigate-context-continuation | qa-v1-shard-0996e447beced8fcf2c5fda6 | completed | 3291 | 2603 | 24903 | 0 | 30797 | 0 | yes |
| 305 | qa-v1-attempt-ce01034f088c62730d9c9baa | investigator | qa-investigate-context-continuation | qa-v1-shard-5e0bfe70390df3e3bb0608b0 | completed | 5725 | 3878 | 63603 | 0 | 73206 | 0 | yes |
| 306 | qa-v1-attempt-ce01034f088c62730d9c9baa | investigator | qa-investigate-context-continuation | qa-v1-shard-094881bad022ff682cb6bdd3 | completed | 8174 | 5703 | 38476 | 0 | 52353 | 0 | yes |
| 307 | qa-v1-attempt-ce01034f088c62730d9c9baa | investigator | qa-investigate | qa-v1-shard-87a98f2a703274929d2f62e1 | completed | 29963 | 4394 | 1956 | 0 | 36313 | 0 | yes |
| 308 | qa-v1-attempt-ce01034f088c62730d9c9baa | investigator | qa-investigate | qa-v1-shard-ad3bda42f5958c8e8bc1ed40 | completed | 54883 | 3298 | 1941 | 0 | 60122 | 0 | yes |
| 309 | qa-v1-attempt-ce01034f088c62730d9c9baa | investigator | qa-investigate-context-continuation | qa-v1-shard-87a98f2a703274929d2f62e1 | completed | 7189 | 3341 | 31919 | 0 | 42449 | 0 | yes |
| 310 | qa-v1-attempt-ce01034f088c62730d9c9baa | investigator | qa-investigate-context-continuation | qa-v1-shard-ad3bda42f5958c8e8bc1ed40 | completed | 2810 | 3206 | 60107 | 0 | 66123 | 0 | yes |
| 311 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 126731 | 9733 | 1957 | 0 | 138421 | 0 | yes |
| 312 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 10178 | 9602 | 128688 | 0 | 148468 | 0 | yes |
| 313 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 1 | 6226 | 128687 | 0 | 134914 | 0 | yes |
| 314 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 6658 | 6178 | 128704 | 0 | 141540 | 0 | yes |
| 315 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 126804 | 6567 | 1956 | 0 | 135327 | 0 | yes |
| 316 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 7013 | 6556 | 128760 | 0 | 142329 | 0 | yes |
| 317 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 128667 | 5947 | 128 | 0 | 134742 | 0 | yes |
| 318 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 6501 | 5927 | 128795 | 0 | 141223 | 0 | yes |
| 319 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 126835 | 5205 | 1956 | 0 | 133996 | 0 | yes |
| 320 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 5755 | 5212 | 128791 | 0 | 139758 | 0 | yes |
| 321 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 1 | 3611 | 128790 | 0 | 132402 | 0 | yes |
| 322 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 4129 | 3551 | 128807 | 0 | 136487 | 0 | yes |
| 323 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 1 | 5167 | 128790 | 0 | 133958 | 0 | yes |
| 324 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 5691 | 4855 | 128823 | 0 | 139369 | 0 | yes |
| 325 | qa-v1-attempt-ce01034f088c62730d9c9baa | issue_reconciler | qa-reconcile-issues | qa-v1-map-06f0fa0cbf1f72965954ec4d | completed | 2893 | 1535 | 1957 | 0 | 6385 | 0 | yes |
| 326 | qa-v1-attempt-ce01034f088c62730d9c9baa | issue_reconciler | qa-reconcile-issues | qa-v1-map-06f0fa0cbf1f72965954ec4d | completed | 1570 | 1535 | 4850 | 0 | 7955 | 0 | yes |
| 327 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 128663 | 4832 | 128 | 0 | 133623 | 0 | yes |
| 328 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 5386 | 5632 | 128791 | 0 | 139809 | 0 | yes |
| 329 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 1 | 4930 | 128790 | 0 | 133721 | 0 | yes |
| 330 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 5449 | 3122 | 128808 | 0 | 137379 | 0 | yes |
| 331 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 1 | 6246 | 128790 | 0 | 135037 | 0 | yes |
| 332 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 6776 | 6243 | 128807 | 0 | 141826 | 0 | yes |
| 333 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 126859 | 6194 | 1955 | 0 | 135008 | 0 | yes |
| 334 | qa-v1-attempt-ce01034f088c62730d9c9baa | arbiter | qa-arbitrate | qa-v1-arbiter-group-140ff92e934d2d06c365f544 | completed | 6773 | 3962 | 128814 | 0 | 139549 | 0 | yes |
| 335 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | semantic-mapper | qa-map | qa-v1-map-a9ad3c081158218e8297b634 | completed | 109464 | 1746 | 1956 | 0 | 113166 | 0 | yes |
| 336 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | semantic-mapper | qa-map | qa-v1-map-a9ad3c081158218e8297b634 | completed | 1792 | 1717 | 111420 | 0 | 114929 | 0 | yes |
| 337 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | investigator | qa-investigate | qa-v1-shard-3193408982dc662a93e61e28 | completed | 41456 | 1942 | 1956 | 0 | 45354 | 0 | yes |
| 338 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | investigator | qa-investigate | qa-v1-shard-6ab290df4c630f3916c596bf | completed | 31399 | 2699 | 128 | 0 | 34226 | 0 | yes |
| 339 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | investigator | qa-investigate-context-continuation | qa-v1-shard-3193408982dc662a93e61e28 | completed | 4741 | 2059 | 43412 | 0 | 50212 | 0 | yes |
| 340 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | investigator | qa-investigate | qa-v1-shard-23e63153174fbd672bc29665 | completed | 17109 | 1625 | 1956 | 0 | 20690 | 0 | yes |
| 341 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | investigator | qa-investigate-context-continuation | qa-v1-shard-6ab290df4c630f3916c596bf | completed | 8116 | 2701 | 31527 | 0 | 42344 | 0 | yes |
| 342 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | investigator | qa-investigate | qa-v1-shard-c590ff62914768bccc9a20bd | completed | 23229 | 3171 | 1956 | 0 | 28356 | 0 | yes |
| 343 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | investigator | qa-investigate-context-continuation | qa-v1-shard-23e63153174fbd672bc29665 | completed | 4851 | 1562 | 19065 | 0 | 25478 | 0 | yes |
| 344 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | investigator | qa-investigate-output-repair | qa-v1-shard-c590ff62914768bccc9a20bd | completed | 3286 | 3042 | 25185 | 0 | 31513 | 0 | yes |
| 345 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | investigator | qa-investigate-context-continuation | qa-v1-shard-c590ff62914768bccc9a20bd | completed | 14110 | 3326 | 28471 | 0 | 45907 | 0 | yes |
| 346 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | investigator | qa-investigate-context-continuation-output-repair | qa-v1-shard-c590ff62914768bccc9a20bd | completed | 3442 | 2557 | 42581 | 0 | 48580 | 0 | yes |
| 347 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | arbiter | qa-arbitrate | qa-v1-arbiter-group-6452913dd36ab80f17c1175b | completed | 49228 | 1515 | 1956 | 0 | 52699 | 0 | yes |
| 348 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | arbiter | qa-arbitrate | qa-v1-arbiter-group-bb9584c751dd5ec01450a23c | completed | 33901 | 2835 | 1956 | 0 | 38692 | 0 | yes |
| 349 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | investigator | qa-investigate-evidence-continuation | unknown | completed | 1119 | 2020 | 71626 | 0 | 74765 | 14 | yes |
| 350 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | investigator | qa-investigate-evidence-continuation | unknown | completed | 59406 | 2551 | 2153 | 0 | 64110 | 0 | yes |
| 351 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | arbiter | qa-arbitrate | qa-v1-arbiter-group-6452913dd36ab80f17c1175b | completed | 50534 | 1394 | 51184 | 0 | 103112 | 0 | yes |
| 352 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | investigator | qa-investigate-evidence-continuation | unknown | completed | 239 | 2134 | 86199 | 0 | 88572 | 5 | yes |
| 353 | qa-v1-attempt-98401cd3467c6117ee3f0e38 | arbiter | qa-arbitrate | qa-v1-arbiter-group-6452913dd36ab80f17c1175b | completed | 51645 | 1432 | 101718 | 0 | 154795 | 0 | yes |
| 354 | qa-v1-attempt-a1b4a1027eec0d6a81a0a090 | semantic-mapper | qa-map | qa-v1-map-b81623fad26158cb6dca9749 | completed | 111309 | 9710 | 128 | 0 | 121147 | 0 | yes |
| 355 | qa-v1-attempt-a1b4a1027eec0d6a81a0a090 | semantic-mapper | qa-map | qa-v1-map-b81623fad26158cb6dca9749 | completed | 9754 | 9708 | 111437 | 0 | 130899 | 0 | yes |
| 356 | qa-v1-attempt-786976aad89e2994a6d8a6f0 | semantic-mapper | qa-map | qa-v1-map-63d5699a4f892ce78efba47b | completed | 111303 | 3963 | 128 | 0 | 115394 | 0 | yes |
| 357 | qa-v1-attempt-786976aad89e2994a6d8a6f0 | semantic-mapper | qa-map | qa-v1-map-63d5699a4f892ce78efba47b | completed | 4008 | 3939 | 111431 | 0 | 119378 | 0 | yes |
| 358 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | semantic-mapper | qa-map | qa-v1-map-1be7fa39a703a2fca8abe526 | completed | 111316 | 2702 | 128 | 0 | 114146 | 0 | yes |
| 359 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | semantic-mapper | qa-map | qa-v1-map-1be7fa39a703a2fca8abe526 | completed | 2862 | 2659 | 111444 | 0 | 116965 | 0 | yes |
| 360 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | investigator | qa-investigate | qa-v1-shard-3ae5c0065e93bf75f4e3bcf7 | completed | 20544 | 2653 | 1956 | 0 | 25153 | 0 | yes |
| 361 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | investigator | qa-investigate | qa-v1-shard-3cf67c46748f670b167e415d | completed | 25825 | 3124 | 1956 | 0 | 30905 | 0 | yes |
| 362 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | investigator | qa-investigate-context-continuation | qa-v1-shard-3ae5c0065e93bf75f4e3bcf7 | completed | 4145 | 3160 | 22500 | 0 | 29805 | 0 | yes |
| 363 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | investigator | qa-investigate | qa-v1-shard-70315e9ee57a291cfafce830 | completed | 52051 | 4826 | 1956 | 0 | 58833 | 0 | yes |
| 364 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | investigator | qa-investigate-output-repair | qa-v1-shard-70315e9ee57a291cfafce830 | completed | 4936 | 1267 | 54007 | 0 | 60210 | 0 | yes |
| 365 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | investigator | qa-investigate-context-continuation | qa-v1-shard-70315e9ee57a291cfafce830 | completed | 6844 | 1323 | 58943 | 0 | 67110 | 0 | yes |
| 366 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | investigator | qa-investigate | qa-v1-shard-b70344614f64d1af285495d7 | completed | 41178 | 5988 | 128 | 0 | 47294 | 0 | yes |
| 367 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | investigator | qa-investigate-output-repair | qa-v1-shard-b70344614f64d1af285495d7 | completed | 6103 | 5438 | 41306 | 0 | 52847 | 0 | yes |
| 368 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | investigator | qa-investigate | qa-v1-shard-fc758164650b5080465701ed | completed | 29454 | 3650 | 1956 | 0 | 35060 | 0 | yes |
| 369 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | investigator | qa-investigate-output-repair | qa-v1-shard-fc758164650b5080465701ed | completed | 3758 | 3488 | 31410 | 0 | 38656 | 0 | yes |
| 370 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | investigator | qa-investigate-context-continuation | qa-v1-shard-b70344614f64d1af285495d7 | completed | 8257 | 4323 | 47409 | 0 | 59989 | 0 | yes |
| 371 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | investigator | qa-investigate-output-repair | qa-v1-shard-3cf67c46748f670b167e415d | completed | 1 | 3126 | 31011 | 0 | 34138 | 0 | yes |
| 372 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | investigator | qa-investigate-context-continuation | qa-v1-shard-3cf67c46748f670b167e415d | completed | 8645 | 3841 | 31012 | 0 | 43498 | 0 | yes |
| 373 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | arbiter | qa-arbitrate | qa-v1-arbiter-group-0aac61d59d1133d593b032d6 | completed | 43269 | 1561 | 1957 | 0 | 46787 | 0 | yes |
| 374 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | arbiter | qa-arbitrate | qa-v1-arbiter-group-56b88a844aa9d6554d4a3481 | completed | 44269 | 1463 | 796 | 0 | 46528 | 0 | yes |
| 375 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | arbiter | qa-arbitrate | qa-v1-arbiter-group-85ecdcd8608f5b1b653dfdf8 | completed | 36706 | 2114 | 1920 | 0 | 40740 | 0 | yes |
| 376 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | arbiter | qa-arbitrate | qa-v1-arbiter-group-c1f766d2f33e16d715633ae9 | completed | 57504 | 1025 | 1792 | 0 | 60321 | 0 | yes |
| 377 | qa-v1-attempt-a60cd305792af0d9a0ba45fe | arbiter | qa-arbitrate | qa-v1-arbiter-group-f251ac28912b0b0998c786e1 | completed | 25478 | 2052 | 1792 | 0 | 29322 | 0 | yes |
| 378 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | semantic-mapper | qa-map | qa-v1-map-f1275a70344ee3066baf40ef | completed | 109501 | 3668 | 1957 | 0 | 115126 | 0 | yes |
| 379 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate | qa-v1-shard-5b5b359ca77500d80503d632 | completed | 17796 | 1065 | 128 | 0 | 18989 | 0 | yes |
| 380 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate | qa-v1-shard-6753ce6b4b0a685830373b28 | completed | 30939 | 1363 | 1956 | 0 | 34258 | 0 | yes |
| 381 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate-output-repair | qa-v1-shard-6753ce6b4b0a685830373b28 | completed | 1493 | 976 | 32895 | 0 | 35364 | 0 | yes |
| 382 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate-context-continuation | qa-v1-shard-5b5b359ca77500d80503d632 | completed | 3260 | 1037 | 18944 | 0 | 23241 | 0 | yes |
| 383 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate | qa-v1-shard-8e27024daf94fc7377826291 | completed | 22584 | 2464 | 1792 | 0 | 26840 | 0 | yes |
| 384 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate | qa-v1-shard-952758a52239eb2d05eca36b | completed | 51577 | 2002 | 1920 | 0 | 55499 | 0 | yes |
| 385 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate-context-continuation | qa-v1-shard-8e27024daf94fc7377826291 | completed | 5707 | 2654 | 26752 | 0 | 35113 | 0 | yes |
| 386 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate-context-continuation | qa-v1-shard-952758a52239eb2d05eca36b | completed | 4243 | 1633 | 55424 | 0 | 61300 | 0 | yes |
| 387 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate-context-continuation-output-repair | qa-v1-shard-8e27024daf94fc7377826291 | completed | 152 | 2663 | 35072 | 0 | 37887 | 0 | yes |
| 388 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate | qa-v1-shard-4f4c288282d7080558e277d5 | completed | 35632 | 4765 | 1956 | 0 | 42353 | 0 | yes |
| 389 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate-context-continuation | qa-v1-shard-4f4c288282d7080558e277d5 | completed | 10164 | 6028 | 37588 | 0 | 53780 | 0 | yes |
| 390 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate-context-continuation-output-repair | qa-v1-shard-4f4c288282d7080558e277d5 | completed | 6143 | 5159 | 47752 | 0 | 59054 | 0 | yes |
| 391 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | arbiter | qa-arbitrate | qa-v1-arbiter-group-514bebf49f76afca4448e5b5 | completed | 9349 | 1111 | 3227 | 0 | 13687 | 0 | yes |
| 392 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | arbiter | qa-arbitrate | qa-v1-arbiter-group-a17ad226755ad3eeec5d0fec | completed | 46864 | 1180 | 1792 | 0 | 49836 | 0 | yes |
| 393 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | arbiter | qa-arbitrate | qa-v1-arbiter-group-b049438d2a16c6ecd54061fd | completed | 38770 | 1281 | 1910 | 0 | 41961 | 0 | yes |
| 394 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | arbiter | qa-arbitrate | qa-v1-arbiter-group-eab994ad945d2e337357a091 | completed | 38692 | 1553 | 1925 | 0 | 42170 | 0 | yes |
| 395 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate-evidence-continuation | unknown | completed | 26269 | 44 | 1792 | 0 | 28105 | 0 | yes |
| 396 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate-evidence-continuation | unknown | completed | 65055 | 132 | 1920 | 0 | 67107 | 0 | yes |
| 397 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate-evidence-continuation | unknown | completed | 76701 | 2758 | 2219 | 0 | 81678 | 0 | yes |
| 398 | qa-v1-attempt-270f663e1f84d8b77a8e5542 | investigator | qa-investigate-evidence-continuation | unknown | completed | 22860 | 2849 | 78920 | 0 | 104629 | 0 | yes |
| 399 | qa-v1-attempt-df5017c85f82412e531451c2 | semantic-mapper | qa-map | qa-v1-map-06a191501d6817ff945a06e0 | completed | 109491 | 10584 | 1956 | 0 | 122031 | 0 | yes |
| 400 | qa-v1-attempt-df5017c85f82412e531451c2 | semantic-mapper | qa-map | qa-v1-map-06a191501d6817ff945a06e0 | completed | 10706 | 10486 | 111447 | 0 | 132639 | 0 | yes |
| 401 | qa-v1-attempt-df5017c85f82412e531451c2 | semantic-mapper | qa-map | qa-v1-map-06a191501d6817ff945a06e0 | completed | 1 | 3248 | 111446 | 0 | 114695 | 0 | yes |
| 402 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate | qa-v1-shard-3f289c9d0ebec439f155b8ff | completed | 27954 | 1265 | 1956 | 0 | 31175 | 0 | yes |
| 403 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate | qa-v1-shard-6f73417074f0d5cb4386d2f9 | completed | 16761 | 2268 | 1956 | 0 | 20985 | 0 | yes |
| 404 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate-output-repair | qa-v1-shard-3f289c9d0ebec439f155b8ff | completed | 1378 | 1258 | 29910 | 0 | 32546 | 0 | yes |
| 405 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate | qa-v1-shard-7fbd464f54af23c091e4dd86 | completed | 32233 | 2868 | 1958 | 0 | 37059 | 0 | yes |
| 406 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate-context-continuation | qa-v1-shard-6f73417074f0d5cb4386d2f9 | completed | 5475 | 2625 | 18717 | 0 | 26817 | 0 | yes |
| 407 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate-context-continuation | qa-v1-shard-3f289c9d0ebec439f155b8ff | completed | 6878 | 1543 | 31288 | 0 | 39709 | 0 | yes |
| 408 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate-context-continuation | qa-v1-shard-7fbd464f54af23c091e4dd86 | completed | 10948 | 2883 | 34191 | 0 | 48022 | 0 | yes |
| 409 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate | qa-v1-shard-f40b1c68cebf4cb439aee5ec | completed | 47434 | 2080 | 1956 | 0 | 51470 | 0 | yes |
| 410 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate | qa-v1-shard-924606d8248a279c4122981f | completed | 34628 | 2821 | 1920 | 0 | 39369 | 0 | yes |
| 411 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate-context-continuation | qa-v1-shard-f40b1c68cebf4cb439aee5ec | completed | 10569 | 2396 | 49390 | 0 | 62355 | 0 | yes |
| 412 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate-context-continuation | qa-v1-shard-924606d8248a279c4122981f | completed | 5466 | 2655 | 39296 | 0 | 47417 | 0 | yes |
| 413 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-2f7d737ec9e648ea87c03a3d | completed | 24380 | 651 | 1956 | 0 | 26987 | 0 | yes |
| 414 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-321f5b4257869a16ce8be36a | completed | 17985 | 1130 | 128 | 0 | 19243 | 0 | yes |
| 415 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-430fb9c3de58d188e5e1ea67 | completed | 25728 | 803 | 1956 | 0 | 28487 | 0 | yes |
| 416 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-481822ebd6491d72321f39bf | completed | 32878 | 1024 | 1957 | 0 | 35859 | 0 | yes |
| 417 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-53bbd0ceebc50a61074f086b | completed | 29590 | 600 | 1956 | 0 | 32146 | 0 | yes |
| 418 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-5f5369aaeea49de40a7ae56c | completed | 15783 | 861 | 1956 | 0 | 18600 | 0 | yes |
| 419 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-aa15d281d3d26152c6d3f0cf | completed | 29848 | 545 | 1792 | 0 | 32185 | 0 | yes |
| 420 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-bd1d568740c3cb4787b67c8f | completed | 22819 | 703 | 1955 | 0 | 25477 | 0 | yes |
| 421 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-dda08238da176437c19959a2 | completed | 19362 | 978 | 1956 | 0 | 22296 | 0 | yes |
| 422 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-e8fbbac13c2b402e3dbf6dcf | completed | 26090 | 699 | 1956 | 0 | 28745 | 0 | yes |
| 423 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate-evidence-continuation | unknown | completed | 42923 | 2602 | 2215 | 0 | 47740 | 0 | yes |
| 424 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate-evidence-continuation | unknown | completed | 2690 | 2602 | 45138 | 0 | 50430 | 0 | yes |
| 425 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate-evidence-continuation | unknown | completed | 148 | 221 | 85248 | 0 | 85617 | 20 | yes |
| 426 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate-evidence-continuation | unknown | completed | 105 | 365 | 95863 | 0 | 96333 | 14 | yes |
| 427 | qa-v1-attempt-df5017c85f82412e531451c2 | investigator | qa-investigate-evidence-continuation | unknown | completed | 442 | 283 | 134556 | 0 | 135281 | 11 | yes |
| 428 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-aa15d281d3d26152c6d3f0cf | completed | 59269 | 401 | 1920 | 0 | 61590 | 0 | yes |
| 429 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-bd1d568740c3cb4787b67c8f | completed | 22858 | 830 | 24774 | 0 | 48462 | 0 | yes |
| 430 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-dda08238da176437c19959a2 | completed | 20170 | 839 | 21318 | 0 | 42327 | 0 | yes |
| 431 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-2f7d737ec9e648ea87c03a3d | completed | 50443 | 659 | 128 | 0 | 51230 | 0 | yes |
| 432 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-321f5b4257869a16ce8be36a | completed | 32213 | 2000 | 1957 | 0 | 36170 | 0 | yes |
| 433 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-430fb9c3de58d188e5e1ea67 | completed | 51027 | 709 | 1958 | 0 | 53694 | 0 | yes |
| 434 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-481822ebd6491d72321f39bf | completed | 65552 | 891 | 1956 | 0 | 68399 | 0 | yes |
| 435 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-53bbd0ceebc50a61074f086b | completed | 58586 | 916 | 1920 | 0 | 61422 | 0 | yes |
| 436 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-5f5369aaeea49de40a7ae56c | completed | 31197 | 666 | 1956 | 0 | 33819 | 0 | yes |
| 437 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-aa15d281d3d26152c6d3f0cf | completed | 90466 | 200 | 128 | 0 | 90794 | 0 | yes |
| 438 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-bd1d568740c3cb4787b67c8f | completed | 23501 | 319 | 47632 | 0 | 71452 | 0 | yes |
| 439 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-dda08238da176437c19959a2 | completed | 20034 | 286 | 41488 | 0 | 61808 | 0 | yes |
| 440 | qa-v1-attempt-df5017c85f82412e531451c2 | arbiter | qa-arbitrate | qa-v1-arbiter-group-e8fbbac13c2b402e3dbf6dcf | completed | 53477 | 676 | 128 | 0 | 54281 | 0 | yes |
| 441 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | semantic-mapper | qa-map | qa-v1-map-173ceaa657dcc1d11e8fd7e4 | completed | 109490 | 3223 | 1956 | 0 | 114669 | 0 | yes |
| 442 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | semantic-mapper | qa-map | qa-v1-map-173ceaa657dcc1d11e8fd7e4 | completed | 3344 | 3167 | 111446 | 0 | 117957 | 0 | yes |
| 443 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | semantic-mapper | qa-map | qa-v1-map-173ceaa657dcc1d11e8fd7e4 | completed | 109490 | 7589 | 1956 | 0 | 119035 | 0 | yes |
| 444 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate | qa-v1-shard-7938a7c1c28fdc97f54e8e02 | completed | 19422 | 1841 | 1956 | 0 | 23219 | 0 | yes |
| 445 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate | qa-v1-shard-1a4666aeee5d35da7bc990b1 | completed | 96895 | 2770 | 1980 | 0 | 101645 | 0 | yes |
| 446 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate | qa-v1-shard-79bc86993a2dcdbafe410e88 | completed | 30558 | 2728 | 1956 | 0 | 35242 | 0 | yes |
| 447 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate-output-repair | qa-v1-shard-7938a7c1c28fdc97f54e8e02 | completed | 1971 | 1834 | 21378 | 0 | 25183 | 0 | yes |
| 448 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate-output-repair | qa-v1-shard-1a4666aeee5d35da7bc990b1 | completed | 2881 | 2916 | 98875 | 0 | 104672 | 0 | yes |
| 449 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate-context-continuation | qa-v1-shard-79bc86993a2dcdbafe410e88 | completed | 8141 | 2636 | 32514 | 0 | 43291 | 0 | yes |
| 450 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate-context-continuation | qa-v1-shard-7938a7c1c28fdc97f54e8e02 | completed | 5059 | 1665 | 23349 | 0 | 30073 | 0 | yes |
| 451 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate | qa-v1-shard-b5f819abce24eb4c1ffd3c2b | completed | 46769 | 4351 | 128 | 0 | 51248 | 0 | yes |
| 452 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate-context-continuation | qa-v1-shard-b5f819abce24eb4c1ffd3c2b | completed | 5684 | 3071 | 51200 | 0 | 59955 | 0 | yes |
| 453 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate | qa-v1-shard-dbf3b0b154797f9c69b3ef53 | completed | 38017 | 3397 | 1924 | 0 | 43338 | 0 | yes |
| 454 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate-output-repair | qa-v1-shard-dbf3b0b154797f9c69b3ef53 | completed | 3527 | 3394 | 39941 | 0 | 46862 | 0 | yes |
| 455 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | arbiter | qa-arbitrate | qa-v1-arbiter-group-c535af3f4855d4250f3f8d0b | completed | 17076 | 1225 | 128 | 0 | 18429 | 0 | yes |
| 456 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | arbiter | qa-arbitrate | qa-v1-arbiter-group-d5fc1cfc47feb9c20dee119a | completed | 44303 | 782 | 128 | 0 | 45213 | 0 | yes |
| 457 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | arbiter | qa-arbitrate | qa-v1-arbiter-group-e535da9e3ca37ec6bdeccefc | completed | 16928 | 732 | 128 | 0 | 17788 | 0 | yes |
| 458 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | arbiter | qa-arbitrate | qa-v1-arbiter-group-fec31d16f496a0922cc3d89c | completed | 44423 | 1119 | 128 | 0 | 45670 | 0 | yes |
| 459 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate-evidence-continuation | unknown | completed | 69393 | 54 | 1920 | 0 | 71367 | 0 | yes |
| 460 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate-evidence-continuation | unknown | completed | 121 | 142 | 71296 | 0 | 71559 | 0 | yes |
| 461 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate-evidence-continuation | unknown | completed | 11781 | 42 | 71424 | 0 | 83247 | 0 | yes |
| 462 | qa-v1-attempt-603400ff0a1ff8fdd8bc3ae0 | investigator | qa-investigate-evidence-continuation | unknown | completed | 83308 | 123 | 83200 | 0 | 166631 | 0 | yes |
| 463 | qa-v1-attempt-17125f062b2d2aa093a128c1 | semantic-mapper | qa-map | qa-v1-map-f5f6390e9b636dd2858254f3 | completed | 109519 | 5163 | 1941 | 0 | 116623 | 0 | yes |
| 464 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate | qa-v1-shard-2344f2a79d1d85d822c759ad | completed | 17988 | 2151 | 1984 | 0 | 22123 | 0 | yes |
| 465 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate | qa-v1-shard-1e9f0062cf8435478d4eece0 | completed | 39507 | 1954 | 2014 | 0 | 43475 | 0 | yes |
| 466 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate | qa-v1-shard-59df45fe5ff45fa04cc1d131 | completed | 31795 | 2218 | 1957 | 0 | 35970 | 0 | yes |
| 467 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate-context-continuation | qa-v1-shard-1e9f0062cf8435478d4eece0 | completed | 4717 | 1415 | 41521 | 0 | 47653 | 0 | yes |
| 468 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate-context-continuation | qa-v1-shard-2344f2a79d1d85d822c759ad | completed | 4941 | 2214 | 19972 | 0 | 27127 | 0 | yes |
| 469 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate-context-continuation | qa-v1-shard-59df45fe5ff45fa04cc1d131 | completed | 7564 | 2052 | 33752 | 0 | 43368 | 0 | yes |
| 470 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate-context-continuation-output-repair | qa-v1-shard-1e9f0062cf8435478d4eece0 | completed | 1533 | 1504 | 46238 | 0 | 49275 | 0 | yes |
| 471 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate | qa-v1-shard-883d8811be5f090cdf95f3fc | completed | 16450 | 2736 | 1958 | 0 | 21144 | 0 | yes |
| 472 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate | qa-v1-shard-a700bbe09915506ba1bed4a1 | completed | 14871 | 1093 | 1956 | 0 | 17920 | 0 | yes |
| 473 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate | qa-v1-shard-94828c24db676c5a03660307 | completed | 25037 | 1115 | 1792 | 0 | 27944 | 0 | yes |
| 474 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate-context-continuation | qa-v1-shard-94828c24db676c5a03660307 | completed | 2782 | 672 | 27904 | 0 | 31358 | 0 | yes |
| 475 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate-context-continuation | qa-v1-shard-883d8811be5f090cdf95f3fc | completed | 8373 | 2734 | 18408 | 0 | 29515 | 0 | yes |
| 476 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate-context-continuation | qa-v1-shard-a700bbe09915506ba1bed4a1 | completed | 2536 | 1237 | 16827 | 0 | 20600 | 0 | yes |
| 477 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate | qa-v1-shard-e057948d6551ca3996a059e9 | completed | 23956 | 3265 | 1956 | 0 | 29177 | 0 | yes |
| 478 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate | qa-v1-shard-fcf64fde09d6588099982c6d | completed | 33383 | 3994 | 1980 | 0 | 39357 | 0 | yes |
| 479 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate-output-repair | qa-v1-shard-fcf64fde09d6588099982c6d | completed | 4104 | 3946 | 35363 | 0 | 43413 | 0 | yes |
| 480 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate-context-continuation | qa-v1-shard-e057948d6551ca3996a059e9 | completed | 8613 | 2778 | 25912 | 0 | 37303 | 0 | yes |
| 481 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-13255efd092a126abcfa0af8 | completed | 14108 | 722 | 1956 | 0 | 16786 | 0 | yes |
| 482 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-16ddacd9d415dd7b970a43c5 | completed | 16607 | 617 | 1920 | 0 | 19144 | 0 | yes |
| 483 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-22115b8513c48b16f7e75340 | completed | 14163 | 551 | 1955 | 0 | 16669 | 0 | yes |
| 484 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-22e93f4d5e40a9114a9feeaf | completed | 23104 | 940 | 1956 | 0 | 26000 | 0 | yes |
| 485 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-22ed11faa606d6e98db4ab3f | completed | 8017 | 475 | 1956 | 0 | 10448 | 0 | yes |
| 486 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-2a23f84bff81ae88e475f966 | completed | 11455 | 528 | 1956 | 0 | 13939 | 0 | yes |
| 487 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-2da89b6ef382cf6527b0907b | completed | 17072 | 1821 | 128 | 0 | 19021 | 0 | yes |
| 488 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-465c74767f40a11e7b73f59d | completed | 14197 | 1017 | 1956 | 0 | 17170 | 0 | yes |
| 489 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-4f434814336c803b068b84d4 | completed | 16310 | 841 | 1956 | 0 | 19107 | 0 | yes |
| 490 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-57572364c4186f0d73596406 | completed | 14268 | 555 | 1956 | 0 | 16779 | 0 | yes |
| 491 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-705d8575fdff0a95de9a863e | completed | 9904 | 583 | 128 | 0 | 10615 | 0 | yes |
| 492 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-9e949038e711d398f95a3050 | completed | 23124 | 1208 | 1920 | 0 | 26252 | 0 | yes |
| 493 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-b467c46026f9522ca069d320 | completed | 9983 | 518 | 128 | 0 | 10629 | 0 | yes |
| 494 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-c134e8b27473583b4e06d614 | completed | 11403 | 566 | 1956 | 0 | 13925 | 0 | yes |
| 495 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-dcc8622cce838085eaaee058 | completed | 11529 | 712 | 1956 | 0 | 14197 | 0 | yes |
| 496 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-f1711ce34029a4cd9dfd40b8 | completed | 14294 | 842 | 128 | 0 | 15264 | 0 | yes |
| 497 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate-evidence-continuation | unknown | completed | 1609 | 102 | 71224 | 0 | 72935 | 9 | yes |
| 498 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate-evidence-continuation | unknown | completed | 1097 | 107 | 85670 | 0 | 86874 | 2 | yes |
| 499 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate-evidence-continuation | unknown | completed | 32086 | 2491 | 2217 | 0 | 36794 | 0 | yes |
| 500 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate-evidence-continuation | unknown | completed | 1266 | 2693 | 50561 | 0 | 54520 | 6 | yes |
| 501 | qa-v1-attempt-17125f062b2d2aa093a128c1 | investigator | qa-investigate-evidence-continuation | unknown | completed | 1494 | 188 | 45901 | 0 | 47583 | 14 | yes |
| 502 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-13255efd092a126abcfa0af8 | completed | 14194 | 682 | 16064 | 0 | 30940 | 0 | yes |
| 503 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-22115b8513c48b16f7e75340 | completed | 14573 | 230 | 16118 | 0 | 30921 | 0 | yes |
| 504 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-2a23f84bff81ae88e475f966 | completed | 11304 | 223 | 13411 | 0 | 24938 | 0 | yes |
| 505 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-b467c46026f9522ca069d320 | completed | 18011 | 191 | 128 | 0 | 18330 | 0 | yes |
| 506 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-13255efd092a126abcfa0af8 | completed | 14674 | 481 | 30258 | 0 | 45413 | 0 | yes |
| 507 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-16ddacd9d415dd7b970a43c5 | completed | 34357 | 720 | 128 | 0 | 35205 | 0 | yes |
| 508 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-22115b8513c48b16f7e75340 | completed | 14253 | 215 | 30691 | 0 | 45159 | 0 | yes |
| 509 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-22e93f4d5e40a9114a9feeaf | completed | 47746 | 841 | 128 | 0 | 48715 | 0 | yes |
| 510 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-22ed11faa606d6e98db4ab3f | completed | 15279 | 504 | 1956 | 0 | 17739 | 0 | yes |
| 511 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-2a23f84bff81ae88e475f966 | completed | 11000 | 224 | 24715 | 0 | 35939 | 0 | yes |
| 512 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-2da89b6ef382cf6527b0907b | completed | 31079 | 1673 | 1956 | 0 | 34708 | 0 | yes |
| 513 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-465c74767f40a11e7b73f59d | completed | 30993 | 370 | 128 | 0 | 31491 | 0 | yes |
| 514 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-4f434814336c803b068b84d4 | completed | 32230 | 671 | 1956 | 0 | 34857 | 0 | yes |
| 515 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-57572364c4186f0d73596406 | completed | 28825 | 627 | 1956 | 0 | 31408 | 0 | yes |
| 516 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-705d8575fdff0a95de9a863e | completed | 15504 | 538 | 1957 | 0 | 17999 | 0 | yes |
| 517 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-9e949038e711d398f95a3050 | completed | 47982 | 1239 | 128 | 0 | 49349 | 0 | yes |
| 518 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-b467c46026f9522ca069d320 | completed | 23921 | 203 | 1920 | 0 | 26044 | 0 | yes |
| 519 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-c134e8b27473583b4e06d614 | completed | 22118 | 607 | 1980 | 0 | 24705 | 0 | yes |
| 520 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-dcc8622cce838085eaaee058 | completed | 22540 | 631 | 1956 | 0 | 25127 | 0 | yes |
| 521 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-f1711ce34029a4cd9dfd40b8 | completed | 22087 | 879 | 4413 | 0 | 27379 | 0 | yes |
| 522 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-13255efd092a126abcfa0af8 | completed | 14473 | 491 | 44932 | 0 | 59896 | 0 | yes |
| 523 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-22115b8513c48b16f7e75340 | completed | 14238 | 235 | 44944 | 0 | 59417 | 0 | yes |
| 524 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-2a23f84bff81ae88e475f966 | completed | 11001 | 222 | 35715 | 0 | 46938 | 0 | yes |
| 525 | qa-v1-attempt-17125f062b2d2aa093a128c1 | arbiter | qa-arbitrate | qa-v1-arbiter-group-b467c46026f9522ca069d320 | completed | 31763 | 163 | 1792 | 0 | 33718 | 0 | yes |
| 526 | qa-v1-attempt-17beb8c93b489e7de239ffa9 | issue-repair | qa-repair-proposal | qa-v2-issue-8089b4d4a85bb9b1b62a7cc4 | completed | 161 | 23 | 7296 | 0 | 7480 | 2 | yes |
| 527 | qa-v1-attempt-17beb8c93b489e7de239ffa9 | issue-repair | qa-repair-proposal | qa-v2-issue-447e431118d936af6d9381f4 | completed | 75 | 23 | 10880 | 0 | 10978 | 2 | yes |
| 528 | qa-v1-attempt-17beb8c93b489e7de239ffa9 | issue-repair | qa-repair-proposal | qa-v2-issue-447e431118d936af6d9381f4 | completed | 67 | 25 | 7533 | 0 | 7625 | 2 | yes |
| 529 | qa-v1-attempt-17beb8c93b489e7de239ffa9 | issue-repair | qa-repair-proposal | qa-v2-issue-8089b4d4a85bb9b1b62a7cc4 | completed | 73 | 28 | 7424 | 0 | 7525 | 2 | yes |
| 530 | qa-v1-attempt-17beb8c93b489e7de239ffa9 | issue-repair | qa-repair-proposal | qa-v2-issue-447e431118d936af6d9381f4 | completed | 129 | 35 | 10752 | 0 | 10916 | 1 | yes |
| 531 | qa-v1-attempt-26286c0fb478c4d017df2cdf | issue-repair | qa-repair-proposal | qa-v2-issue-78bbf6cf92cc5f0b1d4d0450 | completed | 67 | 23 | 7424 | 0 | 7514 | 2 | yes |
| 532 | qa-v1-attempt-26286c0fb478c4d017df2cdf | issue-repair | qa-repair-proposal | qa-v2-issue-78bbf6cf92cc5f0b1d4d0450 | completed | 129 | 21 | 7321 | 0 | 7471 | 2 | yes |
| 533 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | semantic-mapper | qa-map | qa-v1-map-95aa9228bfc5f5c13a5e85f6 | completed | 111317 | 3625 | 128 | 0 | 115070 | 0 | yes |
| 534 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate | qa-v1-shard-3f9be43ae92d06063338f090 | completed | 29104 | 2485 | 1895 | 0 | 33484 | 0 | yes |
| 535 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate | qa-v1-shard-302f06e864bffa1963d8f7d4 | completed | 23754 | 1644 | 128 | 0 | 25526 | 0 | yes |
| 536 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-output-repair | qa-v1-shard-3f9be43ae92d06063338f090 | completed | 1282 | 2867 | 32313 | 0 | 36462 | 0 | yes |
| 537 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate | qa-v1-shard-22f857197f22dbf26753e901 | completed | 48831 | 1958 | 128 | 0 | 50917 | 0 | yes |
| 538 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-output-repair | qa-v1-shard-302f06e864bffa1963d8f7d4 | completed | 1758 | 1562 | 23882 | 0 | 27202 | 0 | yes |
| 539 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-output-repair | qa-v1-shard-22f857197f22dbf26753e901 | completed | 2072 | 1911 | 48959 | 0 | 52942 | 0 | yes |
| 540 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate | qa-v1-shard-55e00ae30f8341b528621464 | completed | 32373 | 1875 | 128 | 0 | 34376 | 0 | yes |
| 541 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-context-continuation | qa-v1-shard-302f06e864bffa1963d8f7d4 | completed | 4465 | 2579 | 25640 | 0 | 32684 | 0 | yes |
| 542 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-context-continuation | qa-v1-shard-22f857197f22dbf26753e901 | completed | 7446 | 1638 | 51031 | 0 | 60115 | 0 | yes |
| 543 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-context-continuation | qa-v1-shard-55e00ae30f8341b528621464 | completed | 7340 | 2252 | 32501 | 0 | 42093 | 0 | yes |
| 544 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate | qa-v1-shard-a8b3777893b83e3bb9aac06a | completed | 38659 | 2644 | 128 | 0 | 41431 | 0 | yes |
| 545 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate | qa-v1-shard-850a09f7a5a48fa08b4e1f58 | completed | 21435 | 2166 | 128 | 0 | 23729 | 0 | yes |
| 546 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-context-continuation | qa-v1-shard-a8b3777893b83e3bb9aac06a | completed | 2882 | 2929 | 41344 | 0 | 47155 | 0 | yes |
| 547 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate | qa-v1-shard-d578fe5c962b0be8ffb0c8cd | completed | 22342 | 1464 | 128 | 0 | 23934 | 0 | yes |
| 548 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-output-repair | qa-v1-shard-850a09f7a5a48fa08b4e1f58 | completed | 2280 | 2156 | 21563 | 0 | 25999 | 0 | yes |
| 549 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-output-repair | qa-v1-shard-d578fe5c962b0be8ffb0c8cd | completed | 1593 | 1462 | 22470 | 0 | 25525 | 0 | yes |
| 550 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate | qa-v1-shard-f39aad19459172ec6e431219 | completed | 29609 | 3462 | 1910 | 0 | 34981 | 0 | yes |
| 551 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-context-continuation | qa-v1-shard-d578fe5c962b0be8ffb0c8cd | completed | 4279 | 1639 | 24063 | 0 | 29981 | 0 | yes |
| 552 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-context-continuation | qa-v1-shard-850a09f7a5a48fa08b4e1f58 | completed | 5403 | 2195 | 23843 | 0 | 31441 | 0 | yes |
| 553 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-output-repair | qa-v1-shard-f39aad19459172ec6e431219 | completed | 3569 | 3561 | 31519 | 0 | 38649 | 0 | yes |
| 554 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-2d384c0d93a58d1b8ecbe518 | completed | 20698 | 629 | 128 | 0 | 21455 | 0 | yes |
| 555 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-2fa866df764103dff4b61e20 | completed | 18800 | 701 | 1955 | 0 | 21456 | 0 | yes |
| 556 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-490246aeacdcc1f67acc43e7 | completed | 22123 | 778 | 1792 | 0 | 24693 | 0 | yes |
| 557 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-4a4c294b8cd3a1564ad4d324 | completed | 23447 | 1638 | 128 | 0 | 25213 | 0 | yes |
| 558 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-51ed66718424dd15f7e7346a | completed | 18683 | 473 | 148 | 0 | 19304 | 0 | yes |
| 559 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-549987c7de9a2c20fcba203e | completed | 18024 | 1218 | 128 | 0 | 19370 | 0 | yes |
| 560 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-799be597d15d43a29ac2dfa7 | completed | 23901 | 685 | 128 | 0 | 24714 | 0 | yes |
| 561 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-80746bc81d7c4ea98c42119b | completed | 16189 | 856 | 1924 | 0 | 18969 | 0 | yes |
| 562 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-95d48c79567f1a0a15ea90cc | completed | 23845 | 776 | 128 | 0 | 24749 | 0 | yes |
| 563 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-a05e05d354daa3b27b1a5dd7 | completed | 29314 | 865 | 128 | 0 | 30307 | 0 | yes |
| 564 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-a32bbc349ffd7674538a39d8 | completed | 21267 | 283 | 1792 | 0 | 23342 | 0 | yes |
| 565 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-c716325bb4e4f4c020cbbdec | completed | 32153 | 707 | 1910 | 0 | 34770 | 0 | yes |
| 566 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-ea1be3613f8d264ea585aa44 | completed | 33433 | 1182 | 128 | 0 | 34743 | 0 | yes |
| 567 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-f7b78592e8f3e7fb238770ef | completed | 23896 | 797 | 128 | 0 | 24821 | 0 | yes |
| 568 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-evidence-continuation | unknown | completed | 127 | 1343 | 58885 | 0 | 60355 | 16 | yes |
| 569 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-evidence-continuation | unknown | completed | 1703 | 1987 | 70962 | 0 | 74652 | 2 | yes |
| 570 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-evidence-continuation | unknown | completed | 467 | 1227 | 68204 | 0 | 69898 | 9 | yes |
| 571 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-evidence-continuation | unknown | completed | 1583 | 319 | 44039 | 0 | 45941 | 3 | yes |
| 572 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-evidence-continuation | unknown | completed | 1087 | 807 | 83113 | 0 | 85007 | 3 | yes |
| 573 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-evidence-continuation | unknown | failed | 1920 | 404 | 174005 | 0 | 176329 | 50 | no |
| 574 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-evidence-continuation | unknown | completed | 51976 | 474 | 1920 | 0 | 54370 | 0 | yes |
| 575 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-evidence-continuation | unknown | completed | 186 | 41 | 54272 | 0 | 54499 | 0 | yes |
| 576 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-2d384c0d93a58d1b8ecbe518 | completed | 37718 | 223 | 1920 | 0 | 39861 | 0 | yes |
| 577 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-2fa866df764103dff4b61e20 | completed | 39959 | 643 | 128 | 0 | 40730 | 0 | yes |
| 578 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-490246aeacdcc1f67acc43e7 | completed | 44081 | 719 | 1920 | 0 | 46720 | 0 | yes |
| 580 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-2d384c0d93a58d1b8ecbe518 | completed | 57917 | 204 | 128 | 0 | 58249 | 0 | yes |
| 581 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-2fa866df764103dff4b61e20 | completed | 19276 | 684 | 40087 | 0 | 60047 | 0 | yes |
| 582 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-490246aeacdcc1f67acc43e7 | completed | 21437 | 671 | 46592 | 0 | 68700 | 0 | yes |
| 583 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-4a4c294b8cd3a1564ad4d324 | completed | 66731 | 473 | 128 | 0 | 67332 | 0 | yes |
| 584 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-4a4c294b8cd3a1564ad4d324 | completed | 1026 | 732 | 66859 | 0 | 68617 | 0 | yes |
| 585 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-95d48c79567f1a0a15ea90cc | completed | 46494 | 949 | 128 | 0 | 47571 | 0 | yes |
| 586 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-95d48c79567f1a0a15ea90cc | completed | 1502 | 1010 | 46622 | 0 | 49134 | 0 | yes |
| 587 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-evidence-continuation | unknown | completed | 1400 | 919 | 101591 | 0 | 103910 | 3 | yes |
| 588 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-evidence-continuation | unknown | completed | 378 | 424 | 186158 | 0 | 186960 | 2 | yes |
| 589 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-evidence-continuation | unknown | completed | 60714 | 123 | 1920 | 0 | 62757 | 0 | yes |
| 590 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-evidence-continuation | unknown | completed | 125 | 14 | 60672 | 0 | 60811 | 0 | yes |
| 591 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-evidence-continuation | unknown | completed | 988 | 794 | 119762 | 0 | 121544 | 5 | yes |
| 592 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-95d48c79567f1a0a15ea90cc | completed | 71498 | 1163 | 128 | 0 | 72789 | 0 | yes |
| 593 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-c716325bb4e4f4c020cbbdec | completed | 64766 | 1121 | 1955 | 0 | 67842 | 0 | yes |
| 594 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-f7b78592e8f3e7fb238770ef | completed | 45989 | 827 | 1792 | 0 | 48608 | 0 | yes |
| 595 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | investigator | qa-investigate-evidence-continuation | unknown | completed | 2049 | 434 | 208914 | 0 | 211397 | 10 | yes |
| 596 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | arbiter | qa-arbitrate | qa-v1-arbiter-group-c716325bb4e4f4c020cbbdec | completed | 100281 | 1339 | 128 | 0 | 101748 | 0 | yes |
| 597 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | issue-repair | qa-repair-proposal | qa-v2-issue-0ac7dcc692bd709ddd383310 | completed | 435 | 126 | 32986 | 0 | 33547 | 14 | yes |
| 598 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | issue-repair | qa-repair-proposal | qa-v2-issue-0ac7dcc692bd709ddd383310 | completed | 822 | 82 | 28894 | 0 | 29798 | 9 | yes |
| 599 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | issue-repair | qa-repair-proposal | qa-v2-issue-0ac7dcc692bd709ddd383310 | completed | 624 | 107 | 47486 | 0 | 48217 | 11 | yes |
| 600 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | issue-repair | qa-repair-proposal | qa-v2-issue-0ac7dcc692bd709ddd383310 | completed | 22155 | 52 | 1792 | 0 | 23999 | 0 | yes |
| 601 | qa-v1-attempt-77bf26b1fcbff4b449d4e784 | issue-repair | qa-repair-proposal | qa-v2-issue-0ac7dcc692bd709ddd383310 | completed | 397 | 59 | 41314 | 0 | 41770 | 11 | yes |

Campaign totals: 301 model calls, 7,695,962 uncached input tokens, 620,480 output tokens, 8,433,471 cache-read tokens, zero cache-write tokens, 16,749,913 total reported tokens, and 256 observed tool calls.

At OpenRouter's 2026-09-04 paid MiniMax M3 list rates of $0.23/M uncached input, $0.96/M output, and $0.05/M cache read, the API-equivalent campaign cost is $2.787406: $1.770071 input, $0.595661 output, and $0.421674 cache read. The actual runs used `minimax-m3:free`, so this is a paid-model counterfactual, not money spent.

Compared with the previous post-policy rerun above, this campaign used 301 versus 32 calls and 256 versus zero tool calls. The larger count includes every retained failed and successful attempt while the flow was being repaired. The final live QA-and-promotion slice used 68 calls, 1,495,377 uncached input tokens, 80,156 output tokens, 1,989,778 cache-read tokens, 3,565,311 total reported tokens, and 148 tool calls. The prior campaign's 32-call run used 531,830 input, 73,574 output, 1,015,291 cache-read, 1,620,695 total tokens, and zero tool calls. The completed evidence-producing slice used 2.1 times as many calls, 2.8 times the uncached input, 1.1 times the output, 2.0 times the cache reads, and 2.2 times the total tokens. Those extra calls include investigator test authoring, evidence continuation, exact-session re-arbitration, five repair proposals, and the successful paired-patch promotion.

## Sprint 39 GPT-5.6-sol QA dogfood

Run date: 2026-09-06. Source: `/home/antonioborgerees/coding/ultraplan/ultraplan-go-workspace/projects/ultraplan-go/sprints/39-performance-stage/.runtime-metrics.json`

This is the first retained QA dogfood on a paid OpenAI route. The retained QA and repair calls from the prior sections above used `openrouter/minimax/minimax-m3:free`, which reported zero cost. Sprint 39 switched the QA model route to `openai/gpt-5.6-sol`, which reported the actual paid cost per call. The attempt ended incomplete: six of thirteen shards returned `permission_denied` when the investigator tried to create its private per-shard target copy, so the assessment is `pass_with_findings` with 14 evidence records accepted, 0 rejected, 0 promoted issues.

The two cancelled attempts that preceded this one are excluded from the table because they reported no metrics: `qa-v1-attempt-43e4e9d0af621e8189e8e6f6` on `minimax-coding-plan/MiniMax-M3/high` and `qa-v1-attempt-29a1a22c3d351fa1ebd0b2d8` on `openai/got-5.6-sol/low`. The table below covers only the 18 retained calls on the successful attempt `qa-v1-attempt-c382cef18d1a659e6fa57c62`.

| Seq | Attempt | Agent | Operation | Task | Status | Input | Output | Cache read | Cache write | Total | Tool calls | Exact |
|---:|---|---|---|---|---|---:|---:|---:|---:|---:|---:|:---:|
| 54 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | semantic-mapper | qa-map | qa-v1-map-0ce19d014b123dbd4b9d4d44 | completed | 152245 | 6291 | unknown | known | 158790 | 0 | yes |
| 55 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | semantic-mapper | qa-map | qa-v1-map-0ce19d014b123dbd4b9d4d44 | completed | 6842 | 5984 | 152064 | known | 165170 | 0 | yes |
| 56 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | investigator | qa-investigate | qa-v1-shard-55c75ed35ae9a28675ef8779 | completed | 919 | 686 | 28928 | known | 30856 | 3 | yes |
| 57 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | investigator | qa-investigate | qa-v1-shard-65c6fc03c89d778911b504e3 | completed | 9835 | 517 | 31232 | known | 41976 | 8 | yes |
| 58 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | investigator | qa-investigate | qa-v1-shard-62b033bcef3afb3af1bba41c | completed | 2146 | 1091 | 34176 | known | 37805 | 4 | yes |
| 59 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | investigator | qa-investigate-output-repair | qa-v1-shard-55c75ed35ae9a28675ef8779 | completed | 1287 | 698 | 29696 | known | 31715 | 0 | yes |
| 60 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | investigator | qa-investigate-output-repair | qa-v1-shard-65c6fc03c89d778911b504e3 | completed | 1143 | 486 | 40960 | known | 42649 | 0 | yes |
| 61 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | investigator | qa-investigate | qa-v1-shard-81e0d9c9e1087e58135f030b | completed | 33469 | 504 | unknown | known | 34307 | 0 | yes |
| 62 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | investigator | qa-investigate | qa-v1-shard-b29b255f71007d67ac6ef323 | completed | 24768 | 453 | unknown | known | 25721 | 0 | yes |
| 63 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | investigator | qa-investigate-output-repair | qa-v1-shard-81e0d9c9e1087e58135f030b | completed | 1154 | 564 | 33280 | known | 35058 | 0 | yes |
| 64 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | investigator | qa-investigate-output-repair | qa-v1-shard-b29b255f71007d67ac6ef323 | completed | 1272 | 470 | 24576 | known | 26337 | 0 | yes |
| 65 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | investigator | qa-investigate | qa-v1-shard-85fe99477f20d95596434ef9 | completed | 735 | 1021 | 34432 | known | 36428 | 6 | yes |
| 66 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | investigator | qa-investigate | qa-v1-shard-b951f08c0478926a7e675a81 | completed | 32987 | 447 | unknown | known | 33861 | 0 | yes |
| 67 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | investigator | qa-investigate-output-repair | qa-v1-shard-b951f08c0478926a7e675a81 | completed | 1092 | 458 | 32896 | known | 34477 | 0 | yes |
| 68 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | arbiter | qa-arbitrate | qa-v1-arbiter-group-d5b86fa600c0d38f48c365a9 | completed | 94237 | 2022 | unknown | known | 96426 | 0 | yes |
| 69 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | issue_reconciler | qa-reconcile-issues | qa-v1-map-0ce19d014b123dbd4b9d4d44 | completed | 3998 | 898 | unknown | known | 4896 | 0 | yes |
| 70 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | arbiter | qa-arbitrate | qa-v1-arbiter-group-d5b86fa600c0d38f48c365a9 | completed | 93857 | 2098 | 94080 | known | 190082 | 0 | yes |
| 71 | qa-v1-attempt-c382cef18d1a659e6fa57c62 | issue_reconciler | qa-reconcile-issues | qa-v1-map-0ce19d014b123dbd4b9d4d44 | completed | 3998 | 898 | unknown | known | 4896 | 0 | yes |

`Cache read: unknown` means the metric file marked the value unknown for that call even though the metric exists. `Cache write: known` means the field is known to be a real value but the metric file did not record the count. The reasoning token field (separate from the output token field) is reported per call in the source runtime-metrics.json and is folded into the Total column.

Role totals across the 18 completed calls: semantic-mapper 2 calls (159,087 input, 12,275 output, 152,064 cache read, 323,960 total, $1.3637), investigator 12 calls (110,807 input, 7,395 output, 290,176 cache read, 411,190 total, 21 tool calls, $1.0131), arbiter 2 calls (188,094 input, 4,120 output, 94,080 cache read, 286,508 total, $1.2222), issue_reconciler 2 calls (7,996 input, 1,796 output, 0 cache read, 9,792 total, $0.1032). Total 18 calls, 465,984 input, 25,586 output, 536,320 cache read, 1,031,450 total, 21 tool calls, $3.7022.

Cancelled calls: `qa-v1-attempt-43e4e9d0af621e8189e8e6f6` on `minimax-coding-plan/MiniMax-M3/high` and `qa-v1-attempt-29a1a22c3d351fa1ebd0b2d8` on `openai/got-5.6-sol/low` (model name typo for `gpt-5.6-sol`). Both cancelled during validation with `error_category: cancellation` and reported no usage metrics. They appear in the source file at sequences 52 and 53.

Wall time: first call 2026-09-06T14:09:42+01:00, last call 2026-09-06T15:41:50+01:00, total 92 minutes 8 seconds. Sum of call durations 1,099.3 seconds. The 4,429-second gap between the mapper finishing at 14:13:52 and the investigators starting at 15:29:16 is not represented by any retained call.

Compared with the 30-call MiniMax M3 final fixture ledger above (Seq 1 to 30), the 18-call GPT run used 465,984 vs 979,779 uncached input (-52.4%), 25,586 vs 85,903 output (-70.2%), 536,320 vs 775,807 cache read (-30.9%), and 1,031,450 vs 1,841,489 total reported tokens (-44.0%). The MiniMax M3 run cost $0 on the free route; the GPT run cost $3.7022 on the paid OpenAI route. The comparison is not exact because the Sprint 39 attempt was incomplete (six shards refused to create their per-shard target copy and produced no evidence or theories) and the MiniMax M3 fixture completed all of its shards. Tool calls are also not directly comparable: zero in the MiniMax M3 Seq-1-to-30 fixture (forced no-tool policy), zero in the MiniMax M3 post-fix rerun (no call exercised an allowed tool), 21 here (read-only grep and read operations under a `read_only` sandbox with `permission_default: deny`).

The companion report at [qa-sprint-39-gpt-dogfood.md](qa-sprint-39-gpt-dogfood.md) records the per-call table, the role totals, the cost comparison at OpenRouter and GPT rates, and the six permission-denied blockers that prevented issue promotion.
