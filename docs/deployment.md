# Deployment

## Release contents

A release archive contains:

```text
bin/seven-framework-server
configs/application.example.yaml
migrations/mysql
migrations/postgres
web/dist
deploy/nginx/seven-framework.conf.example
LICENSE
NOTICE
README.md
SHA256SUMS
sbom.spdx.json
```

Verify the archive checksum and the internal `SHA256SUMS` before installation. Copy `application.example.yaml` to `application.yaml`, inject secrets outside the archive, and run migrations before switching traffic.

## Service

Set the package root explicitly in a process manager:

```bash
/opt/seven-framework/bin/seven-framework-server serve \
  --home /opt/seven-framework
```

The same binary runs migrations and prints build identity. `version` exposes only version, commit, and build time.

## Nginx and Web

Copy `deploy/nginx/seven-framework.conf.example` and replace:

- `__WEB_LISTEN__` with a deployment listener;
- `__SERVER_NAME__` with the deployment domain;
- `__WEB_ROOT__` with the absolute `web/dist` path;
- `__SERVER_UPSTREAM__` with the local Go service host and port.

Configure TLS according to your environment. The template serves the SPA with `index.html` fallback and proxies only `/api/`. It intentionally contains no real domain, certificate path, account, key, or production port.

## Upgrade

Install a new release beside the old one, verify checksums, run migration inspection and upgrade, start the new service, confirm readiness, and atomically switch the active package path. Preserve a rollback package, but remember that forward-only database migrations may prevent application rollback after a schema change.
