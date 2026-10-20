#!/usr/bin/env python3
"""
Compares the SCC liveness engine with the old cycle finder.

Finds every spec under examples/ with a liveness assertion and model checks
it in shadow mode (FIZZ_LIVENESS_ENGINE=shadow), along with variants where
every action is made unfair, weakly fair or strongly fair, and one where the
top-level actions are declared in reverse order. Shadow mode checks each
property with both checkers and validates every counterexample against the
fairness definition and against the cycle finder's own fairness check.

    python3 test_liveness_shadow.py

Exits non-zero on a disagreement not listed in KNOWN_DIFFERENCES, or on a
counterexample that fails validation. Variants are written to a temporary
directory; the examples themselves are not modified.
"""

import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

FIZZ = os.environ.get("FIZZ", str(Path("fizz").resolve()))
TIMEOUT_SECONDS = int(os.environ.get("FIZZ_TIMEOUT", "120"))

# Disagreements already settled by hand. Each entry says which checker is
# wrong and why.
KNOWN_DIFFERENCES = {
    # The cycle finder misses a fair loop: node 0 stays down, every action
    # is taken infinitely often, and the nodes never agree. Its own fairness
    # check judges that loop fair, so this is a search miss.
    ("examples/comparisons/gossa-v1/gossa.fizz", "strong"),
}

ACTION = re.compile(
    r"^(\s*)((?:atomic|serial|oneof|parallel)\s+)?((?:fair(?:<\w+>)?\s+)?)action\s+(\w+)",
    re.M,
)


def set_fairness(src, fair):
    def sub(m):
        if m.group(4) == "Init":
            return m.group(0)
        return m.group(1) + (m.group(2) or "") + fair + "action " + m.group(4)

    return ACTION.sub(sub, src)


def reverse_top_actions(src):
    """Reverses contiguous top-level action blocks, or returns None."""
    lines = src.split("\n")
    starts = [
        i
        for i, l in enumerate(lines)
        if l and not l[0].isspace() and ACTION.match(l) and ACTION.match(l).group(4) != "Init"
    ]
    if len(starts) < 2:
        return None
    blocks = []
    for s in starts:
        e = s + 1
        while e < len(lines) and (lines[e] == "" or lines[e][0].isspace()):
            e += 1
        blocks.append((s, e))
    if any(e1 != s2 for (_, e1), (s2, _) in zip(blocks, blocks[1:])):
        return None
    head, tail = lines[: blocks[0][0]], lines[blocks[-1][1] :]
    body = [l for s, e in reversed(blocks) for l in lines[s:e]]
    return "\n".join(head + body + tail)


def liveness_specs():
    for path in sorted(Path("examples").rglob("*.fizz")):
        if "/out/" in str(path):
            continue
        if re.search(r"^\s*(always\s+eventually|eventually\s+always)\b", path.read_text(), re.M):
            yield path


def run(spec_file):
    env = dict(os.environ, FIZZ_LIVENESS_ENGINE="shadow")
    try:
        r = subprocess.run(
            [FIZZ, "--test", str(spec_file)],
            capture_output=True, text=True, timeout=TIMEOUT_SECONDS, env=env,
        )
    except subprocess.TimeoutExpired:
        return None
    return [l for l in r.stdout.splitlines() if l.startswith("Liveness shadow")]


def main():
    work = Path(tempfile.mkdtemp(prefix="liveness-shadow-"))
    checks = failures = 0
    try:
        for spec in liveness_specs():
            src = spec.read_text()
            variants = {
                "original": src,
                "unfair": set_fairness(src, ""),
                "weak": set_fairness(src, "fair "),
                "strong": set_fairness(src, "fair<strong> "),
            }
            reversed_src = reverse_top_actions(src)
            if reversed_src:
                variants["reversed"] = reversed_src
            for name, text in variants.items():
                d = work / str(spec.parent).replace("/", "_") / name
                shutil.copytree(spec.parent, d, ignore=shutil.ignore_patterns("out", "*.json"))
                (d / spec.name).write_text(text)
                lines = run(d / spec.name)
                if lines is None:
                    print(f"TIMEOUT  {spec} [{name}]")
                    continue
                for line in lines:
                    checks += 1
                    bad = "DISAGREE" in line or "invalid" in line or "unfair by" in line
                    known = (str(spec), name) in KNOWN_DIFFERENCES
                    if bad and not known:
                        failures += 1
                    tag = "KNOWN" if bad and known else ("FAIL" if bad else "ok")
                    print(f"{tag:8} {spec} [{name}] {line[len('Liveness shadow '):]}")
    finally:
        shutil.rmtree(work, ignore_errors=True)
    print(f"\n{checks} property checks, {failures} unexplained")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
