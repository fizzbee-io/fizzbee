"""Tests for import statements.

The random test is differential: it generates import statements, parses them
with the fizz parser and with Python's own `ast` module, and checks that both
agree on which modules, members and aliases are imported. The fizz grammar
took its import rules from Python's, so Python is the syntax oracle. It is
not the semantic one: FizzBee's import rules are stricter (imports come
first in the file, are resolved eagerly, and only at the top level).
"""
import ast as pyast
import contextlib
import io
import random
import unittest

from parser.parser import parse_file

# Identifiers that are not keywords in fizz or Python.
NAMES = ["a", "b", "util", "queue", "threads", "lib", "net", "x1", "Helper",
         "math", "sym", "inside", "count", "_private", "deep_name"]

BODY = "\naction Init:\n    x = 0\n"


def parse(code):
    with contextlib.redirect_stdout(io.StringIO()):
        return parse_file("test.fizz", code)


def fizz_imports(file):
    out = []
    for imp in file.imports:
        out.append((imp.path, imp.alias, tuple((n.name, n.alias) for n in imp.names)))
    return out


def python_imports(code):
    out = []
    for node in pyast.parse(code).body:
        if isinstance(node, pyast.Import):
            for alias in node.names:
                out.append((alias.name, alias.asname or "", ()))
        elif isinstance(node, pyast.ImportFrom):
            names = tuple((a.name, a.asname or "") for a in node.names)
            out.append((node.module, "", names))
    return out


def gen_dotted(rng):
    return ".".join(rng.choice(NAMES) for _ in range(rng.choice([1, 1, 1, 2, 3])))


def gen_alias(rng, required=False):
    if required or rng.random() < 0.4:
        return " as " + rng.choice(NAMES)
    return ""


def gen_import(rng):
    if rng.random() < 0.5:
        parts = []
        for _ in range(rng.randint(1, 3)):
            path = gen_dotted(rng)
            parts.append(path + gen_alias(rng, required="." in path))
        return "import " + ", ".join(parts)
    members = [rng.choice(NAMES) + gen_alias(rng) for _ in range(rng.randint(1, 4))]
    joined = ", ".join(members)
    if rng.random() < 0.3:
        joined = "(" + joined + ("," if rng.random() < 0.5 else "") + ")"
    return "from " + gen_dotted(rng) + " import " + joined


class RandomImportTest(unittest.TestCase):
    def test_matches_python(self):
        rng = random.Random(438317)
        for i in range(300):
            lines = [gen_import(rng) for _ in range(rng.randint(1, 4))]
            code = "\n".join(lines) + "\n"
            with self.subTest(i=i, code=code):
                self.assertEqual(fizz_imports(parse(code + BODY)), python_imports(code))


class ImportTest(unittest.TestCase):
    def test_imports_do_not_become_statements(self):
        file = parse("import math\nfrom threads import inside as at\n" + BODY)
        self.assertEqual(len(file.stmts), 0)
        self.assertEqual(fizz_imports(file), [("math", "", ()), ("threads", "", (("inside", "at"),))])

    def test_imports_before_other_statements(self):
        file = parse("# header\nimport math\n\nN = 3\n" + BODY)
        self.assertEqual(fizz_imports(file), [("math", "", ())])
        self.assertEqual(len(file.stmts), 1)

    def test_line_numbers(self):
        file = parse("\n\nimport math\n" + BODY)
        self.assertEqual(file.imports[0].source_info.start.line, 3)

    def assertRejected(self, code, message):
        with self.assertRaises(Exception) as cm:
            parse(code)
        self.assertIn(message, str(cm.exception))

    def test_rejected_forms(self):
        self.assertRejected("from math import *\n" + BODY, "import *")
        self.assertRejected("from . import util\n" + BODY, "relative imports")
        self.assertRejected("from .lib import util\n" + BODY, "relative imports")
        self.assertRejected("import lib.queue\n" + BODY, "needs an alias")
        self.assertRejected("import math; y = 1\n" + BODY, "its own line")
        self.assertRejected("N = 3\nimport math\n" + BODY, "must come before")
        self.assertRejected(BODY + "import math\n", "must come before")
        self.assertRejected("func f():\n    return 1\nimport math\n" + BODY, "must come before")

    def test_rejected_inside_blocks(self):
        self.assertRejected("action Init:\n    import math\n    x = 0\n", "top level")
        self.assertRejected("func f():\n    import math\n    return 1\n" + BODY, "top level")
        self.assertRejected("role R:\n    import math\n    action Init:\n        self.x = 0\n", "top level")
        self.assertRejected(BODY + "    if x == 0:\n        import math\n", "top level")


if __name__ == "__main__":
    unittest.main()
