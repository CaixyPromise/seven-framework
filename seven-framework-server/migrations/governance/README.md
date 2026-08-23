# DG0/DG4 table-rename governance registry

`table_registry.csv` is the frozen legacy-table checkpoint. It records every
physical table present through migration `20260730160000`. Tables added by
later DG phases are owned by those phases and must be appended at their
checkpoint.

Each DG4 batch is an in-place rename rather than a compatibility migration:

1. use the existing repository, Facade, or controlled HTTP call to create a
   minimal fixture and retain its ID and observable values;
2. rename the batch tables in place and replace all Go, SQL, job, listener,
   seed, command, and script references in the same controlled release;
3. use the same call to read the old fixture, update it where supported, and
   create a new one; direct checks verify the new table name, absence of the
   old name, and preservation of rows, primary keys, indexes, and sequences.

No batch may add dual-write, copying, backfill, or union reads. Migrate first,
then start the new application: an old process must not remain live after its
table has been renamed. If a post-rename call fails, repair and redeploy
forward against the current schema; operators must not run a destructive Goose
`Down`. Keep a database restore point and the previous application artifact
until the batch checkpoint passes.

The registry records a direct lifecycle rather than a compatibility phase:
`rename`, `verify`, and `release`. Unchanged snake_case tables are `no_op`;
pending mixed-name tables remain `planned / blocked / blocked`; a completed
batch is `renamed / passed / complete` only after both isolated databases have
passed its before-and-after business-call evidence.

## B1–B3 staged rename acceptance

`scripts/audit/run_dg4_staged_rename_acceptance.sh` is the reproducible
acceptance runner for the completed SSO/platform, external identity/federation,
and notification batches. It creates only temporary historical source
snapshots so the same repository/Client calls can write records before each
rename, then read, update, and create records after it. It never introduces a
runtime compatibility mode or writes outside the two exact isolated governance
databases guarded by the Go tests. Operators provide the two local DSNs only at
execution time; credentials are not stored in the script or evidence.

## DG0 forward migration contract

Migrations at or before `20260730160000` are historical. Their mixed-case
physical table names remain usable only because they are explicitly listed in
`table_registry.csv`; the registry is the migration plan from each current
name to its lower snake_case target. DG0 does not rewrite, rename, or execute
those migrations.

Every later Go-owned MySQL or PostgreSQL migration is checked before merge:

- a newly created physical table must already use lower snake_case;
- every newly created table or rename target must be present in
  `table_registry.csv` (a legacy rename target) or in the separately reviewed
  `future_table_registry.csv`; an empty future registry is intentional until a
  new physical table is approved;
- a table rename target must use lower snake_case;
- a new `FOREIGN KEY` or inline `REFERENCES` declaration is forbidden.
  Relationship integrity belongs to the application boundary, not database
  foreign keys;
- existing column names are frozen: `RENAME COLUMN` is forbidden. DG4
  will verify column signatures and indexes as part of each reviewed table
  migration batch;
- the historical foreign-key removal migration is allowed because it is at the
  frozen checkpoint and removes rather than creates a constraint.

`TestDG0FutureMigrationsUseLowerSnakeTablesAndRejectForeignKeys` enforces this
forward-only rule. It intentionally scans only versioned source migrations,
not generated historical baselines.

## Command SQL dialect contracts

Every production Go source file below `cmd/` containing a raw SQL statement
must begin with exactly one source-controlled declaration:

```go
// sql-governance: postgres-capable
```

or:

```go
// sql-governance: mysql-only
```

`postgres-capable` commands must render all mixed-case identifiers through an
explicit reviewed renderer and rebind placeholders for the active provider.
`mysql-only` commands must reject a non-MySQL datasource before they create or
change any data. `TestDG1CommandRawSQLDeclaresDialectContract` enforces the
declaration for every non-test command source so new operational SQL cannot
silently bypass the dual-database contract.

## Operational SQL manifest

`operational_sql_manifest.csv` is the bounded inventory for raw SQL outside
repositories: production commands, future job/listener sources, and executable
`scripts/` or `script/` sources. Every SQL-bearing source must have exactly one
manifest row with a dialect and `registry` table contract. Go command/job/
listener sources also retain the source-level `sql-governance:` declaration;
the checker requires it to match the manifest.

The existing audit scripts are deliberately `mysql-only`: they drive local
MySQL federation fixtures and use MySQL syntax. They remain listed rather than
being silently treated as PostgreSQL-capable. Pure unit-test/assertion scripts
are excluded only when their name identifies them as tests and they do not
invoke a database client; a test script that invokes a database process remains
in the inventory.
