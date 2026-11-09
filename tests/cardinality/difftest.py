#!/usr/bin/env python3
"""Differential test for cardinality.counter.

Each case directory NN-name/ holds:
  Counter.fizz   the spec written with cardinality.counter
  Int.fizz       the same spec with a plain int bounded by a constant N
  expected.txt   the expected verdict of every assertion in Counter.fizz

FizzBee stops at the first failing assertion, so every assertion is checked
in its own run: the spec is copied to a temp directory with all the other
assertions removed.

Checks, per assertion:
  1. soundness: if Int FAILS (or errors), Counter must FAIL (or error);
  2. Counter matches expected.txt;
  3. Counter FAIL with Int PASS is allowed only when expected.txt marks the
     assertion `imprecision`;
  4. Int matches the `int=` verdict recorded in expected.txt, if any;
  5. for a Counter FAIL, the counterexample loop contains every action named
     by `loop_includes=A,B` (`Recv` also matches a role action `Server#1.Recv`).

Optional flags.txt (both specs), int_flags.txt, counter_flags.txt: extra
command line flags, e.g. --no-symmetry-reduction for the Int ground truth.

expected.txt lines (a `#` at the start of a line or after a space starts a
comment):
  reason: <one line of reasoning>
  <Assertion>: PASS|FAIL|ERROR [imprecision] [int=PASS|FAIL|ERROR] [loop_includes=A,B]

Usage:
  python3 tests/cardinality/difftest.py [--sweep 2,3,4,5] [-v] [case-filter ...]

  FIZZ          fizz wrapper to run (default: ./fizz at the repo root)
  FIZZ_TIMEOUT  per-run timeout in seconds (default 120)

--sweep reruns Int.fizz with `N = <n>` for each n; Int counts as FAIL if any
N fails. Exit status: 0 all ok, 1 mismatch, 2 feature not implemented.
"""

import argparse
import concurrent.futures
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent.parent
FIZZ = os.environ.get("FIZZ", str(REPO / "fizz"))
TIMEOUT = int(os.environ.get("FIZZ_TIMEOUT", "120"))

ASSERT_RE = re.compile(r"^[a-z][a-z ]*\bassertion\s+(\w+)\s*:")
NOT_IMPL_RE = re.compile(
    r"cardinality.*not found|undefined: cardinality|name 'cardinality'|"
    r"has no \.?counter (field|method)|Receiver cardinality",
    re.IGNORECASE)
VERDICTS = ("PASS", "FAIL", "ERROR")


# ---------------------------------------------------------------- spec text

def assertion_blocks(lines):
    """[(name, start, end)] for each top-level assertion; start includes any
    decorator lines (@assume ...) directly above it."""
    blocks = []
    i = 0
    while i < len(lines):
        m = ASSERT_RE.match(lines[i])
        if not m:
            i += 1
            continue
        start = i
        while start > 0 and lines[start - 1].startswith("@"):
            start -= 1
        end = i + 1
        while end < len(lines) and (not lines[end].strip() or lines[end][0] in " \t"):
            end += 1
        blocks.append((m.group(1), start, end))
        i = end
    return blocks


def only_assertion(text, keep):
    lines = text.splitlines(keepends=True)
    drop = set()
    for name, s, e in assertion_blocks(lines):
        if name != keep:
            drop.update(range(s, e))
    return "".join(l for i, l in enumerate(lines) if i not in drop)


def assertion_names(text):
    return [n for n, _, _ in assertion_blocks(text.splitlines(keepends=True))]


def with_n(text, n):
    new, count = re.subn(r"(?m)^N\s*=\s*\d+", f"N = {n}", text, count=1)
    return new if count else None


# ---------------------------------------------------------------- running

def loop_actions(outdir):
    """Action names on the loop of the counterexample in error-graph.json:
    the nodes after the first earlier occurrence of the last node."""
    files = sorted(Path(outdir).glob("out/*/error-graph.json"))
    if not files:
        return None
    try:
        path = json.loads(files[-1].read_text())
    except (OSError, ValueError):
        return None
    if not isinstance(path, list) or not path:
        return None

    def key(entry):
        node = dict(entry.get("Node") or {})
        for k in ("stats", "witness", "failedInvariants"):
            node.pop(k, None)
        return json.dumps(node, sort_keys=True)

    last = key(path[-1])
    for i in range(len(path) - 1):
        if key(path[i]) == last:
            return [e.get("Name", "") for e in path[i + 1:]]
    return []  # stutter at the last state


def read_flags(case, label):
    """flags.txt applies to both specs; int_flags.txt / counter_flags.txt to
    one. One or more flags per line, `#` starts a comment."""
    flags = []
    for name in ("flags.txt", f"{label.lower()}_flags.txt"):
        f = case / name
        if f.exists():
            for line in f.read_text().splitlines():
                flags.extend(line.split("#", 1)[0].split())
    return flags


def run(text, label, assertion, flags=()):
    """Run one spec text; returns dict(verdict, detail, loop)."""
    tmp = tempfile.mkdtemp(prefix="cardtest-")
    try:
        spec = Path(tmp) / f"{label}.fizz"
        spec.write_text(text)
        try:
            r = subprocess.run([FIZZ, *flags, str(spec)], capture_output=True, text=True,
                               timeout=TIMEOUT, cwd=tmp)
            out = r.stdout + "\n" + r.stderr
        except subprocess.TimeoutExpired:
            return {"verdict": "TIMEOUT", "detail": f"> {TIMEOUT}s", "loop": None}
        except OSError as e:
            return {"verdict": "ERROR", "detail": f"cannot run {FIZZ}: {e}", "loop": None}

        if label == "Counter" and NOT_IMPL_RE.search(out):
            return {"verdict": "NOTIMPL", "detail": NOT_IMPL_RE.search(out).group(0), "loop": None}
        if "PASSED:" in out:
            return {"verdict": "PASS", "detail": "", "loop": None}
        if "FAILED:" in out:
            m = re.search(r"^Invariant:\s*(\S+)", out, re.M)
            detail = ""
            if m and m.group(1) != assertion:
                detail = f"failed {m.group(1)}"
            if "DEADLOCK" in out:
                detail = "deadlock"
            return {"verdict": "FAIL", "detail": detail, "loop": loop_actions(tmp)}
        m = re.search(r"^(panic:.*|.*Error.*)$", out, re.M)
        detail = m.group(1) if m else out.strip()[-200:]
        detail = re.sub(r"^[EWI]\d{4} [\d:.]+\s+\d+ \S+\] ", "", detail)  # glog prefix
        return {"verdict": "ERROR", "detail": detail[:120],
                "loop": None}
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


# ---------------------------------------------------------------- expected

def parse_expected(path):
    exp, reason = {}, ""
    if not path.exists():
        return exp, reason
    for raw in path.read_text().splitlines():
        line = re.sub(r"(^|\s)#.*", "", raw).strip()
        if not line:
            continue
        if line.startswith("reason:"):
            reason = line[len("reason:"):].strip()
            continue
        name, _, rest = line.partition(":")
        toks = rest.split()
        if not toks or toks[0] not in VERDICTS:
            continue
        e = {"verdict": toks[0], "imprecision": False, "int": None, "loop_includes": []}
        for t in toks[1:]:
            if t == "imprecision":
                e["imprecision"] = True
            elif t.startswith("int="):
                e["int"] = t[4:]
            elif t.startswith("loop_includes="):
                e["loop_includes"] = [a for a in t[len("loop_includes="):].split(",") if a]
        exp[name.strip()] = e
    return exp, reason


def combine_int(results):
    vs = [r["verdict"] for r in results]
    for v in ("ERROR", "FAIL", "TIMEOUT"):
        if v in vs:
            return v
    return "PASS"


def bad(v):
    return v in ("FAIL", "ERROR")


# ---------------------------------------------------------------- main

def check_case(case, sweep):
    counter_text = (case / "Counter.fizz").read_text()
    int_text = (case / "Int.fizz").read_text()
    expected, _ = parse_expected(case / "expected.txt")
    names = assertion_names(counter_text)
    int_names = set(assertion_names(int_text))

    int_variants = [("file", int_text)]
    if sweep:
        int_variants = [(f"N={n}", with_n(int_text, n)) for n in sweep]
        int_variants = [(l, t) for l, t in int_variants if t is not None] or [("file", int_text)]

    rows = []
    for name in names:
        c = run(only_assertion(counter_text, name), "Counter", name, read_flags(case, "Counter"))
        if name in int_names:
            ir = [dict(run(only_assertion(t, name), "Int", name, read_flags(case, "Int")), n=l) for l, t in int_variants]
            iv = combine_int(ir)
            idetail = " ".join(f"{r['n']}:{r['verdict']}" for r in ir) if len(ir) > 1 else ""
        else:
            iv, idetail = "-", "no Int counterpart"
        e = expected.get(name)
        problems, status = [], "OK"
        cv = c["verdict"]
        if cv == "NOTIMPL":
            status = "NOTIMPL"
        else:
            if bad(iv) and not bad(cv):
                problems.append(f"UNSOUND: Int {iv} but Counter {cv}")
            if bad(iv) and bad(cv) and iv != cv:
                problems.append(f"Int {iv} but Counter {cv}")
            if e is None:
                problems.append("no expected verdict")
            else:
                if e["int"] and iv != "-" and iv != e["int"] and not sweep:
                    problems.append(f"Int {iv}, expected int={e['int']}")
                if cv != e["verdict"]:
                    if e["imprecision"] and e["verdict"] == "FAIL" and cv == iv == "PASS":
                        status = "IMPROVED"
                    else:
                        problems.append(f"Counter {cv}, expected {e['verdict']}")
                if cv == "FAIL" and iv == "PASS" and not e["imprecision"]:
                    problems.append("Counter FAIL with Int PASS not marked imprecision")
                if cv == "FAIL" and e["loop_includes"]:
                    loop = c["loop"]
                    if loop is None:
                        problems.append("no counterexample graph to check loop")
                    else:
                        # "Recv" matches the role action "Server#1.Recv" too
                        names = set(loop) | {n.rsplit(".", 1)[-1] for n in loop}
                        miss = [a for a in e["loop_includes"] if a not in names]
                        if miss:
                            problems.append(f"loop {loop} lacks {miss}")
            if cv == "TIMEOUT" or iv == "TIMEOUT":
                problems.append("timeout")
            if problems:
                status = "MISMATCH"
        exp_s = "-" if e is None else e["verdict"] + ("~" if e["imprecision"] else "")
        notes = "; ".join(problems + [d for d in (c["detail"], idetail) if d])
        rows.append((case.name, name, iv, cv, exp_s, status, notes))
    return rows


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("filters", nargs="*", help="substrings of case names to run")
    ap.add_argument("--sweep", help="comma separated N values for Int.fizz, e.g. 2,3,4,5")
    ap.add_argument("-j", "--jobs", type=int, default=os.cpu_count() or 4)
    args = ap.parse_args()
    sweep = [int(x) for x in args.sweep.split(",")] if args.sweep else None

    cases = sorted(d for d in HERE.iterdir()
                   if d.is_dir() and (d / "Counter.fizz").exists() and (d / "Int.fizz").exists())
    if args.filters:
        cases = [c for c in cases if any(f in c.name for f in args.filters)]

    with concurrent.futures.ThreadPoolExecutor(args.jobs) as pool:
        results = list(pool.map(lambda c: check_case(c, sweep), cases))
    rows = [r for rs in results for r in rs]

    hdr = ("case", "assertion", "Int", "Counter", "expected", "status", "notes")
    widths = [max(len(str(r[i])) for r in rows + [hdr]) for i in range(6)]
    fmt = "  ".join(f"{{:<{w}}}" for w in widths) + "  {}"
    print(fmt.format(*hdr))
    print(fmt.format(*["-" * w for w in widths], "-----"))
    for r in rows:
        print(fmt.format(*r))

    counts = {}
    for r in rows:
        counts[r[5]] = counts.get(r[5], 0) + 1
    print("\n" + ", ".join(f"{k}: {v}" for k, v in sorted(counts.items())))
    if counts.get("MISMATCH"):
        return 1
    if counts.get("NOTIMPL"):
        print("cardinality.counter is not implemented in this build")
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
