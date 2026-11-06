"""Tests for parsing @assume on assertions."""
import contextlib
import io
import unittest

from parser.parser import parse_file

PRELUDE = """
action Init:
    x = 0

atomic action A:
    x = 1

atomic action B:
    x = 0
"""

ASSERTION = """
eventually always assertion P:
    return x == 0
"""


def parse(decorators, assertion=ASSERTION):
    with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
        return parse_file("test.fizz", PRELUDE + decorators + assertion)


class AssumeTest(unittest.TestCase):
    def test_all_lists(self):
        f = parse('@assume(finite=["A"], weak_fair=["B"], strong_fair=[], unfair=["Role.C"])\n')
        a = f.invariants[0].assumption
        self.assertEqual(list(a.finite), ["A"])
        self.assertEqual(list(a.weak_fair), ["B"])
        self.assertEqual(list(a.strong_fair), [])
        self.assertEqual(list(a.unfair), ["Role.C"])

    def test_no_decorator(self):
        self.assertFalse(parse("").invariants[0].HasField("assumption"))

    def test_formula_assertion(self):
        f = parse('@assume(finite=["A"])\n', "\nassertion F:\n    return always (x == 1 implies eventually x == 0)\n")
        self.assertTrue(f.invariants[0].HasField("formula"))
        self.assertEqual(list(f.invariants[0].assumption.finite), ["A"])

    def test_errors(self):
        for decorators, message in [
            ('@assume\n', "needs at least one"),
            ('@assume()\n', "needs at least one"),
            ('@assume(["A"])\n', "only keyword arguments"),
            ('@assume(stops=["A"])\n', "no argument stops"),
            ('@assume(finite="A")\n', "must be a list"),
            ('@assume(finite=[A])\n', "must be a list"),
            ('@assume(finite=["A"], finite=["B"])\n', "more than once"),
            ('@assume(finite=["A"])\n@assume(unfair=["B"])\n', "more than one @assume"),
            ('@other(finite=["A"])\n', "only the @assume decorator"),
        ]:
            with self.subTest(decorators=decorators):
                with self.assertRaises(Exception) as cm:
                    parse(decorators)
                self.assertIn(message, str(cm.exception))


if __name__ == "__main__":
    unittest.main()
