# Deployment health-check failures

An image build succeeding does not establish application readiness. The
deployment script checks the app inside its container, then checks nginx and
the public containment routes. A failed check triggers restoration of the
previous image and environment.

## Connection refused after startup and rollback

Fiber uses an IPv4 listener by default. Alpine's BusyBox `wget` can resolve
`localhost` to IPv6 `::1` and fail without trying IPv4, even when the app is
healthy. The deployment script and Compose healthchecks use `127.0.0.1` to
match the listener. Internal app traffic and probes use port `4000`; the
deployment's `PORT_NUMBER` must also be `4000`.

On the deployment host, inspect the current process and probe it directly:

```bash
cd ~/autotable
docker inspect --format 'state={{.State.Status}} exit={{.State.ExitCode}} oom={{.State.OOMKilled}} restarts={{.RestartCount}}' autotable-app
docker compose exec -T app wget --spider --timeout=5 http://127.0.0.1:4000/health
docker compose exec -T app wget --spider --timeout=5 http://127.0.0.1:4000/ready
```

`/health` tests process liveness. `/ready` also checks MongoDB and Redis. The
first legacy release may only provide `/health`; the script retains that
contract when restoring it. A `503` from `/ready` means dependency readiness
failed. A connection refusal on explicit IPv4 means the app has not started
listening on that port; inspect its startup output and configuration.

## Failure diagnostics

Before replacing a failed container, the script saves its last 100 log lines
to `.deploy/<release-tag>.failed.log`. If the previous release also fails to
recover, its output is saved separately to
`.deploy/<release-tag>.rollback.log`. Both files have mode `600` inside the
mode `700` `.deploy` directory and are excluded from Git, Docker build
context, and release synchronization. The CI console prints container state
and file paths; it does not print application log contents.

For a deployment run that predates diagnostic capture, inspect
`docker compose logs --tail=100 app` on the host. Check startup errors for
missing or invalid JWT configuration, dependency connection failures, and
index initialization failures. Startup establishes required unique indexes;
resolve any reported data conflicts deliberately rather than dropping
indexes or deleting records to make startup pass.

If the app is ready but nginx still returns a gateway error after container
replacement, validate and refresh nginx's upstream address:

```bash
docker compose exec -T nginx nginx -t
docker compose exec -T nginx nginx -s reload
```

The orphan `autotable-certbot` warning does not explain an app-container
loopback connection refusal. Avoid deleting unrelated containers as a
health-check fix.
