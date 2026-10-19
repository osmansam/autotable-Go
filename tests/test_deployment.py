"""Black-box deployment checks; every Docker/curl invocation is a local fake."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class DeploymentTests(unittest.TestCase):
    def deploy(self, fail_build=False, fail_readiness=False, fail_security=False, previous_has_ready=False):
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
    print('sha256:previous');sys.exit(0)
if 'build' in args and os.environ.get('FAIL_BUILD')=='1':sys.exit(1)
if 'up' in args and 'app' in args:
    expected='health' if os.environ.get('AUTOTABLE_IMAGE')=='sha256:previous' and os.environ.get('PREVIOUS_HAS_READY')!='1' else 'ready'
    if os.environ.get('AUTOTABLE_READINESS_PATH')!=expected:sys.exit(1)
if 'wget' in args:
    if os.environ.get('AUTOTABLE_IMAGE')=='sha256:previous' and args[-1].endswith('/ready') and os.environ.get('PREVIOUS_HAS_READY')!='1':sys.exit(1)
    if os.environ.get('FAIL_READINESS')=='1' and os.environ.get('AUTOTABLE_IMAGE')!='sha256:previous':sys.exit(1)
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
            environment = {**os.environ, "PATH": str(binaries) + os.pathsep + os.environ["PATH"], "AUTOTABLE_RELEASE_TAG": "test-release", "AUTOTABLE_DEPLOY_DIRECTORY": str(root), "MOCK_LOG": str(root / "commands.log"), "FAIL_BUILD": str(int(fail_build)), "FAIL_READINESS": str(int(fail_readiness)), "FAIL_SECURITY":str(int(fail_security)), "PREVIOUS_HAS_READY":str(int(previous_has_ready))}
            result = subprocess.run(["bash", str(ROOT / "scripts/deploy-production.sh")], env=environment, capture_output=True, text=True)
            commands = (root / "commands.log").read_text() if (root / "commands.log").exists() else ""
            return result, commands, (root / ".env").read_text()

    def test_build_failure_keeps_running_release_and_environment(self):
        result, commands, environment = self.deploy(fail_build=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("force-recreate", commands)
        self.assertNotIn(" down", commands)
        self.assertEqual(environment, "VALUE=old\n")

    def test_failed_readiness_rolls_back_image_and_environment(self):
        result, commands, environment = self.deploy(fail_readiness=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("sha256:previous compose up", commands)
        self.assertIn("sha256:previous compose exec -T nginx nginx -s reload", commands)
        self.assertIn("Previous release restored.", result.stderr)
        self.assertEqual(environment, "VALUE=old\n")

    def test_success_builds_before_replacement_and_keeps_new_environment(self):
        result, commands, environment = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertLess(commands.index("compose build"), commands.index("force-recreate"))
        self.assertNotIn(" down", commands)
        self.assertEqual(environment, "VALUE=new\n")

    def test_security_smoke_failure_also_rolls_back(self):
        result, commands, environment = self.deploy(fail_security=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Previous release restored.", result.stderr)
        self.assertEqual(environment, "VALUE=old\n")

    def test_newer_previous_release_uses_dependency_readiness(self):
        result, commands, environment = self.deploy(fail_readiness=True, previous_has_ready=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Previous release restored.", result.stderr)
        self.assertIn("sha256:previous compose exec -T app wget --quiet --spider --timeout=5 http://localhost:4000/ready",commands)
        self.assertNotIn("http://localhost:4000/health",commands)


if __name__ == "__main__":
    unittest.main()
