# Product-state SQLite migration

UltraPlan can store mutable study and sprint execution state in the workspace
database instead of rewriting complete JSON documents after each transition.

Preview the migration:

```sh
scripts/migrate-product-state.sh /path/to/workspace --dry-run
```

Import every valid state artifact:

```sh
scripts/migrate-product-state.sh /path/to/workspace
```

The command discovers study `run-state.json`, sprint `flow-state.json`, and
sprint `.run-state.json` files. It validates each file with the existing product
validator before importing it. Invalid files remain unchanged and produce a
partial-failure exit status. Re-running the command skips records already in the
database.

After import, SQLite is authoritative for that record. Existing JSON files stay
in place as compatibility checkpoints. Unmigrated records remain file-backed,
so migration can be performed incrementally.

The normalized tables are `product_states` and `product_state_items`. Study
tasks, sprint stages, and sprint execute tasks occupy separate ordered rows.
Large generated reports, prompts, and diagnostics remain files.
