#!/usr/bin/env python3
"""
Signals sent to the `fizz` wrapper's pid must reach the fizzbee processes it
starts, so a supervisor that kills the wrapper leaves no orphans behind.

    python3 tests/test_wrapper_signals.py

FIZZ overrides the wrapper under test, as in test_references.py.
"""

import os
import shutil
import signal
import subprocess
import tempfile
import time
import unittest
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
FIZZ = os.environ.get("FIZZ", str(REPO / "fizz"))

# Big enough that neither model checking nor unlimited simulation finishes
# within the test.
SLOW_SPEC = """\
---
deadlock_detection: false
options:
  max_actions: 1000
---

action Init:
    a = 0
    b = 0
    c = 0

atomic action IncA:
    a = (a + 1) % 200

atomic action IncB:
    b = (b + 1) % 200

atomic action IncC:
    c = (c + 1) % 200
"""


def fizzbee_pids(json_path: Path) -> list[int]:
    r = subprocess.run(["pgrep", "-f", str(json_path)], capture_output=True, text=True)
    return [int(p) for p in r.stdout.split()]


def wait_for(cond, timeout: float) -> bool:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if cond():
            return True
        time.sleep(0.1)
    return cond()


class WrapperSignalTest(unittest.TestCase):
    def setUp(self):
        self.dir = Path(tempfile.mkdtemp(prefix="fizz-signals-"))
        self.spec = self.dir / "Slow.fizz"
        self.spec.write_text(SLOW_SPEC)
        self.json = self.dir / "Slow.json"
        self.proc = None

    def tearDown(self):
        for pid in fizzbee_pids(self.json):
            try:
                os.kill(pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        if self.proc is not None and self.proc.poll() is None:
            self.proc.kill()
            self.proc.wait()
        shutil.rmtree(self.dir, ignore_errors=True)

    def start(self, *flags: str) -> None:
        # A new session keeps the signal off the test's own process group,
        # so only the wrapper's pid gets it, as with `kill <pid>`.
        self.proc = subprocess.Popen(
            [FIZZ, *flags, str(self.spec)],
            cwd=self.dir,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            start_new_session=True,
        )
        started = wait_for(lambda: fizzbee_pids(self.json), timeout=60)
        self.assertTrue(started, "fizzbee never started")

    def assert_term_leaves_no_orphans(self) -> None:
        os.kill(self.proc.pid, signal.SIGTERM)
        self.proc.wait(timeout=30)
        gone = wait_for(lambda: not fizzbee_pids(self.json), timeout=30)
        self.assertTrue(gone, f"fizzbee still running: {fizzbee_pids(self.json)}")

    def test_model_check(self):
        self.start()
        self.assert_term_leaves_no_orphans()

    def test_parallel_simulation(self):
        self.start("-x", "--parallel", "2")
        self.assertGreaterEqual(len(fizzbee_pids(self.json)), 1)
        self.assert_term_leaves_no_orphans()


if __name__ == "__main__":
    unittest.main()
