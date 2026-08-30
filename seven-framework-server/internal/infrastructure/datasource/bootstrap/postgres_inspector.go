package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
)

type postgresInspectorImpl struct{}

func newPostgresInspector() *postgresInspectorImpl {
	return &postgresInspectorImpl{}
}

func (i *postgresInspectorImpl) Inspect(ctx context.Context, db *sql.DB, versionTable string) (Inspection, error) {
	if db == nil {
		return Inspection{}, fmt.Errorf("postgres datasource is not configured")
	}
	versionExists, err := postgresTableExists(ctx, db, versionTable)
	if err != nil {
		return Inspection{}, fmt.Errorf("check version table exists: %w", err)
	}
	businessTableCount, err := postgresBusinessTableCount(ctx, db, versionTable)
	if err != nil {
		return Inspection{}, fmt.Errorf("count business tables: %w", err)
	}
	inspection := Inspection{
		Driver:             "postgres",
		VersionTable:       versionTable,
		VersionTableExists: versionExists,
		BusinessTableCount: businessTableCount,
	}
	switch {
	case versionExists:
		inspection.State = SchemaStateManaged
		inspection.RecommendedAction = "run update"
		inspection.CurrentVersion, err = postgresCurrentVersion(ctx, db)
		if err != nil {
			return Inspection{}, fmt.Errorf("load current version: %w", err)
		}
	case businessTableCount == 0:
		inspection.State = SchemaStateEmpty
		inspection.RecommendedAction = "run clean baseline then update"
	default:
		inspection.State = SchemaStateLegacyUnmanaged
		inspection.RecommendedAction = "manual migration review required"
	}
	return inspection, nil
}

func postgresTableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM information_schema.tables
  WHERE table_schema = 'public' AND table_name = $1
)`, table).Scan(&exists)
	return exists, err
}

func postgresBusinessTableCount(ctx context.Context, db *sql.DB, versionTable string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM information_schema.tables
WHERE table_schema = 'public'
  AND table_type = 'BASE TABLE'
  AND table_name <> $1`, versionTable).Scan(&count)
	return count, err
}

func postgresCurrentVersion(ctx context.Context, db *sql.DB) (int64, error) {
	var version int64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_id), 0) FROM "goose_db_version"`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}
