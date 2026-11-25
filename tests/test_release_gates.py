"""Run release orchestration against a controlled Docker transport."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts/check-container.sh"


class ContainerReleaseGateTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(); self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        tool = self.root / "docker"
        tool.write_text('''#!/usr/bin/env python3
import json,os,sys
from pathlib import Path
args=sys.argv[1:]
with open(os.environ['DOCKER_TEST_LOG'],'a') as out: out.write(json.dumps(args)+'\\n')
if args[0]=='info': sys.exit(int(os.environ.get('DOCKER_TEST_UNAVAILABLE','0')))
if args[0]=='build': sys.exit(int(os.environ.get('DOCKER_TEST_BUILD_FAILURE','0')))
if args[0]=='save': Path(args[args.index('--output')+1]).write_bytes(b'fixture archive')
if args[0]=='run': sys.exit(int(os.environ.get('DOCKER_TEST_RUN_FAILURE','0')))
''')
        tool.chmod(0o755)
        self.log = self.root / "docker.log"

    def run_gate(self, mode, **settings):
        env = dict(os.environ, PATH=str(self.root) + os.pathsep + os.environ["PATH"], DOCKER_TEST_LOG=str(self.log), AUTOTABLE_CHECK_REPORT_DIR=str(self.root / "reports"))
        env.update(settings)
        result = subprocess.run(["bash", str(SCRIPT), mode], env=env, capture_output=True, text=True)
        calls = [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []
        return result, calls

    def test_stopped_docker_fails_without_building_or_running(self):
        result, calls = self.run_gate("image", DOCKER_TEST_UNAVAILABLE="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Docker", result.stderr)
        self.assertFalse(any(call[0] in ("build", "run") for call in calls))

    def test_image_build_failure_propagates_without_runtime_probe(self):
        result, calls = self.run_gate("image", DOCKER_TEST_BUILD_FAILURE="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(any(call[0] == "build" for call in calls))
        self.assertFalse(any(call[0] == "run" for call in calls))

    def test_image_probe_has_no_network_and_never_starts_application(self):
        result, calls = self.run_gate("image")
        self.assertEqual(result.returncode, 0, result.stderr)
        call = next(call for call in calls if call[0] == "run")
        self.assertEqual(call[call.index("--network")+1], "none")
        self.assertEqual(call[call.index("--entrypoint")+1], "/bin/sh")
        result, _ = self.run_gate("image", DOCKER_TEST_RUN_FAILURE="1")
        self.assertNotEqual(result.returncode, 0)

    def test_os_scanner_uses_archive_without_socket_and_propagates_findings(self):
        result, calls = self.run_gate("scan", DOCKER_TEST_RUN_FAILURE="1")
        self.assertNotEqual(result.returncode, 0)
        call = next(call for call in calls if call[0] == "run")
        self.assertIn("--input", call)
        self.assertEqual(call[call.index("--pkg-types")+1], "os")
        self.assertEqual(call[call.index("--exit-code")+1], "1")
        self.assertNotIn("--ignore-unfixed", call)
        self.assertNotIn("docker.sock", " ".join(call))
        self.assertFalse(any(self.root.glob("reports/image-archive.*")))

    def test_success_without_fresh_os_report_fails_closed(self):
        result, _ = self.run_gate("scan")
        self.assertNotEqual(result.returncode, 0)

    def test_mounts_require_explicit_existing_staging_directories(self):
        result, _ = self.run_gate("mounts")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "uploads").exists())
        for name in ("uploads", "logs"): (self.root / name).mkdir()
        marker = self.root / "uploads/sentinel"; marker.write_text("unchanged")
        result, calls = self.run_gate("mounts", STAGING_TEMP_DIR=str(self.root / "uploads"), STAGING_LOG_DIR=str(self.root / "logs"))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(marker.read_text(), "unchanged")
        self.assertFalse(any(call[0] == "build" for call in calls))
        self.assertNotIn("chown", " ".join(calls[-1]))


if __name__ == "__main__": unittest.main()
