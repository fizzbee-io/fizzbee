#!/usr/bin/env python3
"""A signal to the `fizz` wrapper's pid must stop the fizzbee it started.

    python3 tests/test_wrapper_signals.py    (FIZZ overrides the wrapper)
"""

import os
import signal
import subprocess
import tempfile
import time
import unittest
from pathlib import Path

FIZZ = os.environ.get("FIZZ", str(Path(__file__).resolve().parent.parent / "fizz"))

# Too big for model checking or unlimited simulation to finish in the test.
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


class WrapperSignalTest(unittest.TestCase):
    def run_and_kill(self, *flags):
        with tempfile.TemporaryDirectory() as d:
            spec = Path(d, "Slow.fizz")
            spec.write_text(SLOW_SPEC)
            # pgrep -f on the json path finds every fizzbee of this run.
            pids = lambda: subprocess.run(["pgrep", "-f", str(spec.with_suffix(".json"))],
                                          capture_output=True, text=True).stdout.split()
            # A new session keeps the signal to the wrapper's pid alone.
            proc = subprocess.Popen([FIZZ, *flags, str(spec)], cwd=d, start_new_session=True,
                                    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            try:
                self.assertTrue(wait_for(pids, 60), "fizzbee never started")
                proc.send_signal(signal.SIGTERM)
                proc.wait(timeout=30)
                self.assertTrue(wait_for(lambda: not pids(), 10), f"orphaned: {pids()}")
            finally:
                for pid in pids():
                    os.kill(int(pid), signal.SIGKILL)

    def test_model_check(self):
        self.run_and_kill()

    def test_parallel_simulation(self):
        self.run_and_kill("-x", "--parallel", "2")


def wait_for(cond, timeout):
    deadline = time.monotonic() + timeout
    while not cond():
        if time.monotonic() > deadline:
            return False
        time.sleep(0.1)
    return True


if __name__ == "__main__":
    unittest.main()
