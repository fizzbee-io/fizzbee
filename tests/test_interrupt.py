#!/usr/bin/env python3
"""An interrupted model checker stops without checking the partial graph.

    python3 tests/test_interrupt.py    (FIZZ overrides the wrapper)
"""

import os
import signal
import subprocess
import tempfile
import time
import unittest
from pathlib import Path

FIZZ = os.environ.get("FIZZ", str(Path(__file__).resolve().parent.parent / "fizz"))

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
    def test_stop_skips_checks_on_partial_graph(self):
        with tempfile.TemporaryDirectory() as d:
            Path(d, "Slow.fizz").write_text(SLOW_SPEC)
            log = Path(d, "log")
            # Its own process group, so the signal reaches the wrapper and
            # fizzbee as Ctrl-C in a terminal would.
            with open(log, "w") as out:
                proc = subprocess.Popen([FIZZ, "Slow.fizz"], cwd=d, stdout=out,
                                        stderr=subprocess.STDOUT, start_new_session=True)
            try:
                deadline = time.monotonic() + 60
                while "Nodes:" not in log.read_text():
                    self.assertIsNone(proc.poll(), "fizz exited before exploring")
                    self.assertLess(time.monotonic(), deadline, "exploration never started")
                    time.sleep(0.1)
                time.sleep(1)
                os.killpg(proc.pid, signal.SIGINT)
                proc.wait(timeout=60)
            finally:
                if proc.poll() is None:
                    os.killpg(proc.pid, signal.SIGKILL)
            out = log.read_text()
            self.assertIn("Model checker stopped", out)
            self.assertNotIn("Valid Nodes:", out)
            self.assertNotIn("PASSED", out)


if __name__ == "__main__":
    unittest.main()
