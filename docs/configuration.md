# Configuration

## Filesystem contract

The service reads `application.yaml` and an optional profile file such as `application-prod.yaml` from the resolved configuration directory. Release packages include `application.example.yaml`; copy it to `application.yaml` before starting the service.

Resource resolution is deterministic:

1. explicit `--config-dir` and `--migrations-dir`;
2. `--home`;
3. `SEVEN_FRAMEWORK_HOME`;
4. the package root inferred from the executable in `bin/`.

The service does not use an arbitrary current working directory as a release fallback. Required directories and `application.yaml` are validated before bootstrap.

## Environment variables

Configuration keys are mapped to uppercase underscore environment variables. Examples:

```text
SERVER_PORT
SERVER_CONTEXTPATH
DATASOURCE_DRIVER
DATASOURCE_MYSQL_ENABLED
DATASOURCE_MYSQL_DSN
DATASOURCE_POSTGRES_ENABLED
DATASOURCE_POSTGRES_DSN
CACHE_REDIS_PASSWORD
RABBITMQ_URL
```

Use environment variables or a deployment secret manager for passwords, DSNs, cryptographic material, OAuth credentials, and notification-provider secrets. Do not place those values in images, archives, example files, or source control.

## Safe defaults

The public example binds to loopback, disables datasource, cache, RabbitMQ, Docker, setup, and login integrations, and does not contain credentials. Enable only the capabilities whose dependencies and security keys have been configured. Production security validation remains active and can reject incomplete or unsafe values.

## Local first-run configuration

Use `scripts/dev/init-local.sh` with exactly one MySQL or PostgreSQL DSN. It
creates `.local/configs/application.yaml`, a local master key, and an RSA
signing key pair with mode `0600`; `.local/` is ignored by Git. The generated
development configuration enables migrations, Redis, setup, login, and SSO.
RabbitMQ remains disabled unless `--rabbitmq-url` is supplied.

The initializer refuses to overwrite an existing configuration and never
prints secrets. Start it with `make -C seven-framework-server run-local`, then
create the first owner through `/setup`. These development defaults are not a
production deployment template.
