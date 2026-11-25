"""Black-box deployment checks; lifecycle commands are fakes, Compose parsing is read-only."""
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import tempfile
import textwrap
import unittest

ROOT = Path(__file__).resolve().parents[1]


class DeploymentEnvironmentTests(unittest.TestCase):
    def render_environment(self, credential_key, limiter_secret="", limiter_password="", previous_secret="", collaboration=None):
        workflow = (ROOT / ".github/workflows/production.yml").read_text()
        step = workflow.split("      - name: Create .env file\n", 1)[1].split("\n      - name:", 1)[0]
        bindings, script = step.split("        run: |\n", 1)
        secrets = {
            "JWT_SECRET": "synthetic-admin-secret",
            "TENANT_JWT_SECRET": "synthetic-tenant-secret",
            "EXTERNAL_API_CREDENTIAL_KEY": credential_key,
            "OZMISHOW_RATE_LIMIT_SERVICE_SECRET": limiter_secret,
            "OZMISHOW_RATE_LIMIT_REDIS_PASSWORD": limiter_password,
            "OZMISHOW_RATE_LIMIT_PREVIOUS_SECRET": previous_secret,
        }
        secrets.update({key: "" for key in (
            "COLLABORATION_MEMBERS_ENABLED", "COLLABORATION_INVITATIONS_ENABLED", "COLLABORATION_RECOVERY_ENABLED",
            "INVITATION_SMTP_HOST", "INVITATION_SMTP_PORT", "INVITATION_SMTP_TLS_MODE", "INVITATION_SMTP_FROM",
            "INVITATION_SMTP_USERNAME", "INVITATION_SMTP_PASSWORD", "INVITATION_PANEL_URL",
            "INVITATION_DELIVERY_KEYS_JSON", "INVITATION_DELIVERY_ACTIVE_KEY_ID",
        )})
        secrets.update(collaboration or {})
        environment = {"PATH": os.environ["PATH"]}
        for variable, secret in re.findall(r"(DEPLOY_\w+): \$\{\{ secrets\.(\w+) \}\}", bindings):
            environment[variable] = secrets.get(secret, "synthetic-value")
        with tempfile.TemporaryDirectory() as temporary:
            result = subprocess.run(
                ["bash", "-e", "-c", textwrap.dedent(script)],
                cwd=temporary, env=environment, capture_output=True, text=True,
            )
            path = Path(temporary) / ".env"
            content = path.read_text() if path.exists() else ""
            return result, content

    def test_external_credential_key_reaches_deployed_environment(self):
        key = "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI="
        result, content = self.render_environment(key)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("EXTERNAL_API_CREDENTIAL_KEY=" + key + "\n", content)
        self.assertNotIn(key, result.stdout + result.stderr)

    def test_limiter_activation_requires_its_own_redis_password(self):
        result, _ = self.render_environment("key", "s" * 40, "")
        self.assertNotEqual(result.returncode, 0)

    def test_limiter_credentials_and_optional_compose_reach_environment(self):
        result, content = self.render_environment("key", "s" * 40, "r" * 40, "p" * 40)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("OZMISHOW_RATE_LIMIT_SERVICE_SECRET=" + "s" * 40 + "\n", content)
        self.assertIn("OZMISHOW_RATE_LIMIT_REDIS_PASSWORD=" + "r" * 40 + "\n", content)
        self.assertIn("OZMISHOW_RATE_LIMIT_PREVIOUS_SECRET=" + "p" * 40 + "\n", content)
        self.assertIn("COMPOSE_FILE=docker-compose.yml:docker-compose.ozmishow.yml\n", content)
        self.assertNotIn("s" * 40, result.stdout + result.stderr)

    def test_unconfigured_limiter_keeps_base_compose_only(self):
        result, content = self.render_environment("key")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("COMPOSE_FILE=docker-compose.yml\n", content)

    def test_missing_external_credential_key_stops_environment_generation(self):
        result, content = self.render_environment("")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(content, "")

    def test_collaboration_secrets_reach_the_container_environment_without_shell_expansion(self):
        settings = {
            "COLLABORATION_MEMBERS_ENABLED": "true", "COLLABORATION_INVITATIONS_ENABLED": "true",
            "COLLABORATION_RECOVERY_ENABLED": "false", "INVITATION_SMTP_HOST": "smtp.example.test",
            "INVITATION_SMTP_PORT": "587", "INVITATION_SMTP_TLS_MODE": "starttls",
            "INVITATION_SMTP_FROM": "AutoTable <invite@example.test>", "INVITATION_SMTP_USERNAME": "sender",
            "INVITATION_SMTP_PASSWORD": "synthetic$PASSWORD # 'quoted' \\\" `echo secret` $(echo secret)",
            "INVITATION_PANEL_URL": "https://tenant.autoapi.org",
            "INVITATION_DELIVERY_KEYS_JSON": '{"test":"MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI="}',
            "INVITATION_DELIVERY_ACTIVE_KEY_ID": "test",
        }
        result, content = self.render_environment("key", collaboration=settings)
        self.assertEqual(result.returncode, 0, result.stderr)
        # Compose's double-quoted env-file syntax: JSON escapes, literal dollars as $$.
        rendered = dict(line.split("=", 1) for line in content.splitlines())
        for key, expected in settings.items():
            self.assertIn(key, rendered)
            self.assertEqual(json.loads(rendered[key]).replace("$$", "$"), expected)
            self.assertNotIn(expected, result.stdout + result.stderr)
        self.assertIn("$$PASSWORD", rendered["INVITATION_SMTP_PASSWORD"])

    @unittest.skipUnless(shutil.which("docker"), "Docker Compose is not installed")
    def test_compose_preserves_invitation_credentials_and_json(self):
        probe = subprocess.run(["docker", "compose", "version"], capture_output=True)
        if probe.returncode:
            self.skipTest("Docker Compose plugin is not installed")
        result, content = self.render_environment("key", collaboration={
            "COLLABORATION_MEMBERS_ENABLED": "true",
            "INVITATION_SMTP_PASSWORD": "synthetic$PASSWORD # 'quoted' \\",
            "INVITATION_DELIVERY_KEYS_JSON": '{"test":"synthetic-key="}',
        })
        self.assertEqual(result.returncode, 0, result.stderr)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / ".env").write_text(content)
            (root / "compose.yml").write_text("services:\n  app:\n    image: scratch\n    env_file: .env\n")
            parsed = subprocess.run(["docker", "compose", "-f", "compose.yml", "config", "--format", "json"], cwd=root, capture_output=True, text=True)
            self.assertEqual(parsed.returncode, 0, parsed.stderr)
            environment = json.loads(parsed.stdout)["services"]["app"]["environment"]
            self.assertEqual(environment["COLLABORATION_MEMBERS_ENABLED"], "true")
            self.assertEqual(environment["INVITATION_DELIVERY_KEYS_JSON"], '{"test":"synthetic-key="}')
            # Compose serializes literal dollar signs as $$ in its resolved model.
            self.assertEqual(environment["INVITATION_SMTP_PASSWORD"], "synthetic$$PASSWORD # 'quoted' \\")

    def test_unconfigured_collaboration_flags_default_to_disabled(self):
        result, content = self.render_environment("key")
        self.assertEqual(result.returncode, 0, result.stderr)
        for name in ("MEMBERS", "INVITATIONS", "RECOVERY"):
            self.assertIn(f'COLLABORATION_{name}_ENABLED="false"\n', content)

    def test_collaboration_secret_cannot_inject_environment_lines(self):
        result, _ = self.render_environment("key", collaboration={"INVITATION_SMTP_PASSWORD": "private\nCOLLABORATION_MEMBERS_ENABLED=true"})
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("private", result.stdout + result.stderr)


class DeploymentTests(unittest.TestCase):
    def deploy(self, fail_build=False, fail_readiness=False, fail_security=False, previous_has_ready=False, ipv4_only=False, fail_rollback=False, limiter=False, previous_limiter=False, fail_limiter_probe=False, fail_prune=False, cleanup="1", keep_storage=""):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / ".env").write_text("VALUE=old\n" + ("COMPOSE_FILE=docker-compose.yml:docker-compose.ozmishow.yml\n" if previous_limiter else ""))
            (root / ".env.next").write_text("VALUE=new\n" + ("COMPOSE_FILE=docker-compose.yml:docker-compose.ozmishow.yml\n" if limiter else ""))
            if previous_has_ready:
                (root / ".deploy").mkdir()
                (root / ".deploy/ready-image").write_text("sha256:previous\n")
            binaries = root / "bin"
            binaries.mkdir()
            docker = binaries / "docker"
            docker.write_text("""#!/usr/bin/env python3
import os,sys
from pathlib import Path
args=sys.argv[1:]
with open(os.environ['MOCK_LOG'],'a') as log: log.write(os.environ.get('AUTOTABLE_IMAGE','')+' '+ ' '.join(args)+'\\n')
if args[:3]==['compose','config','--services']:
    print('app\\nredis\\nnginx'+('\\nozmishow-redis' if 'docker-compose.ozmishow.yml' in Path('.env').read_text() else ''));sys.exit(0)
if args[:2]==['builder','prune']:
    if os.environ.get('FAIL_PRUNE')=='1':
        print('SYNTHETIC_PRIVATE_CACHE_ERROR',file=sys.stderr);sys.exit(1)
    print('Total reclaimed space: 42GB');sys.exit(0)
if args[:1]==['inspect']:
    if '.State.Status' in args[2]:
        print('state=running exit=0 oom=false restarts=0');sys.exit(0)
    print('sha256:previous');sys.exit(0)
if 'logs' in args:
    print('Startup diagnostic for '+os.environ.get('AUTOTABLE_IMAGE','')+' SYNTHETIC_PRIVATE_LOG');sys.exit(0)
if 'build' in args and os.environ.get('FAIL_BUILD')=='1':sys.exit(1)
if 'up' in args and 'app' in args:
    expected='health' if os.environ.get('AUTOTABLE_IMAGE')=='sha256:previous' and os.environ.get('PREVIOUS_HAS_READY')!='1' else 'ready'
    if os.environ.get('AUTOTABLE_READINESS_PATH')!=expected:sys.exit(1)
if args[:6]==['compose','exec','-T','app','sh','-ec'] and '/internal/ozmishow/rate-limit' in args[-1]:
    if os.environ.get('FAIL_LIMITER_PROBE')=='1' and os.environ.get('AUTOTABLE_IMAGE')!='sha256:previous':sys.exit(1)
    print('{"allowed":true,"retryAfterSeconds":0}');sys.exit(0)
if 'wget' in args:
    # BusyBox may resolve localhost to ::1; Fiber's default listener is IPv4.
    if os.environ.get('IPV4_ONLY')=='1' and not args[-1].startswith('http://127.0.0.1:'):sys.exit(1)
    if os.environ.get('AUTOTABLE_IMAGE')=='sha256:previous' and args[-1].endswith('/ready') and os.environ.get('PREVIOUS_HAS_READY')!='1':sys.exit(1)
    if os.environ.get('FAIL_READINESS')=='1' and os.environ.get('AUTOTABLE_IMAGE')!='sha256:previous':sys.exit(1)
    if os.environ.get('FAIL_ROLLBACK')=='1' and os.environ.get('AUTOTABLE_IMAGE')=='sha256:previous':sys.exit(1)
""")
            docker.chmod(0o755)
            curl = binaries / "curl"
            curl.write_text("""#!/usr/bin/env python3
import os,sys
url=sys.argv[-1]
if '/api/swagger' in url or '/api/schemas' in url: print('200' if os.environ.get('FAIL_SECURITY')=='1' else '404',end='')
elif '/internal/ozmishow/rate-limit' in url: print('401',end='')
elif '/container/create-multiple' in url: print('401',end='')
""")
            curl.chmod(0o755)
            sleep = binaries / "sleep"
            sleep.write_text("#!/bin/sh\nexit 0\n")
            sleep.chmod(0o755)
            environment = {**os.environ, "PATH": str(binaries) + os.pathsep + os.environ["PATH"], "AUTOTABLE_RELEASE_TAG": "test-release", "AUTOTABLE_DEPLOY_DIRECTORY": str(root), "MOCK_LOG": str(root / "commands.log"), "FAIL_BUILD": str(int(fail_build)), "FAIL_READINESS": str(int(fail_readiness)), "FAIL_SECURITY":str(int(fail_security)), "PREVIOUS_HAS_READY":str(int(previous_has_ready)), "IPV4_ONLY":str(int(ipv4_only)), "FAIL_ROLLBACK":str(int(fail_rollback)), "FAIL_LIMITER_PROBE":str(int(fail_limiter_probe)), "FAIL_PRUNE":str(int(fail_prune)), "AUTOTABLE_BUILD_CACHE_CLEANUP":cleanup, "AUTOTABLE_BUILD_CACHE_KEEP_STORAGE":keep_storage}
            result = subprocess.run(["bash", str(ROOT / "scripts/deploy-production.sh")], env=environment, capture_output=True, text=True)
            commands = (root / "commands.log").read_text() if (root / "commands.log").exists() else ""
            diagnostics = {
                path.name: (path.read_text(), stat.S_IMODE(path.stat().st_mode))
                for path in (root / ".deploy").glob("test-release.*.log")
            }
            cache_log = root / ".deploy/build-cache-cleanup.log"
            result.cache_cleanup = (cache_log.read_text(), stat.S_IMODE(cache_log.stat().st_mode)) if cache_log.exists() else None
            return result, commands, (root / ".env").read_text(), diagnostics

    def test_success_prunes_only_unused_build_cache_after_release_checks(self):
        result, commands, _, _ = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("builder prune --all --force --keep-storage 5GB", commands)
        self.assertGreater(commands.index("builder prune"), commands.index("nginx nginx -s reload"))
        self.assertGreater(commands.index("builder prune"), commands.rindex("inspect --format {{.Image}}"))
        for forbidden in ["system prune", "image prune", "volume prune", "container prune"]:
            self.assertNotIn(forbidden, commands)
        self.assertIn("Total reclaimed space: 42GB", result.stdout)

    def test_cache_cleanup_failure_keeps_healthy_release_successful(self):
        result, commands, environment, _ = self.deploy(fail_prune=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("builder prune", commands)
        self.assertIn("cache cleanup failed", result.stderr.lower())
        self.assertNotIn("Previous release restored", result.stderr)
        self.assertNotIn("SYNTHETIC_PRIVATE_CACHE_ERROR", result.stdout + result.stderr)
        self.assertEqual(environment, "VALUE=new\n")
        self.assertIsNotNone(result.cache_cleanup)
        self.assertIn("SYNTHETIC_PRIVATE_CACHE_ERROR", result.cache_cleanup[0])
        self.assertEqual(result.cache_cleanup[1], 0o600)

    def test_cache_cleanup_can_be_disabled(self):
        result, commands, _, _ = self.deploy(cleanup="0")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("builder prune", commands)

    def test_cache_retention_budget_can_be_overridden(self):
        result, commands, _, _ = self.deploy(keep_storage="10GB")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("builder prune --all --force --keep-storage 10GB", commands)

    def test_invalid_cache_budget_skips_cleanup_without_failing_release(self):
        result, commands, _, _ = self.deploy(keep_storage="0")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("builder prune", commands)
        self.assertIn("invalid build-cache retention", result.stderr.lower())

    def test_failed_release_does_not_prune_cache(self):
        for failure in ["fail_build", "fail_security", "fail_readiness"]:
            with self.subTest(failure=failure):
                result, commands, _, _ = self.deploy(**{failure: True})
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("builder prune", commands)

    def test_opt_in_limiter_starts_and_waits_before_app_replacement(self):
        result, commands, _, _ = self.deploy(limiter=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("compose up -d --wait --wait-timeout 30 ozmishow-redis", commands)
        self.assertLess(commands.index("--wait-timeout 30 ozmishow-redis"), commands.index("--force-recreate app"))

    def test_rollback_reconfigures_private_redis_before_previous_app(self):
        result, commands, _, _ = self.deploy(fail_readiness=True, limiter=True, previous_limiter=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("sha256:previous compose up -d --wait --wait-timeout 30 ozmishow-redis", commands)
        self.assertLess(commands.index("sha256:previous compose up -d --wait"), commands.index("sha256:previous compose up -d --no-deps --force-recreate app"))

    def test_authenticated_limiter_failure_rolls_back(self):
        result, commands, _, _ = self.deploy(limiter=True, previous_limiter=True, fail_limiter_probe=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Previous release restored.", result.stderr)
        self.assertIn("/internal/ozmishow/rate-limit", commands)
        self.assertIn("$OZMISHOW_RATE_LIMIT_SERVICE_SECRET", commands)

    def test_build_failure_keeps_running_release_and_environment(self):
        result, commands, environment, diagnostics = self.deploy(fail_build=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("force-recreate", commands)
        self.assertNotIn(" down", commands)
        self.assertEqual(environment, "VALUE=old\n")
        self.assertEqual(diagnostics, {})

    def test_failed_readiness_rolls_back_image_and_environment(self):
        result, commands, environment, _ = self.deploy(fail_readiness=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("sha256:previous compose up", commands)
        self.assertIn("sha256:previous compose exec -T nginx nginx -s reload", commands)
        self.assertIn("Previous release restored.", result.stderr)
        self.assertEqual(environment, "VALUE=old\n")

    def test_success_builds_before_replacement_and_keeps_new_environment(self):
        result, commands, environment, diagnostics = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertLess(commands.index("compose build"), commands.index("force-recreate"))
        self.assertNotIn(" down", commands)
        self.assertEqual(environment, "VALUE=new\n")
        self.assertEqual(diagnostics, {})

    def test_security_smoke_failure_also_rolls_back(self):
        result, commands, environment, _ = self.deploy(fail_security=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Previous release restored.", result.stderr)
        self.assertEqual(environment, "VALUE=old\n")

    def test_newer_previous_release_uses_dependency_readiness(self):
        result, commands, environment, _ = self.deploy(fail_readiness=True, previous_has_ready=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Previous release restored.", result.stderr)
        self.assertIn("sha256:previous compose exec -T app wget --quiet --spider --timeout=5 http://127.0.0.1:4000/ready",commands)
        self.assertNotIn("http://127.0.0.1:4000/health",commands)

    def test_healthy_ipv4_app_is_not_rolled_back(self):
        result, commands, environment, _ = self.deploy(ipv4_only=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("Previous release restored.", result.stderr)
        self.assertEqual(environment, "VALUE=new\n")

    def test_rollback_can_verify_previous_ipv4_app(self):
        result, commands, environment, _ = self.deploy(fail_readiness=True, ipv4_only=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Previous release restored.", result.stderr)
        self.assertEqual(environment, "VALUE=old\n")

    def test_failed_release_logs_survive_rollback_without_entering_ci_output(self):
        result, _, environment, diagnostics = self.deploy(fail_readiness=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Previous release restored.", result.stderr)
        self.assertEqual(environment, "VALUE=old\n")
        self.assertIn("test-release.failed.log", diagnostics)
        content, mode = diagnostics["test-release.failed.log"]
        self.assertIn("autotable-app:test-release", content)
        self.assertIn("SYNTHETIC_PRIVATE_LOG", content)
        self.assertEqual(mode, 0o600)
        self.assertNotIn("SYNTHETIC_PRIVATE_LOG", result.stdout + result.stderr)

    def test_failed_rollback_preserves_both_container_diagnostics(self):
        result, _, environment, diagnostics = self.deploy(fail_readiness=True, fail_rollback=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Automatic rollback failed", result.stderr)
        self.assertEqual(environment, "VALUE=old\n")
        self.assertIn("test-release.failed.log", diagnostics)
        self.assertIn("test-release.rollback.log", diagnostics)
        self.assertIn("autotable-app:test-release", diagnostics["test-release.failed.log"][0])
        content, mode = diagnostics["test-release.rollback.log"]
        self.assertIn("sha256:previous", content)
        self.assertEqual(mode, 0o600)
        self.assertNotIn("SYNTHETIC_PRIVATE_LOG", result.stdout + result.stderr)



class RuntimeSecretPreservationTests(unittest.TestCase):
    def test_rsync_preserves_operator_diagnostics_credentials(self):
        import shlex
        workflow = (ROOT / ".github/workflows/production.yml").read_text()
        command = workflow.split("rsync -avz --delete", 1)[1].split("./ $SSH_USER@", 1)[0]
        exclusions = [arg for arg in shlex.split(command.replace("\\\n", "")) if arg.startswith("--exclude=")]
        with tempfile.TemporaryDirectory() as root:
            source, target = Path(root) / "source", Path(root) / "target"
            source.mkdir(); target.mkdir()
            credential = target / "observability/secrets/autotable-observability-token"
            credential.parent.mkdir(parents=True)
            credential.write_text("synthetic-private-token")
            (target / "stale-source.go").write_text("old")
            result = subprocess.run(["rsync", "-a", "--delete", *exclusions, str(source) + "/", str(target) + "/"], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertTrue(credential.exists(), "deployment deleted the operator's diagnostics credential")
            self.assertEqual(credential.read_text(), "synthetic-private-token")
            self.assertFalse((target / "stale-source.go").exists())

if __name__ == "__main__":
    unittest.main()
