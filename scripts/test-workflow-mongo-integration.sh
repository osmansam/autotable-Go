#!/usr/bin/env bash
# Runs only task-owned, loopback MongoDB; never connects to the app database.
set -euo pipefail
cd "$(dirname "$0")/.."
workflow_mongod_bin="${MONGOD_BIN:-$(command -v mongod || true)}"
if [[ -z "$workflow_mongod_bin" && -x /opt/homebrew/opt/mongodb-community@5.0/bin/mongod ]]; then
  workflow_mongod_bin=/opt/homebrew/opt/mongodb-community@5.0/bin/mongod
fi
workflow_mongosh_bin="${MONGO_SHELL_BIN:-${MONGOSH_BIN:-}}"
if [[ -z "$workflow_mongosh_bin" && -x "$(dirname "$workflow_mongod_bin")/mongo" ]]; then
  workflow_mongosh_bin="$(dirname "$workflow_mongod_bin")/mongo"
fi
workflow_mongosh_bin="${workflow_mongosh_bin:-$(command -v mongosh || true)}"
if [[ -z "$workflow_mongod_bin" || -z "$workflow_mongosh_bin" ]]; then
  echo "mongod and a mongo/mongosh shell are required (override MONGOD_BIN/MONGO_SHELL_BIN)." >&2
  exit 1
fi
workflow_mongo_dir="$(mktemp -d "${TMPDIR:-/tmp}/autotable-workflow-mongo.XXXXXX")"
workflow_mongo_pid=""
cleanup() {
  if [[ -n "$workflow_mongo_pid" ]]; then
    kill "$workflow_mongo_pid" 2>/dev/null || true
    wait "$workflow_mongo_pid" 2>/dev/null || true
  fi
  rm -rf -- "$workflow_mongo_dir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
workflow_mongo_port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
"$workflow_mongod_bin" --dbpath "$workflow_mongo_dir" --bind_ip 127.0.0.1 --port "$workflow_mongo_port" --replSet workflow-integration --logpath "$workflow_mongo_dir/mongod.log" >"$workflow_mongo_dir/process.log" 2>&1 &
workflow_mongo_pid=$!
workflow_mongo_direct="mongodb://127.0.0.1:$workflow_mongo_port/?directConnection=true&serverSelectionTimeoutMS=1000"
workflow_mongo_ready=false
for _ in $(seq 1 40); do
  if "$workflow_mongosh_bin" "$workflow_mongo_direct" --quiet --eval 'quit(db.adminCommand({ping:1}).ok === 1 ? 0 : 1)' >/dev/null 2>&1; then workflow_mongo_ready=true; break; fi
  if ! kill -0 "$workflow_mongo_pid" 2>/dev/null; then cat "$workflow_mongo_dir/process.log" >&2; exit 1; fi
  sleep 0.25
done
if [[ "$workflow_mongo_ready" != true ]]; then cat "$workflow_mongo_dir/mongod.log" >&2; exit 1; fi
# A free-port probe has a short bind race. A responding server is never
# mutated until its PID and exact data path prove it is this task's process.
if ! kill -0 "$workflow_mongo_pid" 2>/dev/null; then cat "$workflow_mongo_dir/process.log" >&2; exit 1; fi
workflow_mongo_expected_path="$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$workflow_mongo_dir")"
"$workflow_mongosh_bin" "$workflow_mongo_direct" --quiet --eval "
  const cmdline = db.adminCommand({getCmdLineOpts:1});
  const server = db.adminCommand({serverStatus:1});
  if (!cmdline.ok || !server.ok || !cmdline.parsed.storage ||
      cmdline.parsed.storage.dbPath !== $workflow_mongo_expected_path ||
      Number(server.pid) !== $workflow_mongo_pid) {
    throw new Error('Refusing to initialize MongoDB: task process ownership mismatch');
  }
  const initiated = rs.initiate({_id:'workflow-integration',members:[{_id:0,host:'127.0.0.1:$workflow_mongo_port'}]});
  if (!initiated.ok) { throw new Error('Task replica-set initialization failed'); }
" >/dev/null
workflow_mongo_ready=false
for _ in $(seq 1 60); do
  if "$workflow_mongosh_bin" "$workflow_mongo_direct" --quiet --eval 'quit(db.hello().isWritablePrimary ? 0 : 1)' >/dev/null 2>&1; then workflow_mongo_ready=true; break; fi
  sleep 0.25
done
if [[ "$workflow_mongo_ready" != true ]]; then cat "$workflow_mongo_dir/mongod.log" >&2; exit 1; fi
"$workflow_mongod_bin" --version | head -1
export AUTOTABLE_WORKFLOW_MONGO_URI="mongodb://127.0.0.1:$workflow_mongo_port/?replicaSet=workflow-integration"
go test -tags=integration ./services -run '^TestWorkflowMongo' -count=1 -v "$@"
