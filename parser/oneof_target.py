"""Target classification and condition rewriting for `TARGET = oneof S if C`.

The model checker forks one process per candidate of a `oneof`. When the
statement has a condition, evaluating it in the parent before forking avoids
creating forks that the condition rejects. For that the condition must read the
candidate through a plain name the checker can bind:

    x = oneof S if C(x)              # C already reads the candidate through x
    self.z = oneof S if C(self.z)    # rewritten to C(__fizz_oneof_choice__)

The second form behaves like

    __c = oneof [__c for __c in S if C(__c)]
    self.z = __c

run as one step. When the rewrite can't be shown to be equivalent, the
condition is left alone and the checker evaluates it in each fork after the
assignment, as before.
"""
import ast as pyast

import proto.fizz_ast_pb2 as fizz_ast

# Bound by the checker to the candidate value. Starts with "__", which spec
# identifiers can't use in practice, so it can't collide with a spec name.
CHOICE_VAR = "__fizz_oneof_choice__"


def _parse_expr(code):
    # Parenthesize so that a condition split over several lines parses.
    return pyast.parse("(" + code + ")", mode="eval").body


def classify_target(target):
    """Returns the AnyTargetKind of a oneof target's source text."""
    try:
        node = _parse_expr(target)
    except SyntaxError:
        return fizz_ast.ANY_TARGET_KIND_UNKNOWN
    if isinstance(node, pyast.Name):
        return fizz_ast.ANY_TARGET_KIND_NAME
    if isinstance(node, pyast.Attribute):
        return fizz_ast.ANY_TARGET_KIND_ATTRIBUTE
    if isinstance(node, pyast.Subscript):
        return fizz_ast.ANY_TARGET_KIND_SUBSCRIPT
    return fizz_ast.ANY_TARGET_KIND_UNKNOWN


def _root_name(node):
    while isinstance(node, (pyast.Attribute, pyast.Subscript)):
        node = node.value
    return node.id if isinstance(node, pyast.Name) else None


def _key(node):
    # Structural identity, ignoring Load/Store context and positions.
    return pyast.dump(node, annotate_fields=False, include_attributes=False) \
        .replace("Store()", "Load()")


def _offsets(code):
    """Maps (lineno, col_offset in UTF-8 bytes) of the parenthesized source to
    an index into code."""
    lines = ("(" + code + ")").split("\n")
    starts = []
    pos = -1  # the added "(" is at index -1 of code
    for line in lines:
        starts.append(pos)
        pos += len(line) + 1

    def to_index(lineno, byte_col):
        line = lines[lineno - 1]
        col = len(line.encode("utf-8")[:byte_col].decode("utf-8"))
        return starts[lineno - 1] + col

    return to_index


def rewrite_condition(target, condition):
    """Rewrites a oneof condition to read the candidate through CHOICE_VAR.

    Every occurrence of the target expression in the condition is replaced by
    CHOICE_VAR. Returns the rewritten condition text, or None when the
    condition can't be filtered in the parent: it doesn't parse, or it still
    refers to the target's base object (for example `self.ok()` or `d[j]` for
    target `d[k]`), which could read the target through another path.
    """
    try:
        target_node = _parse_expr(target)
        cond_node = _parse_expr(condition)
    except SyntaxError:
        return None
    root = _root_name(target_node)
    if root is None:
        return None
    target_key = _key(target_node)

    spans = []

    def visit(node):
        if isinstance(node, pyast.expr) and _key(node) == target_key:
            spans.append(node)
            return
        if isinstance(node, pyast.Name) and node.id in (root, CHOICE_VAR):
            spans.append(None)  # marks a remaining reference to the base object
            return
        for child in pyast.iter_child_nodes(node):
            visit(child)

    visit(cond_node)
    if any(s is None for s in spans):
        return None

    to_index = _offsets(condition)
    out = condition
    # Replace from the end so earlier indexes stay valid.
    for node in sorted(spans, key=lambda n: (n.lineno, n.col_offset), reverse=True):
        start = to_index(node.lineno, node.col_offset)
        end = to_index(node.end_lineno, node.end_col_offset)
        out = out[:start] + CHOICE_VAR + out[end:]
    return out


def fill_target_info(any_stmt, is_block):
    """Sets target_kind and choice_var on an AnyStmt, and rewrites its
    condition for a non-name target when the rewrite is safe."""
    if len(any_stmt.loop_vars) != 1:
        return
    target = any_stmt.loop_vars[0]
    kind = classify_target(target)
    any_stmt.target_kind = kind
    if kind == fizz_ast.ANY_TARGET_KIND_UNKNOWN:
        raise SyntaxError(f"oneof target must be a name, attribute or index, got {target}")
    if is_block and kind != fizz_ast.ANY_TARGET_KIND_NAME:
        raise SyntaxError(f"oneof loop variable must be a plain name, got {target}")
    if kind == fizz_ast.ANY_TARGET_KIND_NAME:
        any_stmt.choice_var = target
        return
    if not any_stmt.condition:
        return
    rewritten = rewrite_condition(target, any_stmt.condition)
    if rewritten is None:
        return
    any_stmt.choice_var = CHOICE_VAR
    any_stmt.condition_expr.py_expr = rewritten
