"""Exercise the quality gates as CLIs with controlled reports and tool failures."""
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
GATE = ROOT / "scripts/quality_gate.py"


class QualityGateTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        (self.root / "go.mod").write_text("module example.test/app\n\ngo 1.26.0\n")
        (self.root / "auth.go").write_text('package app\nvar old = "Legacy API message"\n')

    def run_gate(self, kind, *args):
        return subprocess.run([sys.executable, str(GATE), kind, "--root", str(self.root), *map(str, args)], capture_output=True, text=True)

    def coverage(self, body, config=None):
        profile = self.root / "coverage.out"
        profile.write_text(body)
        floors = self.root / "coverage.json"
        floors.write_text(json.dumps(config or {"total": 80, "files": {"auth.go": 100}}))
        return self.run_gate("coverage", "--profile", profile, "--thresholds", floors)

    def test_coverage_weights_statements_and_merges_cross_package_blocks(self):
        result = self.coverage("mode: atomic\nexample.test/app/auth.go:2.1,2.8 8 0\nexample.test/app/auth.go:2.1,2.8 8 1\nexample.test/app/other.go:3.1,3.8 2 0\n")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("80.00%", result.stdout)

    def test_low_total_and_missing_or_uncovered_security_files_fail(self):
        cases = [
            "mode: atomic\nexample.test/app/auth.go:2.1,2.8 1 1\nexample.test/app/other.go:3.1,3.8 9 0\n",
            "mode: atomic\nexample.test/app/other.go:3.1,3.8 2 1\n",
            "mode: atomic\nexample.test/app/auth.go:2.1,2.8 2 0\n",
        ]
        for body in cases:
            with self.subTest(body=body):
                self.assertNotEqual(self.coverage(body).returncode, 0)

    def test_empty_malformed_and_inconsistent_profiles_fail_closed(self):
        for body in ["", "mode: atomic\n", "mode: atomic\ninvalid\n", "mode: atomic\nexample.test/app/auth.go:2.1,2.8 1 1\nexample.test/app/auth.go:2.1,2.8 8 1\n"]:
            with self.subTest(body=body):
                self.assertNotEqual(self.coverage(body).returncode, 0)

    def lint(self, findings, baseline=None, exit_code=1, raw=None):
        report = raw if raw is not None else "\n".join(json.dumps(row) for row in findings)
        tool = self.root / "staticcheck"
        tool.write_text("#!/usr/bin/env python3\nimport sys\nsys.stdout.write(" + repr(report) + ")\nsys.exit(" + str(exit_code) + ")\n")
        tool.chmod(0o755)
        path = self.root / "baseline.json"
        path.write_text(json.dumps({"version": 1, "findings": baseline or []}))
        return self.run_gate("staticcheck", "--binary", tool, "--baseline", path)

    def finding(self, code="ST1005", message="legacy capitalization", line=2):
        return {"code": code, "severity": "error", "location": {"file": str(self.root / "auth.go"), "line": line}, "message": message}

    def baseline(self):
        return [{"code": "ST1005", "file": "auth.go", "message": "legacy capitalization", "source": 'var old = "Legacy API message"', "count": 1}]

    def test_exact_legacy_warning_passes_but_new_or_additional_warning_fails(self):
        row = self.finding()
        self.assertEqual(self.lint([row], self.baseline()).returncode, 0)
        self.assertNotEqual(self.lint([row, row], self.baseline()).returncode, 0)
        self.assertNotEqual(self.lint([self.finding(message="new warning")], self.baseline()).returncode, 0)
        self.assertNotEqual(self.lint([self.finding(code="SA4006")], self.baseline()).returncode, 0)

    def test_moved_legacy_line_passes_but_changed_source_and_stale_baseline_fail(self):
        (self.root / "auth.go").write_text('package app\n\nvar old = "Legacy API message"\n')
        self.assertEqual(self.lint([self.finding(line=3)], self.baseline()).returncode, 0)
        (self.root / "auth.go").write_text('package app\nvar other = "Different API message"\n')
        self.assertNotEqual(self.lint([self.finding()], self.baseline()).returncode, 0)
        self.assertNotEqual(self.lint([], self.baseline(), exit_code=0).returncode, 0)

    def test_tool_error_empty_error_output_and_invalid_json_fail_closed(self):
        for status, raw in [(2, ""), (1, ""), (1, "not-json")]:
            with self.subTest(status=status, raw=raw):
                self.assertNotEqual(self.lint([], exit_code=status, raw=raw).returncode, 0)
        self.assertEqual(self.lint([], exit_code=0).returncode, 0)

    def test_bug_findings_cannot_be_added_to_legacy_baseline(self):
        baseline = self.baseline(); baseline[0]["code"] = "SA4006"
        self.assertNotEqual(self.lint([self.finding(code="SA4006")], baseline).returncode, 0)


if __name__ == "__main__":
    unittest.main()
