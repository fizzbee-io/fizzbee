#!/usr/bin/env python3
"""
Test runner for FizzBee reference examples.

Runs all examples that have a baseline.txt, normalizes the output
(strips timestamps, file paths, elapsed times), and compares key metrics.

Phase 1 (default):
    python3 test_references.py
    Verifies all examples pass. Reports which specs use deprecated `any`.

Phase 2 (after replacing `any` with `oneof`):
    python3 test_references.py
    Same command — expects same metrics, zero DeprecationWarnings.
"""

import re
import subprocess
import sys
import time
from pathlib import Path

REFS_DIR = Path("examples/references")
LOVABLE_DIR = Path("examples/requirements/lovable")
FIZZ = "./fizz"

# Patterns to KEEP in normalized output for comparison
KEEP_PREFIXES = (
    "StateSpaceOptions:",
    "fizz.yaml",           # "fizz.yaml not found. Using default options"
    "Valid Nodes:",
    "IsLive:",
    "Checking ",           # liveness assertion names
    "PASSED:",
    "FAILED:",
    "Skipping dotfile",    # "Skipping dotfile generation. Too many Nodes: N"
)

# Patterns to explicitly SKIP (before the keep check)
SKIP_PREFIXES = (
    "Model checking ",     # has file path
    "configFileName:",     # has file path
    "Time taken",
    "Writen",
    "To generate",
    "dot ",
    "open ",
    "Downloading",
)
SKIP_PATTERNS = [
    re.compile(r"run_\d{4}-\d{2}-\d{2}"),   # timestamp in paths
    re.compile(r"^\./fizz.*\d+s user"),       # shell timing line
]


def normalize(text: str) -> list[str]:
    """Extract stable, comparison-worthy lines from model checker output."""
    result = []
    for raw in text.splitlines():
        s = raw.strip()
        if not s:
            continue

        # Nodes line: only keep the final line (queued: 0), strip elapsed
        if re.match(r"^Nodes:\s*\d+", s):
            if re.search(r",\s*queued:\s*0\b", s):
                s = re.sub(r",\s*elapsed:.*$", "", s)
                result.append(s)
            # skip intermediate progress lines (queued > 0)
            continue

        # Skip noisy lines
        if any(s.startswith(p) for p in SKIP_PREFIXES):
            continue
        if any(p.search(s) for p in SKIP_PATTERNS):
            continue

        # Keep important stable lines
        if any(s.startswith(p) for p in KEEP_PREFIXES):
            # Normalise proto whitespace (e.g. "key:value  key2:value" → single space)
            if s.startswith("StateSpaceOptions:"):
                s = re.sub(r'\s{2,}', ' ', s)
            result.append(s)

    return result


def find_fizz_file(d: Path) -> Path | None:
    """Return the first .fizz file in a directory, or None."""
    files = sorted(d.glob("*.fizz"))
    return files[0] if files else None


def run_spec(fizz_file: Path) -> tuple[str, str, float]:
    t0 = time.monotonic()
    r = subprocess.run([FIZZ, str(fizz_file)], capture_output=True, text=True)
    elapsed = time.monotonic() - t0
    return r.stdout, r.stderr, elapsed


def diff_lines(expected: list[str], actual: list[str]) -> list[str]:
    """Return a simple unified-style diff for two line lists."""
    lines = []
    i = j = 0
    while i < len(expected) or j < len(actual):
        e = expected[i] if i < len(expected) else None
        a = actual[j] if j < len(actual) else None
        if e == a:
            lines.append(f"   {e}")
            i += 1; j += 1
        elif e is not None and e not in actual[j:]:
            lines.append(f"-  {e}")
            i += 1
        elif a is not None and a not in expected[i:]:
            lines.append(f"+  {a}")
            j += 1
        else:
            lines.append(f"-  {e}")
            lines.append(f"+  {a}")
            i += 1; j += 1
    return lines


def main() -> int:
    # Collect examples with baseline.txt and a .fizz file
    examples = []
    for d in sorted(REFS_DIR.iterdir()):
        if not d.is_dir():
            continue
        baseline = d / "baseline.txt"
        fizz = find_fizz_file(d)
        if baseline.exists() and fizz:
            examples.append((d.name, fizz, baseline))

    print(f"Found {len(examples)} examples with baselines\n")

    passed = failed = deprecated = 0
    deprecated_list: list[str] = []
    total_wall = 0.0

    for name, fizz_file, baseline_file in examples:
        expected = normalize(baseline_file.read_text())
        stdout, stderr, wall = run_spec(fizz_file)
        total_wall += wall
        actual = normalize(stdout + "\n" + stderr)

        has_dep = "DeprecationWarning:" in stderr

        if actual == expected:
            status = "PASS"
            passed += 1
        else:
            status = "FAIL"
            failed += 1

        dep_tag = " [any→deprecated]" if has_dep else ""
        if has_dep:
            deprecated += 1
            deprecated_list.append(str(fizz_file))

        print(f"  {status}  [{wall:.2f}s]  {name}{dep_tag}")

        if status == "FAIL":
            for line in diff_lines(expected, actual):
                print(f"       {line}")
            print()

    # Lovable specs (no baseline — just run and report status)
    lovable_specs = sorted(LOVABLE_DIR.rglob("*.fizz")) if LOVABLE_DIR.exists() else []
    if lovable_specs:
        print(f"\n{'='*60}")
        print(f"Lovable specs ({len(lovable_specs)} files, no baseline — status only)")
        print(f"{'='*60}")
        for fizz_file in lovable_specs:
            stdout, stderr, wall = run_spec(fizz_file)
            total_wall += wall
            combined = stdout + "\n" + stderr
            # Extract a few key fields inline
            m_nodes = re.search(r"Nodes:\s*(\d+),\s*queued:\s*0", combined)
            m_valid = re.search(r"Valid Nodes:\s*(\d+)\s+Unique states:\s*(\d+)", combined)
            m_status = "PASSED" if "PASSED:" in combined else ("DEADLOCK" if "DEADLOCK detected" in combined else ("FAILED" if "FAILED:" in combined else "UNKNOWN"))
            nodes_str = m_nodes.group(1) if m_nodes else "?"
            valid_str = f"{m_valid.group(1)}/{m_valid.group(2)}" if m_valid else "?"
            print(f"  {m_status:8s}  [{wall:.2f}s]  nodes={nodes_str} valid(nodes/states)={valid_str}  {fizz_file}")

    # Summary
    print(f"\n{'='*60}")
    print(f"Results: {passed} passed, {failed} failed  (total wall time: {total_wall:.1f}s)")
    print(f"Deprecated `any` usage: {deprecated} files")
    if deprecated_list:
        for f in deprecated_list:
            print(f"  - {f}")

    return 1 if failed > 0 else 0


if __name__ == "__main__":
    sys.exit(main())
