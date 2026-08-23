package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/CaixyPromise/seven-framework/seven-framework-server/internal/infrastructure/datasource"
	dsbootstrap "github.com/CaixyPromise/seven-framework/seven-framework-server/internal/infrastructure/datasource/bootstrap"
	"github.com/CaixyPromise/seven-framework/seven-framework-server/internal/shared/config"
	"github.com/CaixyPromise/seven-framework/seven-framework-server/internal/shared/logger"
	"github.com/pressly/goose/v3"
)

func main() {
	os.Exit(run())
}

func run() int {
	configDir := flag.String("config-dir", "configs", "configuration directory")
	migrationsDir := flag.String("dir", "", "goose migration directory; defaults to migrations/<datasource.driver>")
	tableName := flag.String("table", goose.DefaultTablename, "goose version table name")
	baselineVersion := flag.String("baseline-version", "", "override datasource.bootstrap.baselineVersion for inspect/bootstrap")
	allowLegacySync := flag.Bool("allow-legacy-sync", false, "temporarily allow legacy unmanaged schema sync during bootstrap")
	verbose := flag.Bool("verbose", true, "enable verbose goose output")
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/migrate [flags] <command> [args]")
		return 2
	}

	command := flag.Arg(0)
	args := flag.Args()[1:]
	cfg, err := config.Load(*configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}

	dir := *migrationsDir
	if dir == "" {
		dir = datasource.MigrationDir(cfg.Datasource.Driver)
	}
	bootstrapCfg := cfg.Datasource.Bootstrap
	if bootstrapCfg.MigrationsDir == "" {
		bootstrapCfg.MigrationsDir = dir
	}
	if flagProvided("table") {
		bootstrapCfg.VersionTable = *tableName
	}
	if flagProvided("baseline-version") {
		bootstrapCfg.BaselineVersion = *baselineVersion
	}
	if flagProvided("allow-legacy-sync") {
		bootstrapCfg.AllowLegacySync = *allowLegacySync
	}
	activeTableName := bootstrapCfg.VersionTable
	if activeTableName == "" {
		activeTableName = *tableName
	}

	switch command {
	case "create":
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "usage: go run ./cmd/migrate create <name> [sql|go]")
			return 2
		}
		migrationType := "sql"
		if len(args) > 1 {
			migrationType = args[1]
		}
		if err := goose.Create(nil, dir, args[0], migrationType); err != nil {
			fmt.Fprintf(os.Stderr, "create migration: %v\n", err)
			return 1
		}
		return 0
	case "fix":
		if err := goose.Fix(dir); err != nil {
			fmt.Fprintf(os.Stderr, "fix migrations: %v\n", err)
			return 1
		}
		return 0
	case "inspect":
		if !cfg.Datasource.Configured() {
			fmt.Fprintf(os.Stderr, "datasource.%s is not configured; set datasource.driver and matching datasource section\n", cfg.Datasource.Driver)
			return 1
		}
	case "bootstrap":
		if !cfg.Datasource.Configured() {
			fmt.Fprintf(os.Stderr, "datasource.%s is not configured; set datasource.driver and matching datasource section\n", cfg.Datasource.Driver)
			return 1
		}
		if !bootstrapCfg.ManualEnabled() {
			fmt.Fprintln(os.Stderr, "datasource bootstrap manual mode is disabled; set datasource.bootstrap.mode=manual|both and enabled=true")
			return 1
		}
	}
	if !cfg.Datasource.Configured() {
		fmt.Fprintf(os.Stderr, "datasource.%s is not configured; set datasource.driver and matching datasource section\n", cfg.Datasource.Driver)
		return 1
	}

	log, err := logger.New(cfg.Logging, cfg.Profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build logger: %v\n", err)
		return 1
	}
	defer logger.Sync(log)

	provider, err := datasource.NewProvider(cfg.Datasource, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open datasource provider: %v\n", err)
		return 1
	}
	defer provider.Close()

	bootstrapper := dsbootstrap.NewService(log)
	switch command {
	case "inspect":
		inspection, err := bootstrapper.Inspect(context.Background(), provider, bootstrapCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "inspect datasource bootstrap state: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stdout, "driver=%s state=%s version_table=%s version_table_exists=%t business_table_count=%d current_version=%d recommended_action=%s\n",
			inspection.Driver,
			inspection.State,
			inspection.VersionTable,
			inspection.VersionTableExists,
			inspection.BusinessTableCount,
			inspection.CurrentVersion,
			inspection.RecommendedAction,
		)
		return 0
	case "bootstrap":
		result, err := bootstrapper.Bootstrap(context.Background(), provider, bootstrapCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bootstrap datasource schema: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stdout, "state=%s baseline_applied=%t sync_applied=%t update_applied=%t final_version=%d\n",
			result.Inspection.State,
			result.BaselineApplied,
			result.SyncApplied,
			result.UpdateApplied,
			result.FinalVersion,
		)
		return 0
	}

	goose.SetDialect(provider.Dialect())
	goose.SetTableName(activeTableName)
	goose.SetVerbose(*verbose)

	migrationPath, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve migrations directory: %v\n", err)
		return 1
	}

	if err := goose.RunContext(context.Background(), command, provider.DB(), migrationPath, args...); err != nil {
		fmt.Fprintf(os.Stderr, "goose %s failed: %v\n", command, err)
		return 1
	}
	return 0
}

func flagProvided(name string) bool {
	provided := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			provided = true
		}
	})
	return provided
}
