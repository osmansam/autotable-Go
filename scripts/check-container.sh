#!/usr/bin/env bash
# Local/CI image checks only. Never starts the API or changes host ownership.
set -euo pipefail
cd "$(dirname "$0")/.."
check_mode="${1:-image}"
check_image="${AUTOTABLE_CHECK_IMAGE:-autotable:security-check}"
trivy_image='aquasec/trivy:0.76.2@sha256:8f6bb947dbc528902e11f2f5be52f7b519f6a39d5d148f41166f2040499768fb'

case "$check_mode" in
  image|scan) ;;
  mounts)
    for staging_directory in "${STAGING_TEMP_DIR:-}" "${STAGING_LOG_DIR:-}"; do
      if [[ -z "$staging_directory" || ! -d "$staging_directory" || "$staging_directory" != /* ]]; then
        echo 'STAGING_TEMP_DIR and STAGING_LOG_DIR must name explicit existing absolute staging directories.' >&2
        exit 2
      fi
    done
    ;;
  *) echo 'Usage: bash scripts/check-container.sh image|scan|mounts' >&2; exit 2 ;;
esac

if ! docker info >/dev/null 2>&1; then
  echo 'Docker engine unavailable: image/runtime/OS checks have NOT passed. Start an appropriate local test engine or use CI.' >&2
  exit 2
fi

case "$check_mode" in
  image)
    docker build --platform linux/amd64 -t "$check_image" .
    docker run --rm --network none --entrypoint /bin/sh "$check_image" -ec '
      test "$(id -u)" = "10001"
      test "$(id -g)" = "10001"
      test -w /app/temp && test -x /app/temp
      test -w /app/logs && test -x /app/logs
      test -x /app/server && test ! -w /app/server
      test -r /app/configs/production.json && test ! -w /app/configs/production.json
      touch /app/temp/ci-probe /app/logs/ci-probe
    '
    echo 'Image built and isolated non-root runtime permissions passed.'
    ;;
  scan)
    report_directory="${AUTOTABLE_CHECK_REPORT_DIR:-$PWD/build}"
    mkdir -p "$report_directory"
    report_directory="$(cd "$report_directory" && pwd -P)"
    mkdir -p "$report_directory/trivy-cache"
    archive_directory="$(mktemp -d "$report_directory/image-archive.XXXXXX")"
    trap 'rm -rf -- "$archive_directory"' EXIT
    docker save --output "$archive_directory/image.tar" "$check_image"
    rm -f -- "$report_directory/os-vulnerabilities.json"
    # Export an archive rather than granting the scanner the Docker daemon socket.
    # No ignore file, ignore-unfixed option, env secrets or application process.
    docker run --rm --user "$(id -u):$(id -g)" \
      -v "$archive_directory/image.tar:/scan/image.tar:ro" \
      -v "$report_directory/trivy-cache:/cache" \
      -v "$report_directory:/reports" \
      "$trivy_image" image --input /scan/image.tar --cache-dir /cache \
      --scanners vuln --pkg-types os --severity HIGH,CRITICAL \
      --exit-code 1 --exit-on-eol 1 --ignorefile /dev/null --disable-telemetry \
      --format json --output /reports/os-vulnerabilities.json
    python3 - "$report_directory/os-vulnerabilities.json" <<'PYREPORT'
import json,sys
with open(sys.argv[1]) as source:
    report=json.load(source)
if not report.get('Metadata', {}).get('OS', {}).get('Family'):
    sys.exit('OS scan report lacks distribution metadata; gate has NOT passed')
results=[row for row in report.get('Results', []) if row.get('Class')=='os-pkgs']
if not results:
    sys.exit('OS scan report contains no OS package result; gate has NOT passed')
if any(v.get('Severity') in ('HIGH','CRITICAL') for row in results for v in row.get('Vulnerabilities') or []):
    sys.exit('OS scan report contains HIGH/CRITICAL findings; gate has NOT passed')
PYREPORT
    echo 'OS HIGH/CRITICAL vulnerability gate passed; JSON report saved under build/ or AUTOTABLE_CHECK_REPORT_DIR.'
    ;;
  mounts)
    # Metadata/ACL access check only: no mkdir, chown, probe files or app startup.
    docker run --rm --network none --entrypoint /bin/sh \
      --mount "type=bind,src=$STAGING_TEMP_DIR,dst=/app/temp" --mount "type=bind,src=$STAGING_LOG_DIR,dst=/app/logs" \
      "$check_image" -ec '
        test "$(id -u)" = "10001" && test "$(id -g)" = "10001"
        test -w /app/temp && test -x /app/temp
        test -w /app/logs && test -x /app/logs
      '
    echo 'Staging mount access passed for UID/GID 10001; real upload/log acceptance is still required.'
    ;;
esac
