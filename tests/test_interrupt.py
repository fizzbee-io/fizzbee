#!/usr/bin/env python3
"""
An interrupted model checker stops without checking the partial graph.

    python3 tests/test_interrupt.py

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

# Big enough that exploration is still running when the signal arrives.
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


class InterruptTest(unittest.TestCase):
    def setUp(self):
        self.dir = Path(tempfile.mkdtemp(prefix="fizz-interrupt-"))
        self.spec = self.dir / "Slow.fizz"
        self.spec.write_text(SLOW_SPEC)
        self.log = self.dir / "log"
        self.proc = None

    def tearDown(self):
        if self.proc is not None and self.proc.poll() is None:
            os.killpg(self.proc.pid, signal.SIGKILL)
            self.proc.wait()
        shutil.rmtree(self.dir, ignore_errors=True)

    def start(self) -> None:
        # Its own process group, so a signal to the group reaches the wrapper
        # and fizzbee as Ctrl-C in a terminal would.
        with open(self.log, "w") as out:
            self.proc = subprocess.Popen(
                [FIZZ, str(self.spec)],
                cwd=self.dir,
                stdout=out,
                stderr=subprocess.STDOUT,
                start_new_session=True,
            )
        deadline = time.monotonic() + 60
        while "Nodes:" not in self.log.read_text():
            self.assertIsNone(self.proc.poll(), "fizz exited before exploring")
            self.assertLess(time.monotonic(), deadline, "exploration never started")
            time.sleep(0.1)
        time.sleep(1)

    def output(self) -> str:
        return self.log.read_text()

    def test_stop_skips_checks_on_partial_graph(self):
        self.start()
        os.killpg(self.proc.pid, signal.SIGINT)
        self.proc.wait(timeout=60)
        out = self.output()
        self.assertIn("Model checker stopped", out)
        self.assertNotIn("Valid Nodes:", out)
        self.assertNotIn("PASSED", out)


if __name__ == "__main__":
    unittest.main()
