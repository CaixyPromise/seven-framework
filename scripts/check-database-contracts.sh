#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root_dir/seven-framework-server"

go test ./internal/infrastructure/datasource/governance \
  -run '^TestDG0(FutureMigrationsUseLowerSnakeTablesAndRejectForeignKeys|MigrationSourcesRejectUnboundedTextIDs|MigrationTableAndForeignKeyDetectors)$' \
  -count=1
