#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
version="${1:-dev}"
if [[ ! "$version" =~ ^[0-9A-Za-z][0-9A-Za-z._-]*$ ]]; then
  echo "invalid version: use only letters, numbers, dots, underscores, and hyphens" >&2
  exit 2
fi
goos="${GOOS:-$(go env GOOS)}"
goarch="${GOARCH:-$(go env GOARCH)}"
# GOOS/GOARCH describe the release target, but any Go-based build tools must
# still be installed for the host runner. The service build receives the
# target values explicitly below.
unset GOOS GOARCH
commit="${COMMIT:-$(git -C "$root_dir" rev-parse --short HEAD 2>/dev/null || echo unknown)}"
build_date="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
module_path="github.com/CaixyPromise/seven-framework/seven-framework-server"
package_name="seven-framework-server_${version}_${goos}_${goarch}"
release_root="$root_dir/dist/release"
stage="$release_root/$package_name"
archive="$release_root/$package_name.tar.gz"

if [[ "$stage" != "$release_root"/seven-framework-server_* ]]; then
  echo "refusing unsafe release stage: $stage" >&2
  exit 1
fi
if [[ -e "$stage" ]]; then
  rm -r "$stage"
fi
mkdir -p "$stage/bin" "$stage/configs" "$stage/migrations" "$stage/web" "$stage/deploy/nginx"

cd "$root_dir/seven-framework-web"
pnpm install --frozen-lockfile
pnpm audit --prod --audit-level high
pnpm exec tsc -b
pnpm lint
pnpm build

cd "$root_dir/seven-framework-server"
if ! command -v govulncheck >/dev/null 2>&1; then
  GOBIN="$root_dir/.local/bin" go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
  export PATH="$root_dir/.local/bin:$PATH"
fi
"$root_dir/scripts/check-go-vulnerabilities.sh"
CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath \
  -ldflags "-s -w -X '${module_path}/internal/shared/buildinfo.Version=${version}' -X '${module_path}/internal/shared/buildinfo.Commit=${commit}' -X '${module_path}/internal/shared/buildinfo.BuildDate=${build_date}'" \
  -o "$stage/bin/seven-framework-server" ./cmd/seven-framework-server

cp "$root_dir/seven-framework-server/configs/application.example.yaml" "$stage/configs/"
cp -R "$root_dir/seven-framework-server/migrations/mysql" "$stage/migrations/"
cp -R "$root_dir/seven-framework-server/migrations/postgres" "$stage/migrations/"
cp -R "$root_dir/seven-framework-web/dist" "$stage/web/"
cp "$root_dir/deploy/nginx/seven-framework.conf.example" "$stage/deploy/nginx/"
cp "$root_dir/LICENSE" "$root_dir/NOTICE" "$root_dir/README.md" "$stage/"

if command -v syft >/dev/null 2>&1; then
  syft scan "dir:$stage" -o "spdx-json=$stage/sbom.spdx.json"
else
  GOBIN="$root_dir/.local/bin" go install github.com/anchore/syft/cmd/syft@v1.31.0
  "$root_dir/.local/bin/syft" scan "dir:$stage" -o "spdx-json=$stage/sbom.spdx.json"
fi

"$root_dir/scripts/release/check-sensitive.sh" "$stage"

cd "$stage"
find . -type f ! -name SHA256SUMS -print | LC_ALL=C sort | while IFS= read -r file; do
  shasum -a 256 "$file"
done > SHA256SUMS
shasum -a 256 -c SHA256SUMS

cd "$release_root"
rm -f "$archive"
tar -czf "$archive" "$package_name"
shasum -a 256 "$archive" > "$archive.sha256"
echo "$archive"
