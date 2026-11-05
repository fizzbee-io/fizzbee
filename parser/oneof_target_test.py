"""Tests for oneof target classification and condition rewriting."""
import contextlib
import io
import unittest

from google.protobuf.message import Message

import proto.fizz_ast_pb2 as ast
from parser import oneof_target
from parser.oneof_target import CHOICE_VAR, classify_target, rewrite_condition
from parser.parser import parse_file


def parse(code):
    with contextlib.redirect_stdout(io.StringIO()):
        return parse_file("test.fizz", code)


def any_stmts(msg):
    if isinstance(msg, ast.AnyStmt):
        yield msg
    for field, value in msg.ListFields():
        if field.message_type is None:
            continue
        # A repeated field's value is a container, not a message.
        for v in ([value] if isinstance(value, Message) else value):
            yield from any_stmts(v)


class ClassifyTest(unittest.TestCase):
    def test_kinds(self):
        self.assertEqual(classify_target("x"), ast.ANY_TARGET_KIND_NAME)
        self.assertEqual(classify_target("self.x"), ast.ANY_TARGET_KIND_ATTRIBUTE)
        self.assertEqual(classify_target("r.a.b"), ast.ANY_TARGET_KIND_ATTRIBUTE)
        self.assertEqual(classify_target("d[k]"), ast.ANY_TARGET_KIND_SUBSCRIPT)
        self.assertEqual(classify_target('self.d["k"]'), ast.ANY_TARGET_KIND_SUBSCRIPT)
        self.assertEqual(classify_target("arr[i + 1]"), ast.ANY_TARGET_KIND_SUBSCRIPT)
        self.assertEqual(classify_target("f()"), ast.ANY_TARGET_KIND_UNKNOWN)
        self.assertEqual(classify_target("1"), ast.ANY_TARGET_KIND_UNKNOWN)


class RewriteTest(unittest.TestCase):
    def test_attribute(self):
        self.assertEqual(rewrite_condition("self.z", "self.z != 2"), f"{CHOICE_VAR} != 2")

    def test_every_occurrence(self):
        self.assertEqual(
            rewrite_condition("self.z", "self.z > 0 and self.z not in seen"),
            f"{CHOICE_VAR} > 0 and {CHOICE_VAR} not in seen")

    def test_subscript_any_quotes_and_spacing(self):
        self.assertEqual(rewrite_condition('d["k"]', "d['k'] != None"), f"{CHOICE_VAR} != None")
        self.assertEqual(rewrite_condition("arr[i]", "arr[ i ] > 1"), f"{CHOICE_VAR} > 1")

    def test_keeps_other_text(self):
        self.assertEqual(
            rewrite_condition("r.x", 'len(r.x) > n and "r.x" != r.x'),
            f'len({CHOICE_VAR}) > n and "r.x" != {CHOICE_VAR}')

    def test_multiline_and_unicode(self):
        cond = "(r.x != 'é' and\n     r.x > 0)"
        self.assertEqual(rewrite_condition("r.x", cond), f"({CHOICE_VAR} != 'é' and\n     {CHOICE_VAR} > 0)")

    def test_attribute_of_target_is_rewritten(self):
        # self.z.y reads through the new value of self.z.
        self.assertEqual(rewrite_condition("self.z", "self.z.y > 0"), f"{CHOICE_VAR}.y > 0")

    def test_no_reference_to_target(self):
        self.assertEqual(rewrite_condition("self.z", "n > 0"), "n > 0")

    def test_other_reference_to_base_falls_back(self):
        # The condition can see the target through another path, so it must
        # run after the assignment.
        self.assertIsNone(rewrite_condition("self.z", "self.z != self.y"))
        self.assertIsNone(rewrite_condition("self.z", "self.ok()"))
        self.assertIsNone(rewrite_condition("d[k]", "d[j] != 1"))
        self.assertIsNone(rewrite_condition("d[k]", "len(d) > 1"))
        self.assertIsNone(rewrite_condition("arr[1]", "arr[1] != arr[0]"))

    def test_existing_choice_var_falls_back(self):
        self.assertIsNone(rewrite_condition("r.x", f"{CHOICE_VAR} > 1"))

    def test_unparsable_falls_back(self):
        self.assertIsNone(rewrite_condition("r.x", "r.x >"))


class ParserTest(unittest.TestCase):
    def stmts(self, body):
        file = parse("action Init:\n    x = 0\n    d = {}\n\natomic action A:\n" + body)
        return {s.loop_vars[0]: s for s in any_stmts(file)}

    def test_name_target(self):
        s = self.stmts("    y = oneof [1, 2] if y != x\n")["y"]
        self.assertEqual(s.target_kind, ast.ANY_TARGET_KIND_NAME)
        self.assertEqual(s.choice_var, "y")
        self.assertEqual(s.condition_expr.py_expr, "y != x")

    def test_name_target_without_condition(self):
        s = self.stmts("    y = oneof [1, 2]\n")["y"]
        self.assertEqual(s.choice_var, "y")

    def test_field_target_rewritten(self):
        s = self.stmts('    d["k"] = oneof [1, 2] if d["k"] != x\n')['d["k"]']
        self.assertEqual(s.target_kind, ast.ANY_TARGET_KIND_SUBSCRIPT)
        self.assertEqual(s.choice_var, CHOICE_VAR)
        self.assertEqual(s.condition_expr.py_expr, f"{CHOICE_VAR} != x")
        self.assertEqual(s.condition, 'd["k"] != x')

    def test_field_target_fallback(self):
        s = self.stmts('    d["k"] = oneof [1, 2] if len(d) > 0\n')['d["k"]']
        self.assertEqual(s.target_kind, ast.ANY_TARGET_KIND_SUBSCRIPT)
        self.assertEqual(s.choice_var, "")
        self.assertEqual(s.condition_expr.py_expr, "len(d) > 0")

    def test_field_target_without_condition(self):
        s = self.stmts('    d["k"] = oneof [1, 2]\n')['d["k"]']
        self.assertEqual(s.choice_var, "")

    def test_block_form(self):
        s = self.stmts("    oneof y in [1, 2]:\n        x = y\n")["y"]
        self.assertEqual(s.target_kind, ast.ANY_TARGET_KIND_NAME)
        self.assertEqual(s.choice_var, "y")

    def test_block_form_rejects_field(self):
        with self.assertRaisesRegex(Exception, "plain name"):
            self.stmts('    oneof d["k"] in [1, 2]:\n        x = 1\n')


if __name__ == "__main__":
    unittest.main()
