"""Black-box deployment checks; every Docker/curl invocation is a local fake."""
import os
from pathlib import Path
import stat
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class DeploymentTests(unittest.TestCase):
    def deploy(self, fail_build=False, fail_readiness=False, fail_security=False, previous_has_ready=False, ipv4_only=False, fail_rollback=False):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / ".env").write_text("VALUE=old\n")
            (root / ".env.next").write_text("VALUE=new\n")
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
elif '/container/create-multiple' in url: print('401',end='')
""")
            curl.chmod(0o755)
            sleep = binaries / "sleep"
            sleep.write_text("#!/bin/sh\nexit 0\n")
            sleep.chmod(0o755)
            environment = {**os.environ, "PATH": str(binaries) + os.pathsep + os.environ["PATH"], "AUTOTABLE_RELEASE_TAG": "test-release", "AUTOTABLE_DEPLOY_DIRECTORY": str(root), "MOCK_LOG": str(root / "commands.log"), "FAIL_BUILD": str(int(fail_build)), "FAIL_READINESS": str(int(fail_readiness)), "FAIL_SECURITY":str(int(fail_security)), "PREVIOUS_HAS_READY":str(int(previous_has_ready)), "IPV4_ONLY":str(int(ipv4_only)), "FAIL_ROLLBACK":str(int(fail_rollback))}
            result = subprocess.run(["bash", str(ROOT / "scripts/deploy-production.sh")], env=environment, capture_output=True, text=True)
            commands = (root / "commands.log").read_text() if (root / "commands.log").exists() else ""
            diagnostics = {
                path.name: (path.read_text(), stat.S_IMODE(path.stat().st_mode))
                for path in (root / ".deploy").glob("*.log")
            }
            return result, commands, (root / ".env").read_text(), diagnostics

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


if __name__ == "__main__":
    unittest.main()
