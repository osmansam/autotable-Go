#!/usr/bin/env bash
# Runs only task-owned, loopback MongoDB; never connects to the app database.
set -euo pipefail
cd "$(dirname "$0")/.."
collaboration_mongod_bin="${MONGOD_BIN:-$(command -v mongod || true)}"
if [[ -z "$collaboration_mongod_bin" && -x /opt/homebrew/opt/mongodb-community@5.0/bin/mongod ]]; then
  collaboration_mongod_bin=/opt/homebrew/opt/mongodb-community@5.0/bin/mongod
fi
collaboration_mongosh_bin="${MONGO_SHELL_BIN:-${MONGOSH_BIN:-}}"
if [[ -z "$collaboration_mongosh_bin" && -x "$(dirname "$collaboration_mongod_bin")/mongo" ]]; then
  collaboration_mongosh_bin="$(dirname "$collaboration_mongod_bin")/mongo"
fi
collaboration_mongosh_bin="${collaboration_mongosh_bin:-$(command -v mongosh || true)}"
if [[ -z "$collaboration_mongod_bin" || -z "$collaboration_mongosh_bin" ]]; then
  echo "mongod and a mongo/mongosh shell are required (override MONGOD_BIN/MONGO_SHELL_BIN)." >&2
  exit 1
fi
collaboration_mongo_dir="$(mktemp -d "${TMPDIR:-/tmp}/autotable-collaboration-mongo.XXXXXX")"
collaboration_mongo_pid=""
cleanup() {
  if [[ -n "$collaboration_mongo_pid" ]]; then
    kill "$collaboration_mongo_pid" 2>/dev/null || true
    wait "$collaboration_mongo_pid" 2>/dev/null || true
  fi
  rm -rf -- "$collaboration_mongo_dir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
collaboration_mongo_port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
"$collaboration_mongod_bin" --dbpath "$collaboration_mongo_dir" --bind_ip 127.0.0.1 --port "$collaboration_mongo_port" --replSet collaboration-integration --logpath "$collaboration_mongo_dir/mongod.log" >"$collaboration_mongo_dir/process.log" 2>&1 &
collaboration_mongo_pid=$!
collaboration_mongo_direct="mongodb://127.0.0.1:$collaboration_mongo_port/?directConnection=true&serverSelectionTimeoutMS=1000"
collaboration_mongo_ready=false
for _ in $(seq 1 40); do
  if "$collaboration_mongosh_bin" "$collaboration_mongo_direct" --quiet --eval 'quit(db.adminCommand({ping:1}).ok === 1 ? 0 : 1)' >/dev/null 2>&1; then collaboration_mongo_ready=true; break; fi
  if ! kill -0 "$collaboration_mongo_pid" 2>/dev/null; then cat "$collaboration_mongo_dir/process.log" >&2; exit 1; fi
  sleep 0.25
done
if [[ "$collaboration_mongo_ready" != true ]]; then cat "$collaboration_mongo_dir/mongod.log" >&2; exit 1; fi
# A free-port probe has a short bind race. A responding server is never
# mutated until its PID and exact data path prove it is this task's process.
if ! kill -0 "$collaboration_mongo_pid" 2>/dev/null; then cat "$collaboration_mongo_dir/process.log" >&2; exit 1; fi
collaboration_mongo_expected_path="$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$collaboration_mongo_dir")"
"$collaboration_mongosh_bin" "$collaboration_mongo_direct" --quiet --eval "
  const cmdline = db.adminCommand({getCmdLineOpts:1});
  const server = db.adminCommand({serverStatus:1});
  if (!cmdline.ok || !server.ok || !cmdline.parsed.storage ||
      cmdline.parsed.storage.dbPath !== $collaboration_mongo_expected_path ||
      Number(server.pid) !== $collaboration_mongo_pid) {
    throw new Error('Refusing to initialize MongoDB: task process ownership mismatch');
  }
  const initiated = rs.initiate({_id:'collaboration-integration',members:[{_id:0,host:'127.0.0.1:$collaboration_mongo_port'}]});
  if (!initiated.ok) { throw new Error('Task replica-set initialization failed'); }
" >/dev/null
collaboration_mongo_ready=false
for _ in $(seq 1 60); do
  if "$collaboration_mongosh_bin" "$collaboration_mongo_direct" --quiet --eval 'quit(db.hello().isWritablePrimary ? 0 : 1)' >/dev/null 2>&1; then collaboration_mongo_ready=true; break; fi
  sleep 0.25
done
if [[ "$collaboration_mongo_ready" != true ]]; then cat "$collaboration_mongo_dir/mongod.log" >&2; exit 1; fi
"$collaboration_mongod_bin" --version | head -1
export AUTOTABLE_COLLABORATION_MONGO_URI="mongodb://127.0.0.1:$collaboration_mongo_port/?replicaSet=collaboration-integration"
go test -tags=integration ./repositories ./services ./controllers ./routes ./internal/collaborationmigration -run '^TestCollaboration' -count=1 -v "$@"
