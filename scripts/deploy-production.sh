#!/usr/bin/env bash
# Run on the deployment host. Build first, then replace the single app instance.
set -Eeuo pipefail
umask 077
cd "${AUTOTABLE_DEPLOY_DIRECTORY:-$(dirname "$0")/..}"
: "${AUTOTABLE_RELEASE_TAG:?A unique release tag is required}"
[[ "$AUTOTABLE_RELEASE_TAG" =~ ^[a-zA-Z0-9._-]+$ ]] || { echo 'Invalid release tag' >&2; exit 1; }
[[ -f .env.next ]] || { echo 'Missing staged environment' >&2; exit 1; }
chmod 600 .env.next
mkdir -p .deploy
chmod 700 .deploy
previous_image="$(docker inspect --format '{{.Image}}' autotable-app 2>/dev/null || true)"
had_environment=false
if [[ -f .env ]]; then
  cp .env .deploy/previous.env
  chmod 600 .deploy/previous.env
  had_environment=true
else
  # Compose needs its env_file even for the first build.
  cp .env.next .env
fi
switched=false
wait_ready() {
  probe_path="${1:-ready}"
  for _ in $(seq 1 30); do
    # Fiber listens on IPv4. BusyBox wget can resolve localhost to ::1 and
    # report connection refused without trying the healthy IPv4 listener.
    if docker compose exec -T app wget --quiet --spider --timeout=5 "http://127.0.0.1:4000/$probe_path"; then return 0; fi
    sleep 2
  done
  return 1
}
refresh_ingress() {
  docker compose up -d --no-deps nginx
  docker compose exec -T nginx nginx -t
  # Resolve the app's new container IP after every replacement.
  docker compose exec -T nginx nginx -s reload
}
capture_diagnostics() {
  local phase="$1"
  local diagnostic_file=".deploy/${AUTOTABLE_RELEASE_TAG}.${phase}.log"
  # Print only lifecycle fields, never the full inspect output or environment.
  docker inspect --format 'state={{.State.Status}} exit={{.State.ExitCode}} oom={{.State.OOMKilled}} restarts={{.RestartCount}}' autotable-app >&2 || true
  # App logs can contain customer data. Preserve them on the host rather than
  # sending their contents to the CI console, before recreation loses them.
  if : > "$diagnostic_file" && chmod 600 "$diagnostic_file"; then
    docker compose logs --no-color --tail=100 app > "$diagnostic_file" 2>&1 || true
    echo "Container logs saved on deployment host: $diagnostic_file" >&2
  else
    echo "Could not save container logs to $diagnostic_file" >&2
  fi
}
rollback() {
  result=$?
  trap - ERR INT TERM
  set +e
  if [[ "$switched" == true ]]; then
    capture_diagnostics failed
    echo 'Deployment failed; restoring previous image and environment.' >&2
    if [[ "$had_environment" == true ]]; then cp .deploy/previous.env .env; fi
    if [[ -n "$previous_image" ]]; then
      export AUTOTABLE_IMAGE="$previous_image"
      # The first predecessor predates /ready. Later successful releases record
      # their readiness contract so rollback retains dependency checks for them.
      previous_probe=health
      if [[ -f .deploy/ready-image && "$(cat .deploy/ready-image)" == "$previous_image" ]]; then previous_probe=ready; fi
      export AUTOTABLE_READINESS_PATH="$previous_probe"
      if docker compose up -d --no-deps --force-recreate app && wait_ready "$previous_probe" && refresh_ingress; then
        echo 'Previous release restored.' >&2
      else
        capture_diagnostics rollback
        echo 'Automatic rollback failed; operator intervention required.' >&2
      fi
    else
      echo 'No previous image exists for rollback.' >&2
    fi
  elif [[ "$had_environment" == false ]]; then
    rm -f .env
  fi
  if [[ "$result" == 0 ]]; then result=1; fi
  exit "$result"
}
trap rollback ERR INT TERM
export AUTOTABLE_IMAGE="autotable-app:$AUTOTABLE_RELEASE_TAG"
export AUTOTABLE_READINESS_PATH=ready
# A build failure leaves the running app and its environment alone.
docker compose build app
mv .env.next .env
switched=true
docker compose up -d redis
docker compose up -d --no-deps --force-recreate app
wait_ready
refresh_ingress
base_url="${AUTOTABLE_HEALTH_URL:-http://127.0.0.1}"
curl --silent --show-error --max-time 5 --retry 3 --retry-delay 1 --retry-connrefused --fail "$base_url/ready"
for path in /api/swagger /api/swagger.json /api/schemas; do
  status="$(curl --silent --show-error --max-time 5 --output /dev/null --write-out '%{http_code}' "$base_url$path")"
  if [[ "$status" != 404 ]]; then
    echo "Containment check failed for $path (HTTP $status)." >&2
    false
  fi
done
bulk_status="$(curl --silent --show-error --max-time 5 --output /dev/null --write-out '%{http_code}' \
  --request POST --header 'Content-Type: application/json' --data '[]' \
  "$base_url/api/v1/security-check/security-check/container/create-multiple")"
if [[ "$bulk_status" != 401 ]]; then
  echo "Unauthenticated bulk-write check failed (HTTP $bulk_status)." >&2
  false
fi
docker inspect --format '{{.Image}}' autotable-app > .deploy/ready-image
trap - ERR INT TERM
echo "Release $AUTOTABLE_RELEASE_TAG is ready."
