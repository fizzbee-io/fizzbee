"""Tests for how assertion formulas parse: possibly as a soft keyword, its
operand rules and the atom limit."""
import contextlib
import io
import unittest

import proto.fizz_ast_pb2 as ast
from parser.parser import parse_file

PRELUDE = """
action Init:
    x = 0
    possibly = 1

func helper(v):
    return v
"""


def parse(code):
    with contextlib.redirect_stdout(io.StringIO()):
        return parse_file("test.fizz", PRELUDE + code)


def walk(f):
    yield f
    for a in f.args:
        yield from walk(a)


def shape(f):
    if f.op == ast.Formula.ATOM:
        return f"a{f.atom}"
    return ast.Formula.Op.Name(f.op) + "(" + ", ".join(shape(a) for a in f.args) + ")"


class PossiblyTest(unittest.TestCase):
    def formula(self, ret):
        file = parse(f"assertion A:\n    return {ret}\n")
        return shape(file.invariants[0].formula)

    def error(self, code):
        with self.assertRaises(SyntaxError) as cm:
            parse(code)
        return str(cm.exception)

    def test_operator(self):
        for ret, want in {
            "possibly x == 1": "POSSIBLY(a0)",
            "possibly(x == 1)": "POSSIBLY(a0)",
            "exists eventually x == 1": "POSSIBLY(a0)",
            "not possibly x == 1": "NOT(POSSIBLY(a0))",
            "always possibly (x == 1)": "ALWAYS(POSSIBLY(a0))",
            "always (x == 0 implies possibly x == 1)": "ALWAYS(IMPLIES(a0, POSSIBLY(a1)))",
            "always (x == 0 and (possibly x == 1) or x == 2)": "ALWAYS(OR(AND(a0, POSSIBLY(a1)), a2))",
            "possibly (x == 0 and possibly x == 1)": "POSSIBLY(AND(a0, POSSIBLY(a1)))",
            "(possibly x == 1) until x == 1": "UNTIL(POSSIBLY(a0), a0)",
            # Line breaks inside parentheses don't end the return.
            "always (x == 0 implies\n        possibly(x == 1))": "ALWAYS(IMPLIES(a0, POSSIBLY(a1)))",
        }.items():
            with self.subTest(ret):
                self.assertEqual(want, self.formula(ret))

    def test_plain_returns(self):
        # possibly stays usable as a name: these are plain returns.
        file = parse("""
func f():
    return possibly(1)

func g():
    return helper(possibly(1))

func h():
    return [possibly(v) for v in range(2)]

func k():
    return helper(
        possibly(1))

always assertion Var:
    return possibly == 1

always assertion VarAnd:
    return possibly and x == 0

always assertion Nested:
    return helper(possibly(1)) == 1
""")
        for inv in file.invariants:
            self.assertFalse(inv.HasField("formula"), inv.name)
        returns = {inv.name: inv.block.stmts[-1].return_stmt.py_expr for inv in file.invariants}
        self.assertEqual("possibly == 1", returns["Var"])
        funcs = {fn.name: fn.block.stmts[-1].return_stmt.py_expr for fn in file.functions}
        self.assertEqual("possibly(1)", funcs["f"])
        self.assertEqual("helper(possibly(1))", funcs["g"])
        self.assertEqual("[possibly(v) for v in range(2)]", funcs["h"])
        self.assertEqual("helper(\n        possibly(1))", funcs["k"])

    def test_errors(self):
        for code, want in {
            "assertion A:\n    return possibly (always x == 1)\n": "can't contain always",
            "assertion A:\n    return possibly (eventually x == 1)\n": "drop the `eventually`",
            "assertion A:\n    return exists always x == 1\n": "only used as `exists eventually`",
            "assertion A:\n    return possibly x == 1 and x == 2\n": "`possibly` here applies only to `x == 1`",
            "func f():\n    return (possibly(x)) implies x\n": "can only be used in the return of an assertion",
            "func f():\n    return always possibly(x)\n": "can only be used in the return of an assertion",
        }.items():
            with self.subTest(code):
                self.assertIn(want, self.error(code))

    def test_atom_limit(self):
        # Each distinct condition is one atom, and each use of possibly is
        # one more, nested ones included.
        def formula(n):
            # n separate conditions, then two more inside two uses of
            # possibly: n + 4 atoms in all.
            separate = " and ".join(f"(always x != {i})" for i in range(n))
            return f"assertion A:\n    return {separate} and always possibly (x == 99 and possibly x == 98)\n"
        f = parse(formula(60)).invariants[0].formula
        conditions = len({a.atom for a in walk(f) if a.op == ast.Formula.ATOM})
        uses = len([a for a in walk(f) if a.op == ast.Formula.POSSIBLY])
        self.assertEqual((62, 2), (conditions, uses))
        self.assertEqual(64, conditions + uses)  # the limit, accepted
        self.assertIn("needs 65 atoms", self.error(formula(61)))


if __name__ == "__main__":
    unittest.main()
