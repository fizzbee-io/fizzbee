#!/usr/bin/env python3
"""Check LLM claims against a FizzBee policy spec, Bedrock Automated Reasoning style.

Each claim in claims.json has a premise and a claim, both written as FizzBee
(Python-syntax) boolean expressions over the policy's state variables. For
each one, the policy spec is model checked twice:

  true probe:   is a world reachable where  premises and claim      hold?
  false probe:  is a world reachable where  premises and not claim  hold?

A probe is written as `always assertion: return not (known and X)`. If it
FAILS, X is reachable and the counterexample trace is the witness scenario
(AWS calls these claimsTrueScenario / claimsFalseScenario). This is the same
as `exists assertion: return known and X`, but the failure gives a trace.

  true reachable, false not   -> VALID        (claim holds in every world)
  true and false reachable    -> SATISFIABLE  (holds in some worlds only)
  false reachable, true not   -> INVALID      (claim holds in no world)
  neither reachable           -> IMPOSSIBLE   (premises contradict the policy)
  unknown variable in claim   -> NO_TRANSLATIONS

Usage:
  python3 check_claims.py [--fizz PATH] [--spec HrLeavePolicy.fizz] [--claims claims.json]

The fizz binary is taken from --fizz, then $FIZZ, then `fizz` on PATH.
"""

import argparse
import ast
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
STATE_VAR = re.compile(r"^    (\w+)\s*=", re.MULTILINE)
CONSTANTS = {"True", "False", "None"}


def state_vars(spec_text):
    """Variables assigned in Init. These are the policy's vocabulary."""
    init = spec_text.split("action Init:", 1)[1]
    init = re.split(r"\n\S", init, maxsplit=1)[0]
    return set(STATE_VAR.findall(init)) - {"known"}


def unknown_names(expr, vocabulary):
    names = {n.id for n in ast.walk(ast.parse(expr, mode="eval")) if isinstance(n, ast.Name)}
    return sorted(names - vocabulary - CONSTANTS)


def reachable(fizz, spec_text, expr):
    """Returns (reachable, witness_state_json_or_None)."""
    probe = spec_text + "\n\nalways assertion Probe:\n    return not (known and (%s))\n" % expr
    with tempfile.TemporaryDirectory() as tmp:
        path = os.path.join(tmp, "Probe.fizz")
        with open(path, "w") as f:
            f.write(probe)
        out = subprocess.run([fizz, path], cwd=tmp, capture_output=True, text=True).stdout
    if "PASSED:" in out:
        return False, None
    if "FAILED: Model checker failed. Invariant:" in out:
        states = re.findall(r"^state: (.*)$", out, re.MULTILINE)
        witness = json.loads(states[-1]) if states else None
        if witness:
            witness.pop("known", None)
        return True, witness
    sys.exit("Unexpected fizz output for probe %r:\n%s" % (expr, out))


def verdict(fizz, spec_text, vocabulary, c):
    missing = unknown_names(c["premises"], vocabulary) + unknown_names(c["claim"], vocabulary)
    if missing:
        return "NO_TRANSLATIONS", {"untranslated": missing}
    p, q = "(%s)" % c["premises"], "(%s)" % c["claim"]
    t, t_witness = reachable(fizz, spec_text, "%s and %s" % (p, q))
    f, f_witness = reachable(fizz, spec_text, "%s and not %s" % (p, q))
    result = {(True, False): "VALID", (True, True): "SATISFIABLE",
              (False, True): "INVALID", (False, False): "IMPOSSIBLE"}[(t, f)]
    details = {}
    if t_witness:
        details["claimsTrueScenario"] = t_witness
    if f_witness:
        details["claimsFalseScenario"] = f_witness
    return result, details


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--fizz", default=os.environ.get("FIZZ") or shutil.which("fizz"))
    ap.add_argument("--spec", default=os.path.join(HERE, "HrLeavePolicy.fizz"))
    ap.add_argument("--claims", default=os.path.join(HERE, "claims.json"))
    ap.add_argument("-v", "--verbose", action="store_true", help="print witness scenarios")
    args = ap.parse_args()
    if not args.fizz:
        sys.exit("fizz not found: pass --fizz or set $FIZZ")

    with open(args.spec) as f:
        spec_text = f.read()
    with open(args.claims) as f:
        claims = json.load(f)
    vocabulary = state_vars(spec_text)

    # A policy whose rules conflict allows no world at all, so every claim
    # would come back IMPOSSIBLE. Check that first.
    consistent, _ = reachable(args.fizz, spec_text, "True")
    if not consistent:
        sys.exit("IMPOSSIBLE: the policy rules contradict each other (no world satisfies them)")

    failures = 0
    for c in claims:
        result, details = verdict(args.fizz, spec_text, vocabulary, c)
        ok = result == c["expected"]
        failures += not ok
        print("%-4s %-16s expected %-16s %s  %s" % (
            c["id"], result, c["expected"], "ok  " if ok else "FAIL", c["answer"]))
        if args.verbose:
            for k, v in details.items():
                print("       %s: %s" % (k, json.dumps(v, sort_keys=True)))
    print("\n%d/%d claims matched the expected verdict" % (len(claims) - failures, len(claims)))
    sys.exit(1 if failures else 0)


if __name__ == "__main__":
    main()
