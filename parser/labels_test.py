"""Tests for parsing labeled blocks (`txn:`)."""
import contextlib
import io
import unittest

import proto.fizz_ast_pb2 as ast
from parser.parser import parse_file


def parse(body, stderr=None):
    src = "action Init:\n    x = 0\n\n" + body
    err = stderr if stderr is not None else io.StringIO()
    with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(err):
        return parse_file("test.fizz", src)


def action_block(f, name):
    return [a for a in f.actions if a.name == name][0].block


class LabelsTest(unittest.TestCase):
    def test_bare_label_inherits_flow(self):
        b = action_block(parse("serial action A:\n    x = 1\n    txn:\n        x = 2\n        x = 3\n"), "A")
        lb = b.stmts[1].block
        self.assertEqual(list(lb.labels), ["txn"])
        self.assertEqual(lb.flow, ast.Flow.FLOW_UNKNOWN)
        self.assertEqual(len(lb.stmts), 2)

    def test_one_label_per_block(self):
        # `cs, txn:` is not a labeled block; the parser reports a syntax error.
        err = io.StringIO()
        b = action_block(parse("serial action A:\n    x = 1\n    cs, txn:\n        x = 2\n", stderr=err), "A")
        self.assertFalse(any(s.block.labels for s in b.stmts))
        self.assertIn("mismatched input", err.getvalue())

    def test_nested_labels_stay_nested(self):
        b = action_block(parse("serial action A:\n    x = 1\n    txn:\n        cs:\n            x = 2\n"), "A")
        outer = b.stmts[1].block
        self.assertEqual(list(outer.labels), ["txn"])
        self.assertEqual(list(outer.stmts[0].block.labels), ["cs"])

    def test_flow_word_before_label_is_rejected(self):
        # A label always inherits the flow; nest a flow block for another one.
        with self.assertRaises(Exception):
            parse("serial action A:\n    x = 1\n    atomic txn:\n        x = 2\n")

    def test_lone_nested_block_keeps_its_flow(self):
        # The label must not swallow a lone atomic block's flow.
        b = action_block(parse("serial action A:\n    x = 1\n    txn:\n        atomic:\n            x = 2\n            x = 3\n"), "A")
        lb = b.stmts[1].block
        self.assertEqual(list(lb.labels), ["txn"])
        self.assertEqual(len(lb.stmts), 1)
        self.assertEqual(lb.stmts[0].block.flow, ast.Flow.FLOW_ATOMIC)

    def test_annotated_assignment_is_not_a_label(self):
        b = action_block(parse("atomic action A:\n    y: int = 1\n    x = y\n"), "A")
        self.assertEqual(b.stmts[0].py_stmt.code.replace(" ", ""), "y:int=1")

    def test_keyword_is_not_a_label(self):
        # `atomic:` stays a flow block, not a label named atomic.
        b = action_block(parse("serial action A:\n    x = 1\n    atomic:\n        x = 2\n"), "A")
        self.assertEqual(list(b.stmts[1].block.labels), [])
        self.assertEqual(b.stmts[1].block.flow, ast.Flow.FLOW_ATOMIC)


if __name__ == "__main__":
    unittest.main()
