#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/dev/init-local.sh --mysql-dsn <dsn> [options]
  scripts/dev/init-local.sh --postgres-dsn <dsn> [options]

Options:
  --redis-addr <host:port>   Redis endpoint (default: 127.0.0.1:6379)
  --rabbitmq-url <url>       Enable RabbitMQ with this URL
  --server-port <port>       API port (default: 9277)

The generated configuration and keys are stored below the ignored .local/
directory with owner-only permissions. Existing files are never overwritten.
EOF
}

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
local_root="${SEVEN_FRAMEWORK_LOCAL_DIR:-$repo_root/.local}"
config_dir="$local_root/configs"
secret_dir="$local_root/secrets"
config_file="$config_dir/application.yaml"

driver=""
dsn=""
redis_addr="127.0.0.1:6379"
rabbitmq_url=""
server_port="9277"

while (($# > 0)); do
  case "$1" in
    --mysql-dsn)
      driver="mysql"
      dsn="${2:-}"
      shift 2
      ;;
    --postgres-dsn)
      driver="postgres"
      dsn="${2:-}"
      shift 2
      ;;
    --redis-addr)
      redis_addr="${2:-}"
      shift 2
      ;;
    --rabbitmq-url)
      rabbitmq_url="${2:-}"
      shift 2
      ;;
    --server-port)
      server_port="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$driver" || -z "$dsn" ]]; then
  echo "exactly one non-empty --mysql-dsn or --postgres-dsn is required" >&2
  exit 2
fi
if [[ ! "$server_port" =~ ^[1-9][0-9]{0,4}$ ]] || ((server_port > 65535)); then
  echo "--server-port must be between 1 and 65535" >&2
  exit 2
fi
if [[ -e "$config_file" ]]; then
  echo "refusing to overwrite existing local configuration: $config_file" >&2
  exit 1
fi

command -v openssl >/dev/null 2>&1 || {
  echo "openssl is required to generate local cryptographic material" >&2
  exit 1
}

umask 077
mkdir -p "$config_dir" "$secret_dir"
master_key="$secret_dir/master.key"
private_key="$secret_dir/sso-private.pem"
public_key="$secret_dir/sso-public.pem"
setup_secret="$(openssl rand -base64 48 | tr -d '\n')"
openssl rand -base64 32 >"$master_key"
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$private_key" 2>/dev/null
openssl pkey -in "$private_key" -pubout -out "$public_key" 2>/dev/null
chmod 600 "$master_key" "$private_key" "$public_key"

yaml_quote() {
  local value="$1"
  value="${value//\'/\'\'}"
  printf "'%s'" "$value"
}

mysql_enabled="false"
postgres_enabled="false"
mysql_dsn=""
postgres_dsn=""
if [[ "$driver" == "mysql" ]]; then
  mysql_enabled="true"
  mysql_dsn="$dsn"
else
  postgres_enabled="true"
  postgres_dsn="$dsn"
fi

rabbit_enabled="false"
if [[ -n "$rabbitmq_url" ]]; then
  rabbit_enabled="true"
fi

cat >"$config_file" <<EOF
seven:
  name: seven-framework-server
  env: dev
server:
  host: 127.0.0.1
  port: $server_port
  contextPath: /api
datasource:
  driver: $driver
  bootstrap:
    enabled: true
    mode: startup
    baselineVersion: '20260422000000'
  mysql:
    enabled: $mysql_enabled
    dsn: $(yaml_quote "$mysql_dsn")
  postgres:
    enabled: $postgres_enabled
    dsn: $(yaml_quote "$postgres_dsn")
cache:
  enabled: true
  redis:
    enabled: true
    mode: single
    single:
      addr: $(yaml_quote "$redis_addr")
rabbitmq:
  enabled: $rabbit_enabled
  url: $(yaml_quote "$rabbitmq_url")
setup:
  enabled: true
  tokenSecret: $(yaml_quote "$setup_secret")
  allowedOriginPatterns:
    - http://127.0.0.1:5177
    - http://localhost:5177
login:
  enabled: true
sso:
  enabled: true
  issuer: http://127.0.0.1:$server_port/api/sso
  baseUrl: http://127.0.0.1:$server_port/api/sso
  frontendLoginUrl: http://127.0.0.1:5177/login
  jwt:
    currentKid: local-sso-v1
    privateKeysByKid:
      local-sso-v1: $(yaml_quote "file:$private_key")
    publicKeysByKid:
      local-sso-v1: $(yaml_quote "file:$public_key")
    keyStatusByKid:
      local-sso-v1: ACTIVE
externalLogin:
  enabled: false
authorization:
  internal:
    enabled: false
security:
  originPatterns:
    - http://127.0.0.1:5177
    - http://localhost:5177
  keys:
    provider: local
    master:
      active:
        kid: local-master-v1
        source: $(yaml_quote "file:$master_key")
storage:
  location: $(yaml_quote "$local_root/uploads")
EOF

chmod 600 "$config_file"
echo "Local configuration created at $config_file"
echo "Run: make -C seven-framework-server run-local"
echo "Then open http://127.0.0.1:5177/setup to create the first owner account."
