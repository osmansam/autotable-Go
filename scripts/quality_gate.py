#!/usr/bin/env python3
"""Fail-closed coverage floors and exact legacy Staticcheck accounting."""
import argparse
from collections import Counter, defaultdict
from decimal import Decimal
import json
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
LEGACY_CODES = {"ST1005", "SA1019"}


def load_json(path):
    return json.loads(Path(path).read_text())


def coverage(args):
    config = load_json(args.thresholds)
    module = re.search(r"^module\s+(\S+)", (args.root / "go.mod").read_text(), re.M).group(1)
    lines = args.profile.read_text().splitlines()
    if not lines or lines[0] not in {"mode: atomic", "mode: count", "mode: set"}:
        raise ValueError("missing or invalid coverage mode")
    blocks = {}
    for line in lines[1:]:
        match = re.fullmatch(r"(.+):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)", line)
        if not match:
            raise ValueError("malformed coverage record")
        location, statements, count = line.rsplit(" ", 2)
        source = match.group(1)
        if not source.startswith(module + "/"):
            raise ValueError("coverage record is outside the application module")
        statements, count = int(statements), int(count)
        if location in blocks and blocks[location][0] != statements:
            raise ValueError("inconsistent statement count in duplicate block")
        previous = blocks.get(location, (statements, 0))[1]
        blocks[location] = (statements, previous + count)
    totals = defaultdict(lambda: [0, 0])
    for location, (statements, count) in blocks.items():
        name = location.rsplit(":", 1)[0][len(module) + 1:]
        totals[name][0] += statements
        totals[name][1] += statements if count > 0 else 0
    checks = [("total", [sum(x[0] for x in totals.values()), sum(x[1] for x in totals.values())], config["total"])]
    checks.extend((name, totals.get(name, [0, 0]), floor) for name, floor in config["files"].items())
    failures = []
    for name, (statements, covered), floor in checks:
        minimum = Decimal(str(floor))
        if not minimum.is_finite() or not 0 <= minimum <= 100:
            raise ValueError("coverage threshold must be between 0 and 100")
        if not statements:
            failures.append(name + ": no coverage records")
            continue
        actual = Decimal(covered * 100) / statements
        print(f"{name}: {actual:.2f}% (minimum {minimum}%)")
        if actual < minimum:
            failures.append(name + ": below coverage floor")
    if failures:
        raise ValueError("; ".join(failures))


def fingerprint(row, root):
    code, message = row["code"], row["message"].strip()
    location = row["location"]
    source = Path(location["file"])
    source = source if source.is_absolute() else root / source
    name = source.resolve().relative_to(root).as_posix()
    number = location["line"]
    lines = source.read_text().splitlines()
    if not isinstance(number, int) or number < 1 or number > len(lines):
        raise ValueError("invalid Staticcheck source location")
    return code, name, message, lines[number - 1].strip()


def staticcheck(args):
    baseline = load_json(args.baseline)
    if baseline["version"] != 1:
        raise ValueError("unsupported Staticcheck baseline")
    expected = Counter()
    for row in baseline["findings"]:
        if row["code"] not in LEGACY_CODES or type(row["count"]) is not int or row["count"] < 1:
            raise ValueError("only counted legacy ST1005/SA1019 findings may be baselined")
        key = row["code"], row["file"], row["message"], row["source"]
        if key in expected:
            raise ValueError("duplicate baseline entry")
        expected[key] = row["count"]
    result = subprocess.run([str(args.binary), "-f", "json", "./..."], cwd=args.root, capture_output=True, text=True)
    if args.report:
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(result.stdout)
    if result.returncode not in (0, 1):
        raise ValueError(f"Staticcheck failed with status {result.returncode}: {result.stderr.strip()}")
    rows = [json.loads(line) for line in result.stdout.splitlines() if line.strip()]
    if result.returncode == 1 and not rows:
        raise ValueError("Staticcheck failed without a findings report: " + result.stderr.strip())
    actual = Counter(fingerprint(row, args.root) for row in rows)
    new, stale = actual - expected, expected - actual
    for (code, name, message, _), count in new.items():
        print(f"NEW {name}: {message} ({code}, count {count})", file=sys.stderr)
    for (code, name, _, _), count in stale.items():
        print(f"STALE baseline {name}: {code}, count {count}; remove resolved entries", file=sys.stderr)
    if new or stale:
        raise ValueError("Staticcheck findings differ from the reviewed legacy baseline")
    print(f"Staticcheck: no new findings; {sum(expected.values())} exact legacy style/deprecation findings retained")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="gate", required=True)
    cover = subparsers.add_parser("coverage")
    cover.add_argument("--profile", type=Path, required=True)
    cover.add_argument("--thresholds", type=Path, default=ROOT / "coverage-thresholds.json")
    lint = subparsers.add_parser("staticcheck")
    lint.add_argument("--binary", type=Path, required=True)
    lint.add_argument("--baseline", type=Path, default=ROOT / "staticcheck-baseline.json")
    lint.add_argument("--report", type=Path)
    for command in (cover, lint):
        command.add_argument("--root", type=lambda p: Path(p).resolve(), default=ROOT)
    args = parser.parse_args()
    # An absolute path is essential for local versioned binaries; bare commands
    # remain PATH-resolved for callers such as CI.
    if args.gate == "staticcheck" and args.binary.exists():
        args.binary = args.binary.resolve()
    try:
        (coverage if args.gate == "coverage" else staticcheck)(args)
    except (OSError, ValueError, KeyError, TypeError, AttributeError, IndexError) as error:
        print(f"Quality gate failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
