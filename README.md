# Seven Framework

Seven Framework is an open-source administration platform with a Go service and a React Web application. The service provides authentication, authorization, configuration, dictionaries, files, notifications, audit logs, and operational management through a domain-oriented architecture.

Repository: `github.com/CaixyPromise/seven-framework`

## Repository layout

```text
seven-framework-server/  Go API service, migrations, and tests
seven-framework-web/     React and Vite Web application
deploy/nginx/            Deployment template for Web and API routing
scripts/release/         Reproducible release-package assembly
docs/                    Architecture, configuration, migration, and deployment guides
```

The Web project is private package metadata and is not published to npm. Official release artifacts contain one `seven-framework-server` executable together with the built Web assets, database migrations, example configuration, Nginx template, checksums, and an SPDX SBOM.

## Prerequisites

- Go 1.25 or newer
- Node.js 20.19 or newer
- pnpm 10
- MySQL 8 or PostgreSQL 16 for persistent storage
- Redis and RabbitMQ only when the corresponding optional features are enabled

## Source development

Create a complete local configuration without committing it. The command
generates owner-only local keys, enables setup/login/SSO and Redis, and never
overwrites an existing `.local` configuration:

```bash
scripts/dev/init-local.sh \
  --mysql-dsn 'user:password@tcp(127.0.0.1:3306)/seven_framework?parseTime=true'
```

PostgreSQL is also supported through `--postgres-dsn`. Pass
`--rabbitmq-url` only when local notification/outbox consumption is required.
The public example intentionally remains safe-off.

For manually managed configuration, set sensitive values through environment
variables. Viper maps configuration keys to uppercase underscore names; for example:

```bash
export DATASOURCE_MYSQL_ENABLED=true
export DATASOURCE_MYSQL_DSN='user:password@tcp(127.0.0.1:3306)/seven_framework?parseTime=true'
```

Start the API and Web development servers:

```bash
make -C seven-framework-server run-local
pnpm --dir seven-framework-web install --frozen-lockfile
pnpm --dir seven-framework-web dev
```

The development Web server listens on `127.0.0.1:5177` and proxies `/api` to `127.0.0.1:9277` by default. Override the development target with `VITE_DEV_API_TARGET`.

## Service commands

```text
seven-framework-server serve
seven-framework-server migrate <up|down|status|version|inspect|bootstrap|create|fix>
seven-framework-server version
```

`serve` is the default command. In release packages, resource lookup uses this precedence:

1. `--config-dir` and `--migrations-dir`;
2. `--home`;
3. `SEVEN_FRAMEWORK_HOME`;
4. the release root inferred from `bin/seven-framework-server`.

Missing configuration or migration directories cause startup to fail instead of falling back to the current working directory.

## Build and test

```bash
make web-install
make check
make release VERSION=0.1.0
```

See [configuration](docs/configuration.md), [database migration](docs/migration.md), [deployment](docs/deployment.md), and [architecture](docs/architecture.md) for the operational contract.

## License

Copyright CaixyPromise. Licensed under the [Apache License 2.0](LICENSE).
