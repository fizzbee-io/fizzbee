
from antlr4 import *
import os
import sys

from parser.FizzParser import FizzParser
from parser.FizzParserVisitor import FizzParserVisitor
import proto.fizz_ast_pb2 as ast


class BuildAstVisitor(FizzParserVisitor):
    def __init__(self, input_stream, file_path=""):
        super().__init__()
        self.file_path = file_path
        self.file_name = os.path.basename(file_path)
        self.input_stream = input_stream
        # Formula returns seen while visiting the current assertion's body.
        self.formula_returns = []
        self.in_assertion = False

    def aggregateResult(self, aggregate, nextResult):
        if nextResult and aggregate:
            print("aggregate:", aggregate, "nextResult:", nextResult)
            raise Exception("only one of aggregate or next result handled now", aggregate, nextResult)
        print("aggregate:", aggregate, "nextResult:", nextResult)
        if nextResult is None:
            return aggregate
        return nextResult

    # Visit a parse tree produced by FizzParser#root.
    def visitRoot(self, ctx:FizzParser.RootContext):
        return self.visitFile_input(ctx.getChild(0))

    # Visit a parse tree produced by FizzParser#file_input.
    def visitFile_input(self, ctx:FizzParser.File_inputContext):
        print("\n\nvisitFile_input",ctx.__class__.__name__)
        print("visitFile_input",ctx.getText())
        print("visitFile_input",dir(ctx))
        print("visitFile_input children count",ctx.getChildCount())

        file = ast.File(source_info=get_source_info(ctx))
        file.source_info.file_name = self.file_name
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitFile_input child index",i,child.getText(), child.__class__.__name__)
            if hasattr(child, 'toStringTree'):
                childProto = self.visit(child)
                if isinstance(childProto, ast.StateVars):
                    file.states.CopyFrom(childProto)
                elif isinstance(childProto, ast.Action):
                    if childProto.name == "Init":
                        file.actions.insert(0, childProto)
                    else:
                        file.actions.append(childProto)
                elif isinstance(childProto, ast.Function):
                    file.functions.append(childProto)
                elif isinstance(childProto, ast.Invariant):
                    file.invariants.append(childProto)
                elif BuildAstVisitor.is_list_of_type(childProto, ast.Invariant):
                    file.invariants.extend(childProto)
                elif isinstance(childProto, ast.Statement):
                    file.stmts.append(childProto)
                elif isinstance(childProto, ast.Role):
                    file.roles.append(childProto)
                elif isinstance(childProto, ast.Composition):
                    file.composition.CopyFrom(childProto)
                elif isinstance(childProto, ast.Refinement):
                    file.refinements.append(childProto)
                else:
                    print("visitFile_input childProto (unknown) type",childProto.__class__.__name__, dir(child), dir(child.start), childProto)
                    errorStr = f"Error: Line: {child.start.line}: Unexpected {self.get_py_str(child)}"
                    print(errorStr, file=sys.stderr)
                    raise Exception(errorStr)
            elif hasattr(child, 'getSymbol'):
                if child.getSymbol().type == FizzParser.LINE_BREAK:
                    continue
                self.log_symbol(child)
            else:
                print("visitFile_input child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitFile_input child (unknown) type")
#         x = self.visitChildren(ctx)
#         print('visitFile_inputs children', x)
        print("file", file)
        return file

    # Visit a parse tree produced by FizzParser#decorator.
    def visitDecorator(self, ctx:FizzParser.DecoratorContext):
        print("\n\nvisitDecorator",ctx.__class__.__name__)
        print("visitDecorator",ctx.getText())
        print("visitDecorator",dir(ctx))
        print("visitDecorator children count",ctx.getChildCount())
        print("visitDecorator full text\n", self.get_py_str(ctx))

        decorator = ast.Decorator(source_info=get_source_info(ctx))
#         args = []
#         decorator_name = ""
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitDecorator child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                childProto = self.visit(child)
                print("visitDecorator childProto",childProto)
                if isinstance(child, FizzParser.ArglistContext):
                    decorator.args.extend(self.visitArglist(child))
                    continue
                elif isinstance(childProto, str):
                    decorator.name = childProto
                else:
                    raise Exception("visitDecorator childProto (unknown) type", child, childProto.__class__.__name__, dir(child), childProto)
            elif hasattr(child, 'getSymbol'):
                if child.getSymbol().type == FizzParser.LINE_BREAK:
                    continue
                self.log_symbol(child)
            else:
                print("visitDecorator child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitDecorator child (unknown) type")

        print("decorator", decorator)
        return decorator

    # Visit a parse tree produced by FizzParser#role_def_stmt.
    def visitRole_def_stmt(self, ctx:FizzParser.Role_def_stmtContext):
        print("\n\nvisitRole_def_stmt",ctx.__class__.__name__)
        print("visitRole_def_stmt",ctx.getText())
        print("visitRole_def_stmt",dir(ctx))
        print("visitRole_def_stmt children count",ctx.getChildCount())
        print("visitRole_def_stmt full text\n", self.get_py_str(ctx))
        decorators = []
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitRole_def_stmt child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.NameContext):
                    role_name = child.getText()
                    print("visitRole_def_stmt role_name", role_name)
                    continue
                childProto = self.visit(child)
                print("visitRole_def_stmt childProto",childProto)
                if isinstance(childProto, ast.Role):
                    print("visitRole_def_stmt childProto is role",childProto)
                    role = childProto
                elif isinstance(childProto, ast.Decorator):
                    print("visitRole_def_stmt childProto is decorator",childProto)
                    decorators.append(childProto)
                else:
                    raise Exception("visitRole_def_stmt childProto (unknown) type", child, childProto.__class__.__name__, dir(child), childProto)
            elif hasattr(child, 'getSymbol'):
                if child.getSymbol().type == FizzParser.LINE_BREAK:
                    continue
                self.log_symbol(child)
            else:
                print("visitRole_def_stmt child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitRole_def_stmt child (unknown) type")
        role.decorators.extend(decorators)
        return role

    # Visit a parse tree produced by FizzParser#roledef.
    def visitRoledef(self, ctx:FizzParser.RoledefContext):
        # Almost the same as visitRole_def_stmt
        print("\n\nvisitRoledef",ctx.__class__.__name__)
        print("visitRoledef",ctx.getText())
        print("visitRoledef",dir(ctx))
        print("visitRoledef children count",ctx.getChildCount())

        role = ast.Role(source_info=get_source_info(ctx))

        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitRoledef child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.NameContext):
                    role.name = child.getText()
                    continue
                childProto = self.visit(child)
                if isinstance(childProto, ast.StateVars):
                    role.states.CopyFrom(childProto)
                elif isinstance(childProto, ast.Action):
                    if childProto.name == "Init":
                        role.actions.insert(0, childProto)
                    else:
                        role.actions.append(childProto)
                elif isinstance(childProto, ast.Function):
                    role.functions.append(childProto)
                elif isinstance(childProto, ast.Invariant):
                    role.invariants.append(childProto)
                elif BuildAstVisitor.is_list_of_type(childProto, ast.Invariant):
                    role.invariants.extend(childProto)
                elif isinstance(childProto, ast.Statement):
                    role.stmts.append(childProto)
                elif BuildAstVisitor.is_list_of_type(childProto, ast.Action):
                    role.actions.extend(childProto)
                else:
                    print("visitRoledef childProto (unknown) type",childProto.__class__.__name__, dir(child), dir(child.start), childProto)
                    errorStr = f"Error: Line: {child.start.line}: Unexpected {self.get_py_str(child)}"
                    print(errorStr, file=sys.stderr)
                    raise Exception(errorStr)
            elif hasattr(child, 'getSymbol'):
                if child.getSymbol().type == FizzParser.LINE_BREAK:
                    continue
                if child.getSymbol().type == FizzParser.SYMMETRIC:
                    role.modifiers.append(child.getText())
                    continue
                self.log_symbol(child)
            else:
                print("visitRoledef child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitRoledef child (unknown) type")

        print("role", role)
        return role


    def is_list_of_type(lst, item_type):
        if not isinstance(lst, list):
            return False
        # Check if all elements in the list are instances of ast.Invariant
        return all(isinstance(item, item_type) for item in lst)

    def visitInit_stmt(self, ctx:FizzParser.Init_stmtContext):
        init_str = self.get_py_str(ctx)
        py_str = BuildAstVisitor.transform_code(init_str, 1)
        return ast.StateVars(code=py_str, source_info=get_source_info(ctx))

    def transform_code(input_code, lines_to_skip=0):
        # Split the input code into lines
        lines = input_code.split('\n')

        # Remove the specified number of lines from the beginning
        del lines[:lines_to_skip]

        # Find the indentation of the second line
        indentation = len(lines[0]) - len(lines[0].lstrip())

        # Remove the same indentation from all subsequent lines
        transformed_code = '\n'.join(line[indentation:] if line.strip() else line for line in lines)

        return transformed_code

    # Visit a parse tree produced by FizzParser#visitActiondef.
    def visitActiondef(self, ctx:FizzParser.ActiondefContext):
        print("\n\nvisitActiondef",ctx.__class__.__name__)
        print("visitActiondef",ctx.getText())
        print("visitActiondef",dir(ctx))
        print("visitActiondef children count",ctx.getChildCount())
        print("visitActiondef full text\n", self.get_py_str(ctx))

        action = ast.Action(source_info=get_source_info(ctx))
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitActiondef child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.NameContext):
                    action.name = child.getText()
                    continue

                self.log_childtree(child)
                childProto = self.visit(child)
                if isinstance(childProto, ast.Block):
                    action.block.CopyFrom(childProto)
                if isinstance(childProto, ast.Fairness):
                    action.fairness.CopyFrom(childProto)

                print("visitActiondef childProto",childProto)
            elif hasattr(child, 'getSymbol'):

                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.ACTION
                        or child.getSymbol().type == FizzParser.COLON
                ):
                    continue
                if child.getSymbol().type == FizzParser.ATOMIC:
                    action.flow = ast.Flow.FLOW_ATOMIC
                    continue
                if child.getSymbol().type == FizzParser.SERIAL:
                    action.flow = ast.Flow.FLOW_SERIAL
                    continue
                if child.getSymbol().type == FizzParser.ONEOF:
                    action.flow = ast.Flow.FLOW_ONEOF
                    continue
                if child.getSymbol().type == FizzParser.PARALLEL:
                    action.flow = ast.Flow.FLOW_PARALLEL
                    continue

                self.log_symbol(child)
            else:
                print("visitActiondef child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitActiondef child (unknown) type")

        if action.name == "Init":
            action.fairness.level = ast.FairnessLevel.FAIRNESS_LEVEL_STRONG
            action.flow = ast.Flow.FLOW_ATOMIC
        if action.flow == ast.Flow.FLOW_UNKNOWN and action.block.flow != ast.Flow.FLOW_UNKNOWN:
            action.flow = action.block.flow
        elif action.flow != ast.Flow.FLOW_UNKNOWN and action.block.flow == ast.Flow.FLOW_UNKNOWN:
            action.block.flow = action.flow
        elif action.flow == ast.Flow.FLOW_UNKNOWN and action.block.flow == ast.Flow.FLOW_UNKNOWN:
            action.block.flow =  ast.Flow.FLOW_SERIAL
            action.flow = ast.Flow.FLOW_SERIAL

        print("action.fairness", action.fairness)
        print("action.fairness.level", action.fairness.level)

        if action.fairness.level == ast.FairnessLevel.FAIRNESS_LEVEL_UNKNOWN:
            print("visitActiondef action.fairness not set")
            action.fairness.level = ast.FairnessLevel.FAIRNESS_LEVEL_UNFAIR

        print("action", action)
        return action

    # Visit a parse tree produced by FizzParser#fairness.
    def visitFairness(self, ctx:FizzParser.FairnessContext):
        fairness = ast.Fairness(source_info=get_source_info(ctx))
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitFairness child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.NameContext):
                    levelStr = child.getText()
                    print("visitFairness child name", levelStr)
                    if levelStr == "strong":
                        fairness.level = ast.FairnessLevel.FAIRNESS_LEVEL_STRONG
                    elif levelStr == "weak":
                        fairness.level = ast.FairnessLevel.FAIRNESS_LEVEL_WEAK
                    else:
                        raise Exception("Fairness can only be weak or strong.", levelStr)
                    break
                self.log_childtree(child)
                childProto = self.visit(child)
                print("visitFairness childProto",childProto)
            elif hasattr(child, 'getSymbol'):
                if (child.getSymbol().type == FizzParser.LESS_THAN
                        or child.getSymbol().type == FizzParser.FAIR
                        or child.getSymbol().type == FizzParser.GREATER_THAN
                ):
                    continue
                self.log_symbol(child)
            else:
                print("visitFairness child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitFairness child (unknown) type")

        if fairness.level == ast.FairnessLevel.FAIRNESS_LEVEL_UNKNOWN:
            fairness.level = ast.FairnessLevel.FAIRNESS_LEVEL_WEAK
        return fairness


    # Visit a parse tree produced by FizzParser#assertiondef.
    def visitAssertiondef(self, ctx:FizzParser.AssertiondefContext):
        print("\n\nvisitAssertiondef",ctx.__class__.__name__)
        print("visitAssertiondef",ctx.getText())
        print("visitAssertiondef",dir(ctx))
        print("visitAssertiondef children count",ctx.getChildCount())
        print("visitAssertiondef full text\n", self.get_py_str(ctx))

        invariant = ast.Invariant(source_info=get_source_info(ctx))
        py_code = "def "
        outer = (self.in_assertion, self.formula_returns)
        self.in_assertion, self.formula_returns = True, []
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitAssertiondef child index",i,child.getText())
            if isinstance(child, FizzParser.Assertion_for_clauseContext):
                invariant.for_clauses.append(self.build_for_clause(child))
                continue
            if hasattr(child, 'start'):
                print("visitAssertiondef child start,stop",child.start,child.stop)

            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.NameContext):
                    invariant.name = child.getText()
                    py_code += child.getText() + "():\n"
                    continue

                self.log_childtree(child)
                childProto = self.visit(child)
                if isinstance(childProto, ast.Block):
                    invariant.block.CopyFrom(childProto)
                elif BuildAstVisitor.is_list_of_type(childProto, ast.Parameter):
                    invariant.params.extend(childProto)
                print("visitAssertiondef childProto",childProto)
            elif hasattr(child, 'getSymbol'):

                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.ASSERTION
                        or child.getSymbol().type == FizzParser.COLON
                ):
                    continue
                if child.getSymbol().type in [FizzParser.EVENTUALLY, FizzParser.ALWAYS, FizzParser.EXISTS, FizzParser.TRANSITION]:
                    invariant.temporal_operators.append(child.getText())
                    continue

                self.log_symbol(child)
            else:
                print("visitAssertiondef child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitAssertiondef child (unknown) type")

        formula_returns = self.formula_returns
        self.in_assertion, self.formula_returns = outer
        self.finish_formula_assertion(ctx, invariant, formula_returns)

        block_str = self.get_py_str(ctx)
        py_code += '\n'.join(block_str.split('\n')[1:])
        invariant.block.flow = ast.Flow.FLOW_ATOMIC
        invariant.py_code = py_code
        print("assertion", invariant)
        return invariant

    # Visit a parse tree produced by FizzParser#refinedef.
    def visitRefinedef(self, ctx:FizzParser.RefinedefContext):
        print("\n\nvisitRefinedef",ctx.__class__.__name__)
        print("visitRefinedef",ctx.getText())
        print("visitRefinedef",dir(ctx))
        print("visitRefinedef children count",ctx.getChildCount())
        print("visitRefinedef full text\n", self.get_py_str(ctx))

        refinement = ast.Refinement(source_info=get_source_info(ctx))
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitRefinedef child index",i,child.getText())
            if hasattr(child, 'toStringTree'):

                self.log_childtree(child)
                childProto = self.visit(child)
                if isinstance(childProto, ast.SpecEntry):
                    refinement.specs.append(childProto)
                    continue
                print("visitRefinedef childProto",childProto)
            elif hasattr(child, 'getSymbol'):
                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.ACTION
                        or child.getSymbol().type == FizzParser.COLON
                ):
                    continue

                self.log_symbol(child)
            else:
                print("visitRefinedef child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitRefinedef child (unknown) type")
        print("refinement", refinement)
        return refinement

    # Visit a parse tree produced by FizzParser#composedef.
    def visitComposedef(self, ctx:FizzParser.ComposedefContext):
        print("\n\nvisitComposedef",ctx.__class__.__name__)
        print("visitComposedef",ctx.getText())
        print("visitComposedef",dir(ctx))
        print("visitComposedef children count",ctx.getChildCount())
        print("visitComposedef full text\n", self.get_py_str(ctx))

        composition = ast.Composition(source_info=get_source_info(ctx))
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitComposedef child index",i,child.getText())
            if hasattr(child, 'toStringTree'):

                self.log_childtree(child)
                childProto = self.visit(child)
                if isinstance(childProto, ast.SpecEntry):
                    composition.specs.append(childProto)
                    continue
                print("visitComposedef childProto",childProto)
            elif hasattr(child, 'getSymbol'):
                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.ACTION
                        or child.getSymbol().type == FizzParser.COLON
                ):
                    continue

                self.log_symbol(child)
            else:
                print("visitComposedef child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitComposedef child (unknown) type")
        print("composition", composition)
        return composition

    # Visit a parse tree produced by FizzParser#compose_entry.
    def visitCompose_entry(self, ctx:FizzParser.Compose_entryContext):
        print("\n\nvisitCompose_entry",ctx.__class__.__name__)
        print("visitCompose_entry",ctx.getText())
        print("visitCompose_entry",dir(ctx))
        print("visitCompose_entry children count",ctx.getChildCount())
        print("visitCompose_entry full text\n", self.get_py_str(ctx))
        entry = ast.SpecEntry(source_info=get_source_info(ctx))
        # entry is defined "name COLON test" in antlr4 parser spec
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitCompose_entry child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.NameContext):
                    entry.name = child.getText()
                    continue
                if isinstance(child, FizzParser.TestContext):
                    entry.expr.py_expr = self.get_py_str(child)
                    entry.expr.source_info.CopyFrom(get_source_info(child))
                    continue

        print("spec_entry", entry)
        return entry
    # Visit a parse tree produced by FizzParser#functiondef.
    def visitFunctiondef(self, ctx:FizzParser.FunctiondefContext):
        print("\n\nvisitFunctiondef",ctx.__class__.__name__)
        print("visitFunctiondef",ctx.getText())
        print("visitFunctiondef",dir(ctx))
        print("visitFunctiondef children count",ctx.getChildCount())
        print("visitFunctiondef full text\n", self.get_py_str(ctx))

        function = ast.Function(source_info=get_source_info(ctx))

        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitFunctiondef child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.NameContext):
                    function.name = child.getText()
                    continue

                self.log_childtree(child)
                childProto = self.visit(child)
                if isinstance(childProto, ast.Block):
                    function.block.CopyFrom(childProto)
                elif BuildAstVisitor.is_list_of_type(childProto, ast.Parameter):
                    function.params.extend(childProto)
                print("visitFunctiondef childProto",childProto)
            elif hasattr(child, 'getSymbol'):

                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.ACTION
                        or child.getSymbol().type == FizzParser.COLON
                ):
                    continue
                if child.getSymbol().type == FizzParser.ATOMIC:
                    function.flow = ast.Flow.FLOW_ATOMIC
                    continue
                if child.getSymbol().type == FizzParser.SERIAL:
                    function.flow = ast.Flow.FLOW_SERIAL
                    continue
                if child.getSymbol().type == FizzParser.ONEOF:
                    function.flow = ast.Flow.FLOW_ONEOF
                    continue
                if child.getSymbol().type == FizzParser.PARALLEL:
                    function.flow = ast.Flow.FLOW_PARALLEL
                    continue

                self.log_symbol(child)
            else:
                print("visitFunctiondef child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitFunctiondef child (unknown) type")

        if function.flow == ast.Flow.FLOW_UNKNOWN and function.block.flow != ast.Flow.FLOW_UNKNOWN:
            function.flow = function.block.flow
        elif function.flow != ast.Flow.FLOW_UNKNOWN and function.block.flow == ast.Flow.FLOW_UNKNOWN:
            function.block.flow = function.flow
        elif function.flow == ast.Flow.FLOW_UNKNOWN and function.block.flow == ast.Flow.FLOW_UNKNOWN:
            function.block.flow =  ast.Flow.FLOW_SERIAL
            function.flow = ast.Flow.FLOW_SERIAL

        print("function", function)
        return function

    # Visit a parse tree produced by FizzParser#func_call_stmt.
    def visitFunc_call_stmt(self, ctx:FizzParser.Func_call_stmtContext):
        print("\n\nvisitFunc_call_stmt",ctx.__class__.__name__)
        print("visitFunc_call_stmt",ctx.getText())
        print("visitFunc_call_stmt",dir(ctx))
        print("visitFunc_call_stmt children count",ctx.getChildCount())
        print("visitFunc_call_stmt full text\n", self.get_py_str(ctx))

        func_call = ast.CallStmt(source_info=get_source_info(ctx))
        has_assign = False
        has_receiver = False
        # Iterating in the reverse direction to make it easy
        for i, child in reversed(list(enumerate(ctx.getChildren()))):
            print()
            print("visitFunc_call_stmt child index",i,child.getText())
            if hasattr(child, 'toStringTree'):

                if isinstance(child, FizzParser.ArglistContext):
                    func_call.args.extend(self.visitArglist(child))
                    continue

                self.log_childtree(child)
                childProto = self.visit(child)
                print("visitFunc_call_stmt childProto",childProto)
            elif hasattr(child, 'getSymbol'):
                if child.getSymbol().type == FizzParser.NAME:
                    if has_assign:
                        func_call.vars.insert(0, child.getText())
                    elif has_receiver:
                        func_call.receiver = child.getText()
                    else:
                        func_call.name = child.getText()
                    continue
                if child.getSymbol().type == FizzParser.DOT:
                    has_receiver = True
                if child.getSymbol().type == FizzParser.ASSIGN:
                    has_assign = True
                    continue
                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.CLOSE_PAREN
                        or child.getSymbol().type == FizzParser.OPEN_PAREN
                ):
                    continue

                self.log_symbol(child)
            else:
                print("visitFunc_call_stmt child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitFunc_call_stmt child (unknown) type")

        print("func_call", func_call)
        return func_call

    # Visit a parse tree produced by FizzParser#funcdef.
    def visitFuncdef(self, ctx:FizzParser.FuncdefContext):
        print("\n\nvisitFuncdef",ctx.__class__.__name__)
        print("visitFuncdef",ctx.getText())
        print("visitFuncdef",dir(ctx))
        print("visitFuncdef children count",ctx.getChildCount())
        print("visitFuncdef full text\n", self.get_py_str(ctx))

        function = ast.Function(source_info=get_source_info(ctx))

        py_str = self.get_py_str(ctx)
        print("visitExpr_stmt full text\n",py_str)
        py_str = BuildAstVisitor.transform_code(py_str)
        return ast.PyStmt(code=py_str, source_info=get_source_info(ctx))


    # Visit a parse tree produced by FizzParser#arglist.
    def visitArglist(self, ctx:FizzParser.ArglistContext):
        arguments = []
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitArglist child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.ArgumentContext):
                    arguments.append(self.visit(child))
                    continue

                self.log_childtree(child)
                childProto = self.visit(child)
                print("visitArglist childProto",childProto)

        return arguments

    # Visit a parse tree produced by FizzParser#argument.
    def visitArgument(self, ctx:FizzParser.ArgumentContext):
        # TODO: Handle named arguments
        argument = ast.Argument(source_info=get_source_info(ctx))
        py_str = self.get_py_str(ctx)
        argument.py_expr = BuildAstVisitor.transform_code(py_str)
        argument.expr.py_expr = argument.py_expr
        argument.expr.source_info.CopyFrom(get_source_info(ctx))
        return argument

    # Visit a parse tree produced by FizzParser#def_parameter.
    def visitDef_parameter(self, ctx:FizzParser.Def_parameterContext):
        print("\n\nvisitDef_parameter",ctx.__class__.__name__)
        print("visitDef_parameter",ctx.getText())
        print("visitDef_parameter",dir(ctx))
        print("visitDef_parameter children count",ctx.getChildCount())
        print("visitDef_parameter full text\n", self.get_py_str(ctx))

        param = ast.Parameter(source_info=get_source_info(ctx))
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitDef_parameter child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.NameContext):
                    param.name = child.getText()
                    continue
                if isinstance(child, FizzParser.TestContext):
                    param.default_py_expr = self.get_py_str(child)
                    param.default_expr.py_expr = param.default_py_expr
                    param.default_expr.source_info.CopyFrom(get_source_info(child))
                    continue
                self.log_childtree(child)
                childProto = self.visit(child)
                if isinstance(childProto, ast.Parameter):
                    param.CopyFrom(childProto)
                print("visitDef_parameter childProto",childProto)
            elif hasattr(child, 'getSymbol'):
                if (child.getSymbol().type == FizzParser.NAME
                        or child.getSymbol().type == FizzParser.COMMA
                ):
                    continue
                self.log_symbol(child)
            else:
                print("visitDef_parameter child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitDef_parameter child (unknown) type")

        print("param", param)
        return param

    # Visit a parse tree produced by FizzParser#def_parameter.
    def visitDef_parameters(self, ctx:FizzParser.Def_parameterContext):
        params = []
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitDef_parameters child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                # if isinstance(child, FizzParser.NameContext):
                #     return child.getText()
                self.log_childtree(child)
                childProto = self.visit(child)
                if isinstance(childProto, ast.Parameter):
                    params.append(childProto)
                    continue
                print("visitDef_parameters childProto",childProto)
            elif hasattr(child, 'getSymbol'):
                if (child.getSymbol().type == FizzParser.NAME
                        or child.getSymbol().type == FizzParser.COMMA
                ):
                    continue
                self.log_symbol(child)
            else:
                print("visitDef_parameters child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitDef_parameters child (unknown) type")

        print("visitDef_parameters params", params)
        return params

    # Visit a parse tree produced by FizzParser#named_parameter.
    def visitNamed_parameter(self, ctx:FizzParser.Named_parameterContext):
        print("\n\nvisitNamed_parameter",ctx.__class__.__name__)
        print("visitNamed_parameter",ctx.getText())
        child = ctx.getChild(0)
        param = ast.Parameter(name=child.getText(), source_info=get_source_info(ctx))
        return param

    # Visit a parse tree produced by FizzParser#dotted_name.
    def visitDotted_name(self, ctx:FizzParser.Dotted_nameContext):
        return ".".join([child.getText() for child in ctx.getChildren()])

    # Visit a parse tree produced by FizzParser#expr_stmt.
    def visitExpr_stmt(self, ctx:FizzParser.Expr_stmtContext):
        py_str = self.get_py_str(ctx)
        print("visitExpr_stmt full text\n",py_str)
        py_str = BuildAstVisitor.transform_code(py_str)
        return ast.PyStmt(code=py_str, source_info=get_source_info(ctx))

    # Visit a parse tree produced by FizzParser#flow_stmt.
    def visitFlow_stmt(self, ctx:FizzParser.Flow_stmtContext):
        print("\n\nvisitFlow_stmt",ctx.__class__.__name__)
        print("visitFlow_stmt\n",ctx.getText())
        block = None
        flow = ast.Flow.FLOW_UNKNOWN
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitFlow_stmt child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                self.log_childtree(child)
                childProto = self.visit(child)

                if isinstance(childProto, ast.Block):
                    block = childProto
                print("visitFlow_stmt childProto",childProto)
            elif hasattr(child, 'getSymbol'):
                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.ACTION
                        or child.getSymbol().type == FizzParser.COLON
                        or child.getSymbol().type == FizzParser.INDENT
                ):
                    continue
                if child.getSymbol().type == FizzParser.ATOMIC:
                    flow = ast.Flow.FLOW_ATOMIC
                    continue
                if child.getSymbol().type == FizzParser.SERIAL:
                    flow = ast.Flow.FLOW_SERIAL
                    continue
                if child.getSymbol().type == FizzParser.ONEOF:
                    flow = ast.Flow.FLOW_ONEOF
                    continue
                if child.getSymbol().type == FizzParser.PARALLEL:
                    flow = ast.Flow.FLOW_PARALLEL
                    continue
                self.log_symbol(child)
            else:
                print("visitFlow_stmt child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitFlow_stmt child (unknown) type")

        if block is None:
            block = ast.Block(source_info=get_source_info(ctx))
        block.flow = flow
        print("visitFlow_stmt block", block)
        return block

    # Visit a parse tree produced by FizzParser#labeled_stmt.
    def visitLabeled_stmt(self, ctx:FizzParser.Labeled_stmtContext):
        print("\n\nvisitLabeled_stmt",ctx.__class__.__name__)
        print("visitLabeled_stmt\n",ctx.getText())
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitLabeled_stmt child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                self.log_childtree(child)
                childProto = self.visit(child)
                print("visitLabeled_stmt childProto",childProto)
                if isinstance(childProto, ast.Statement):
                    # Get the label from the 0th child's text, and remove the first and last characters
                    label = ctx.getChild(0).getText()
                    childProto.label = label[1:-1]
                    return childProto
                print("visitLabeled_stmt childProto",childProto)
                raise Exception("visitLabeled_stmt childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto)
            elif hasattr(child, 'getSymbol'):
                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.ACTION
                        or child.getSymbol().type == FizzParser.COLON
                        or child.getSymbol().type == FizzParser.INDENT
                ):
                    self.log_symbol(child)
                    continue
                self.log_symbol(child)

        raise Exception("visitLabeled_stmt childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto)

    # Visit a parse tree produced by FizzParser#suite.
    def visitSuite(self, ctx:FizzParser.SuiteContext):
        print("\n\nvisitSuite",ctx.__class__.__name__)
        print("visitSuite\n",ctx.getText())
        block = ast.Block(source_info=get_source_info(ctx))
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitSuite child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                self.log_childtree(child)
                childProto = self.visit(child)
                if isinstance(childProto, ast.Statement):
                    if childProto.py_stmt is not None and (
                            (childProto.py_stmt.code.startswith("'''") and childProto.py_stmt.code.endswith("'''"))
                        or (childProto.py_stmt.code.startswith('"""') and childProto.py_stmt.code.endswith('"""'))
                    ):
                        continue
                    block.stmts.append(childProto)
                    continue

                print("visitSuite childProto",childProto)
                raise Exception("visitSuite childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto, child)
            elif hasattr(child, 'getSymbol'):

                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.ACTION
                        or child.getSymbol().type == FizzParser.COLON
                        or child.getSymbol().type == FizzParser.INDENT
                ):
                    continue

                self.log_symbol(child)
            else:
                print("visitSuite child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitSuite child (unknown) type")
        print("visitSuite block", block)
        if len(block.stmts) == 1 and block.stmts[0].block is not None and block.stmts[0].block.ByteSize() > 0:
            print("visitSuite block.stmts[0].block", block.stmts[0].block, block.stmts[0].block.__class__.__name__)
            return block.stmts[0].block
        return block

    # Visit a parse tree produced by FizzParser#stmt.
    def visitStmt(self, ctx:FizzParser.StmtContext):
        print("\n\nvisitStmt",ctx.__class__.__name__)
        childStmt = ctx.getChild(0)
        count = 0
        if ctx.getChildCount() != 1:
            for i, child in enumerate(ctx.getChildren()):
                print()
                print("visitStmt child index",i,child.getText())
                if hasattr(child, 'toStringTree'):
                    self.log_childtree(child)
                    childStmt = child
                    count += 1
                elif hasattr(child, 'getSymbol'):
                    if child.getSymbol().type == FizzParser.LINE_BREAK:
                        continue
                    self.log_symbol(child)
                else:
                    print("visitStmt child (unknown) type",child.__class__.__name__, child.getText())
            if count != 1:
                raise Exception("visitStmt child count != 1", count, ctx.getText())
        else:
            print("visitStmt child",childStmt.getText())
        childProto = self.visit(childStmt)
        if childProto is None:
            print("childProto is None for visitStmt", childStmt)
            return None
        if isinstance(childProto, ast.PyStmt):
            return ast.Statement(source_info=get_source_info(ctx), py_stmt=childProto)
        elif isinstance(childProto, ast.Block):
            return ast.Statement(source_info=get_source_info(ctx), block=childProto)
        elif isinstance(childProto, ast.IfStmt):
            return ast.Statement(source_info=get_source_info(ctx), if_stmt=childProto)
        elif isinstance(childProto, ast.AnyStmt):
            return ast.Statement(source_info=get_source_info(ctx), any_stmt=childProto)
        elif isinstance(childProto, ast.ForStmt):
            return ast.Statement(source_info=get_source_info(ctx), for_stmt=childProto)
        elif isinstance(childProto, ast.WhileStmt):
            return ast.Statement(source_info=get_source_info(ctx), while_stmt=childProto)
        elif isinstance(childProto, ast.BreakStmt):
            return ast.Statement(source_info=get_source_info(ctx), break_stmt=childProto)
        elif isinstance(childProto, ast.ContinueStmt):
            return ast.Statement(source_info=get_source_info(ctx), continue_stmt=childProto)
        elif isinstance(childProto, ast.ReturnStmt):
            return ast.Statement(source_info=get_source_info(ctx), return_stmt=childProto)
        elif isinstance(childProto, ast.CallStmt):
            return ast.Statement(source_info=get_source_info(ctx), call_stmt=childProto)
        elif isinstance(childProto, ast.RequireStmt):
            return ast.Statement(source_info=get_source_info(ctx), require_stmt=childProto)

        elif isinstance(childProto, ast.StateVars):
            return childProto
        elif isinstance(childProto, ast.Action):
            return childProto
        elif isinstance(childProto, ast.Function):
            return childProto
        elif isinstance(childProto, ast.Invariant):
            return childProto
        elif isinstance(childProto, ast.Statement):
            return childProto
        elif isinstance(childProto, ast.Role):
            return childProto
        elif isinstance(childProto, ast.Composition):
            return childProto
        elif isinstance(childProto, ast.Refinement):
            return childProto
        elif BuildAstVisitor.is_list_of_type(childProto, ast.Invariant):
            return childProto

        raise Exception("visitStmt childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto)

    # Visit a parse tree produced by FizzParser#simple_stmt.
    # Handles inline suite bodies like `if cond: return` where suite = simple_stmt.
    def visitSimple_stmt(self, ctx:FizzParser.Simple_stmtContext):
        childProto = self.visitChildren(ctx)
        if childProto is None:
            return None
        if isinstance(childProto, ast.Statement):
            return childProto
        if isinstance(childProto, ast.PyStmt):
            return ast.Statement(source_info=get_source_info(ctx), py_stmt=childProto)
        elif isinstance(childProto, ast.ReturnStmt):
            return ast.Statement(source_info=get_source_info(ctx), return_stmt=childProto)
        elif isinstance(childProto, ast.BreakStmt):
            return ast.Statement(source_info=get_source_info(ctx), break_stmt=childProto)
        elif isinstance(childProto, ast.ContinueStmt):
            return ast.Statement(source_info=get_source_info(ctx), continue_stmt=childProto)
        elif isinstance(childProto, ast.RequireStmt):
            return ast.Statement(source_info=get_source_info(ctx), require_stmt=childProto)
        elif isinstance(childProto, ast.CallStmt):
            return ast.Statement(source_info=get_source_info(ctx), call_stmt=childProto)
        elif isinstance(childProto, ast.AnyStmt):
            return ast.Statement(source_info=get_source_info(ctx), any_stmt=childProto)
        raise Exception("visitSimple_stmt childProto (unknown) type", childProto.__class__.__name__, childProto)

    # Visit a parse tree produced by FizzParser#if_stmt.
    def visitIf_stmt(self, ctx:FizzParser.If_stmtContext):
        print("\n\nvisitIf_stmt",ctx.__class__.__name__)
        print("visitIf_stmt\n",ctx.getText())
        if_stmt = ast.IfStmt(source_info=get_source_info(ctx))
        branch = ast.Branch()
        if_stmt.branches.append(branch)

        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitIf_stmt child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                self.log_childtree(child)
                if isinstance(child, FizzParser.TestContext):
                    py_expr = self.get_py_str(child)
                    if_stmt.branches[0].condition = py_expr
                    if_stmt.branches[0].condition_expr.py_expr = py_expr
                    if_stmt.branches[0].condition_expr.source_info.CopyFrom(get_source_info(child))
                    if_stmt.branches[0].source_info.CopyFrom(get_source_info(child))
                    continue
                childProto = self.visit(child)
                print("visitIf_stmt childProto",childProto, childProto.__class__.__name__, child.__class__.__name__)
                if isinstance(childProto, ast.Block):
                    if_stmt.branches[0].block.CopyFrom(childProto)
                    if_stmt.branches[0].source_info.end.CopyFrom(childProto.source_info.end)
                elif isinstance(childProto, ast.Branch):
                    if_stmt.branches.append(childProto)
                else:
                    print("visitIf_stmt childProto",childProto)
                    raise Exception("visitIf_stmt childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto)
            elif hasattr(child, 'getSymbol'):
                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.COLON
                        or child.getSymbol().type == FizzParser.INDENT
                        or child.getSymbol().type == FizzParser.IF
                ):
                    continue
                self.log_symbol(child)
                raise Exception("visitIf_stmt child (unknown) type",child.__class__.__name__, dir(child))
            else:
                print("visitIf_stmt child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitIf_stmt child (unknown) type")

        print("visitIf_stmt if_stmt", if_stmt)
        return if_stmt

    # Visit a parse tree produced by FizzParser#elif_clause.
    def visitElif_clause(self, ctx:FizzParser.Elif_clauseContext):
        if ctx.getChildCount() != 4:
            raise Exception("visitElif_clause child count != 4", ctx.getChildCount(), ctx.getText())
        branch = ast.Branch(source_info=get_source_info(ctx))
        branch.condition_expr.py_expr = self.get_py_str(ctx.getChild(1))
        branch.condition_expr.source_info.CopyFrom(get_source_info(ctx.getChild(1)))
        branch.condition = branch.condition_expr.py_expr
        branch.block.CopyFrom(self.visit(ctx.getChild(3)))
        print("visitElif_clause branch", branch)
        return branch


    # Visit a parse tree produced by FizzParser#else_clause.
    def visitElse_clause(self, ctx:FizzParser.Else_clauseContext):
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitElse_clause child index",i,child.getText())
            branch = ast.Branch(source_info=get_source_info(ctx))
            branch.condition = "True"
            branch.condition_expr.py_expr="True"
            if hasattr(child, 'toStringTree'):
                self.log_childtree(child)
                if isinstance(child, FizzParser.TestContext):
                    branch.condition = self.get_py_str(child)
                    continue
                # if isinstance(child, FizzParser.Elif_clauseContext):
                #     branch.condition = self.get_py_str(child)
                #     continue
                childProto = self.visit(child)
                if isinstance(childProto, ast.Block):
                    branch.block.CopyFrom(childProto)
                    continue

                print("visitElse_clause childProto",childProto)
                raise Exception("visitElse_clause childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto)
            elif hasattr(child, 'getSymbol'):
                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.COLON
                        or child.getSymbol().type == FizzParser.INDENT
                ):
                    continue
                self.log_symbol(child)
            else:
                print("visitElse_clause child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitElse_clause child (unknown) type")

        print("visitElse_clause branch", branch)
        return branch

    # Visit a parse tree produced by FizzParser#any_stmt.
    def visitAny_stmt(self, ctx:FizzParser.Any_stmtContext):
        print("\n\nvisitAny_stmt",ctx.__class__.__name__)
        print("visitAny_stmt\n",ctx.getText())
        any_stmt = ast.AnyStmt(source_info=get_source_info(ctx))
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitAny_stmt child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.ExprlistContext):
                    any_stmt.loop_vars.extend(self.visitExprlist(child))
                    continue
                if isinstance(child, FizzParser.TestlistContext):
                    any_stmt.py_expr = self.get_py_str(child)
                    any_stmt.iter_expr.py_expr = any_stmt.py_expr
                    any_stmt.iter_expr.source_info.CopyFrom(get_source_info(child))
                    continue

                self.log_childtree(child)
                childProto = self.visit(child)
                print("visitAny_stmt childProto",childProto)
                if isinstance(childProto, ast.Block):
                    any_stmt.block.CopyFrom(childProto)
                else:
                    print("visitAny_stmt childProto",childProto)
                    raise Exception("visitAny_stmt childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto)
            elif hasattr(child, 'getSymbol'):
                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.COLON
                        or child.getSymbol().type == FizzParser.INDENT
                        or child.getSymbol().type == FizzParser.ONEOF
                        or child.getSymbol().type == FizzParser.IN
                ):
                    continue
                if child.getSymbol().type == FizzParser.ANY:
                    import sys
                    print("DeprecationWarning: 'any VAR in COLLECTION:' is deprecated, use 'oneof VAR in COLLECTION:' instead", file=sys.stderr)
                    continue
                self.log_symbol(child)
            else:
                print("visitAny_stmt child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitAny_stmt child (unknown) type")

        print("visitAny_stmt any_stmt", any_stmt)
        return any_stmt


    # Visit a parse tree produced by FizzParser#any_assign_stmt.
    def visitAny_assign_stmt(self, ctx:FizzParser.Any_assign_stmtContext):
        print("\n\nvisitAny_assign_stmt",ctx.__class__.__name__)
        print("visitAny_assign_stmt\n",ctx.getText())
        any_stmt = ast.AnyStmt(source_info=get_source_info(ctx))
        has_condition = False
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitAny_assign_stmt child index",i,child.getText())
            if has_condition:
                any_stmt.condition = self.get_py_str(child)
                any_stmt.condition_expr.py_expr = any_stmt.condition
                any_stmt.condition_expr.source_info.CopyFrom(get_source_info(child))
                continue
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.ExprlistContext):
                    any_stmt.loop_vars.extend(self.visitExprlist(child))
                    continue
                if isinstance(child, FizzParser.TestlistContext):
                    any_stmt.py_expr = self.get_py_str(child)
                    any_stmt.iter_expr.py_expr = any_stmt.py_expr
                    any_stmt.iter_expr.source_info.CopyFrom(get_source_info(child))
                    continue

                self.log_childtree(child)
                childProto = self.visit(child)
                print("visitAny_assign_stmt childProto",childProto)
                if isinstance(childProto, ast.Block):
                    any_stmt.block.CopyFrom(childProto)
                elif isinstance(childProto, ast.Fairness):
                    any_stmt.fairness.CopyFrom(childProto)
                else:
                    print("visitAny_assign_stmt childProto",childProto, child)
                    raise Exception("visitAny_assign_stmt childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto)
            elif hasattr(child, 'getSymbol'):
                if child.getSymbol().type == FizzParser.COLON or child.getSymbol().type == FizzParser.IF:
                    has_condition = True
                    print("visitAny_assign_stmt has_condition", has_condition)
                    continue
                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.INDENT
                        or child.getSymbol().type == FizzParser.ASSIGN
                        or child.getSymbol().type == FizzParser.ONEOF
                ):
                    continue
                if child.getSymbol().type == FizzParser.ANY:
                    import sys
                    print("DeprecationWarning: 'VAR = any COLLECTION' is deprecated, use 'VAR = oneof COLLECTION' instead", file=sys.stderr)
                    continue
                self.log_symbol(child)
            else:
                print("visitAny_assign_stmt child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitAny_assign_stmt child (unknown) type")

        print("visitAny_assign_stmt any_stmt", any_stmt)
        return any_stmt

    # Visit a parse tree produced by FizzParser#for_stmt.
    def visitFor_stmt(self, ctx:FizzParser.For_stmtContext):
        print("\n\nvisitFor_stmt",ctx.__class__.__name__)
        print("visitFor_stmt\n",ctx.getText())
        for_stmt = ast.ForStmt(source_info=get_source_info(ctx))
        flow = ast.Flow.FLOW_UNKNOWN
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitFor_stmt child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.ExprlistContext):
                    for_stmt.loop_vars.extend(self.visitExprlist(child))
                    continue
                if isinstance(child, FizzParser.TestlistContext):
                    for_stmt.py_expr = self.get_py_str(child)
                    for_stmt.iter_expr.py_expr = for_stmt.py_expr
                    for_stmt.iter_expr.source_info.CopyFrom(get_source_info(child))
                    continue

                self.log_childtree(child)
                childProto = self.visit(child)
                print("visitFor_stmt childProto",childProto)
                if isinstance(childProto, ast.Block):
                    for_stmt.block.CopyFrom(childProto)
                else:
                    print("visitFor_stmt childProto",childProto)
                    raise Exception("visitFor_stmt childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto)
            elif hasattr(child, 'getSymbol'):
                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.COLON
                        or child.getSymbol().type == FizzParser.INDENT
                ):
                    continue
                if child.getSymbol().type == FizzParser.ATOMIC:
                    flow = ast.Flow.FLOW_ATOMIC
                    continue
                if child.getSymbol().type == FizzParser.SERIAL:
                    flow = ast.Flow.FLOW_SERIAL
                    continue
                if child.getSymbol().type == FizzParser.ONEOF:
                    flow = ast.Flow.FLOW_ONEOF
                    continue
                if child.getSymbol().type == FizzParser.PARALLEL:
                    flow = ast.Flow.FLOW_PARALLEL
                    continue
                self.log_symbol(child)
            else:
                print("visitFor_stmt child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitFor_stmt child (unknown) type")

        print("visitFor_stmt for_stmt", for_stmt)
        for_stmt.flow = flow
        return for_stmt

    # Visit a parse tree produced by FizzParser#while_stmt.
    def visitWhile_stmt(self, ctx:FizzParser.While_stmtContext):
        print("\n\nvisitWhile_stmt",ctx.__class__.__name__)
        print("visitWhile_stmt\n",ctx.getText())
        while_stmt = ast.WhileStmt(source_info=get_source_info(ctx))
        flow = ast.Flow.FLOW_UNKNOWN
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitWhile_stmt child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.TestContext):
                    while_stmt.py_expr = self.get_py_str(child)
                    while_stmt.iter_expr.py_expr = while_stmt.py_expr
                    while_stmt.iter_expr.source_info.CopyFrom(get_source_info(child))
                    continue

                self.log_childtree(child)
                childProto = self.visit(child)
                print("visitWhile_stmt childProto",childProto)
                if isinstance(childProto, ast.Block):
                    while_stmt.block.CopyFrom(childProto)
                else:
                    print("visitWhile_stmt childProto",childProto)
                    raise Exception("visitWhile_stmt childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto)
            elif hasattr(child, 'getSymbol'):
                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.COLON
                        or child.getSymbol().type == FizzParser.INDENT
                ):
                    continue
                if child.getSymbol().type == FizzParser.ATOMIC:
                    flow = ast.Flow.FLOW_ATOMIC
                    continue
                if child.getSymbol().type == FizzParser.SERIAL:
                    flow = ast.Flow.FLOW_SERIAL
                    continue
                if child.getSymbol().type == FizzParser.ONEOF:
                    flow = ast.Flow.FLOW_ONEOF
                    continue
                if child.getSymbol().type == FizzParser.PARALLEL:
                    flow = ast.Flow.FLOW_PARALLEL
                    continue
                self.log_symbol(child)
            else:
                print("visitWhile_stmt child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitWhile_stmt child (unknown) type")

        print("visitWhile_stmt while_stmt", while_stmt)
        while_stmt.flow = flow
        return while_stmt

    # Visit a parse tree produced by FizzParser#pass_stmt.
    def visitPass_stmt(self, ctx:FizzParser.Pass_stmtContext):
        return ast.PyStmt(code="pass", source_info=get_source_info(ctx))

    # Visit a parse tree produced by FizzParser#break_stmt.
    def visitBreak_stmt(self, ctx:FizzParser.Break_stmtContext):
        return ast.BreakStmt(source_info=get_source_info(ctx))

    # Visit a parse tree produced by FizzParser#require_stmt.
    def visitRequire_stmt(self, ctx:FizzParser.Require_stmtContext):
        print("\n\nvisitRequire_stmt",ctx.__class__.__name__)
        print("visitRequire_stmt\n",ctx.getText())
        require_stmt = ast.RequireStmt(source_info=get_source_info(ctx))
        require_stmt.condition = self.get_py_str(ctx.getChild(1))
        require_stmt.condition_expr.py_expr = require_stmt.condition
        require_stmt.condition_expr.source_info.CopyFrom(get_source_info(ctx.getChild(1)))
        return require_stmt

    # Visit a parse tree produced by FizzParser#continue_stmt.
    def visitContinue_stmt(self, ctx:FizzParser.Continue_stmtContext):
        return ast.ContinueStmt(source_info=get_source_info(ctx))

    # Visit a parse tree produced by FizzParser#return_stmt.
    def visitReturn_stmt(self, ctx:FizzParser.Return_stmtContext):
        return_stmt = ast.ReturnStmt(source_info=get_source_info(ctx))
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitReturn_stmt child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.TestlistContext):
                    return_stmt.py_expr = self.get_py_str(child)
                    return_stmt.expr.py_expr = return_stmt.py_expr
                    return_stmt.expr.source_info.CopyFrom(get_source_info(child))
                    continue

                self.log_childtree(child)
                childProto = self.visit(child)
                print("visitReturn_stmt childProto",childProto)
                raise Exception("visitReturn_stmt childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto)
            elif hasattr(child, 'getSymbol'):
                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.COLON
                ):
                    continue
                self.log_symbol(child)
            else:
                print("visitReturn_stmt child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitReturn_stmt child (unknown) type")
        return return_stmt


    # Visit a parse tree produced by FizzParser#exprlist.
    def visitExprlist(self, ctx:FizzParser.ExprlistContext):
        py_exprs = []
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitExprlist child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.ExprContext):
                    py_str = self.get_py_str(child)
                    print("visitExprlist full text of child\n",py_str)
                    py_expr = BuildAstVisitor.transform_code(py_str)
                    py_exprs.append(py_expr)
                    continue
                self.log_childtree(child)
                childProto = self.visit(child)
                print("visitExprlist childProto",childProto)
                raise Exception("visitExprlist childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto)

        return py_exprs

    # Visit a parse tree produced by FizzParser#invariant_stmt.
    def visitInvariant_stmt(self, ctx:FizzParser.Invariant_stmtContext):
        print("\n\nvisitInvariant_stmt",ctx.__class__.__name__)
        print("visitInvariant_stmt\n",ctx.getText())
        invariant = ast.Invariant(source_info=get_source_info(ctx))
        rootInvariant = invariant
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitInvariant_stmt child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.TestContext):
                    py_str = self.get_py_str(child)
                    print("visitExpr_stmt full text\n",py_str)
                    invariant.pyExpr = BuildAstVisitor.transform_code(py_str)
                    continue
                self.log_childtree(child)
                childProto = self.visit(child)
                print("visitInvariant_stmt childProto",childProto)
            elif hasattr(child, 'getSymbol'):
                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.COLON
                ):
                    continue
                if child.getSymbol().type == FizzParser.ALWAYS:
                    if invariant.eventually:
                        invariant.nested.CopyFrom(ast.Invariant())
                        invariant = invariant.nested
                    invariant.always = True
                    continue
                if child.getSymbol().type == FizzParser.EVENTUALLY:
                    invariant.eventually = True
                    continue
                self.log_symbol(child)
            else:
                print("visitInvariant_stmt child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitInvariant_stmt child (unknown) type")

        print("visitInvariant_stmt invariant", rootInvariant)
        return rootInvariant

    def build_for_clause(self, ctx):
        clause = ast.ForClause(source_info=get_source_info(ctx))
        tests = []
        for child in ctx.getChildren():
            if isinstance(child, FizzParser.ExprlistContext):
                targets = [t.strip() for t in self.visit(child)]
                for t in targets:
                    if not t.isidentifier():
                        raise formula_error(child, f"for {t} in ...: the targets on an assertion header must be names, like `for a, b in PAIRS`")
                clause.targets.extend(targets)
            elif isinstance(child, FizzParser.Logical_testContext):
                tests.append(self.get_py_str(child))
        clause.iterable_py_expr = tests[0]
        clause.filter_py_exprs.extend(tests[1:])
        return clause

    # A formula return: `return <formula>` (FizzParser.g4 `formula`). The
    # formula is kept for visitAssertiondef, which decides what the assertion
    # compiles to; the ReturnStmt returned here is a placeholder it replaces.
    def visitFormula_return_stmt(self, ctx:FizzParser.Formula_return_stmtContext):
        f = self.build_formula(ctx.formula())
        if not self.in_assertion and not f.temporal and not uses_implies(ctx.formula()):
            # possibly is a soft keyword: a return that uses it is parsed as
            # a formula (see ReturnsPossibly), but outside an assertion it is
            # a plain return that calls a function named possibly.
            return self.plain_return(ctx)
        if not self.in_assertion:
            word = "implies" if f.kind == "atom" else "a temporal operator"
            raise formula_error(ctx, f"{word} can only be used in the return of an assertion")
        return_stmt = ast.ReturnStmt(source_info=get_source_info(ctx))
        if f.kind == "atom":
            # Just an expression, e.g. `return a implies b`.
            return_stmt.py_expr = f.text
            return_stmt.expr.py_expr = f.text
            return_stmt.expr.source_info.CopyFrom(get_source_info(ctx.formula()))
            return return_stmt
        self.formula_returns.append((f, ctx))
        return return_stmt

    def plain_return(self, ctx):
        return_stmt = ast.ReturnStmt(source_info=get_source_info(ctx))
        return_stmt.py_expr = self.get_py_str(ctx.formula())
        return_stmt.expr.py_expr = return_stmt.py_expr
        return_stmt.expr.source_info.CopyFrom(get_source_info(ctx.formula()))
        return return_stmt

    def build_formula(self, ctx):
        if isinstance(ctx, FizzParser.FormulaContext):
            operands = [self.build_formula(c) for c in ctx.formula_or()]
            ops = [c.getText() for c in ctx.formula_binop()]
            if len(ops) > 1:
                raise formula_error(ctx, f"use parentheses to combine {ops[0]} and {ops[1]}; they don't chain")
            if not ops:
                return operands[0]
            check_left_operand(operands[0], ops[0], self.input_stream)
            return Formula.combine(ops[0], operands, ctx)
        if isinstance(ctx, FizzParser.Formula_orContext) or isinstance(ctx, FizzParser.Formula_andContext):
            children = ctx.formula_and() if isinstance(ctx, FizzParser.Formula_orContext) else ctx.formula_unary()
            operands = [self.build_formula(c) for c in children]
            if len(operands) == 1:
                return operands[0]
            op = "or" if isinstance(ctx, FizzParser.Formula_orContext) else "and"
            for operand in operands[:-1]:
                check_left_operand(operand, op, self.input_stream)
            return Formula.combine(op, operands, ctx)
        if isinstance(ctx, FizzParser.Formula_unaryContext):
            if ctx.comparison() is not None:
                return Formula.atom(BuildAstVisitor.transform_code(self.get_py_str(ctx.comparison())), ctx)
            if ctx.formula() is not None:
                inner = self.build_formula(ctx.formula())
                return inner.parenthesized()
            if ctx.formula_possibly() is not None or ctx.EXISTS() is not None:
                return self.build_possibly(ctx)
            op = ctx.getChild(0).getText()
            return Formula.combine(op, [self.build_formula(ctx.formula_unary())], ctx)
        raise Exception("build_formula: unexpected node", ctx.__class__.__name__)

    def build_possibly(self, ctx):
        """`possibly Q`, or its other spelling `exists eventually Q`. Q must
        be a condition on one state: atoms and nested `possibly`."""
        operand_ctx = ctx.formula_unary()
        if ctx.EXISTS() is not None:
            if operand_ctx.EVENTUALLY() is None:
                raise formula_error(ctx, "inside a formula, `exists` is only used as `exists eventually`; "
                                    "write `possibly Q` for \"some path from here reaches Q\"")
            operand_ctx = operand_ctx.formula_unary()
        operand = self.build_formula(operand_ctx)
        if operand.kind == "eventually":
            raise formula_error(ctx, "`possibly (eventually Q)` means the same as `possibly Q`; drop the `eventually`")
        if operand.temporal:
            raise formula_error(ctx, "`possibly` takes a condition on a state, which can't contain "
                                "always, eventually, until or leadsto")
        return Formula.combine("possibly", [operand], ctx)

    def finish_formula_assertion(self, ctx, invariant, formula_returns):
        """Checks the formula rules for an assertion and compiles its formula.

        An assertion without a prefix must end with `return <formula>`, and
        that must be its only return. The formulas `always P`,
        `always eventually P` and `eventually always P` compile to today's
        prefixed assertions, so they are checked exactly as before. Any other
        formula keeps its tree in invariant.formula, and the final return
        gives the list of its atoms' values.
        """
        if invariant.temporal_operators:
            if formula_returns:
                raise formula_error(formula_returns[0][1], f"assertion {invariant.name} already has temporal operators "
                                    "before `assertion`; remove them to return a temporal formula")
            return
        stmts = invariant.block.stmts
        if len(formula_returns) != 1 or not stmts or not stmts[-1].HasField("return_stmt") \
                or stmts[-1].return_stmt.source_info != get_source_info(formula_returns[0][1]):
            if formula_returns:
                raise formula_error(formula_returns[-1][1], f"assertion {invariant.name} must have a single return, "
                                    "as its last statement; compute a local and use it in the formula")
            raise formula_error(ctx, f"assertion {invariant.name} must end with `return` and a temporal formula, "
                                "such as `return always P` or `return P leadsto Q`")
        if count_returns(invariant.block) != 1:
            raise formula_error(formula_returns[0][1], f"assertion {invariant.name} must have a single return, "
                                "as its last statement; compute a local and use it in the formula")
        f = formula_returns[0][0]
        ret = stmts[-1].return_stmt
        classic = f.classic_operators()
        if classic is not None:
            ops, body = classic
            invariant.temporal_operators.extend(ops)
            ret.py_expr = body.text
        else:
            atoms = []
            invariant.formula.CopyFrom(f.to_proto(atoms))
            # Each use of possibly is computed into one more atom.
            count = len(atoms) + f.count("possibly")
            if count > MAX_FORMULA_ATOMS:
                raise formula_error(formula_returns[0][1], f"assertion {invariant.name} needs {count} atoms "
                                    f"(each distinct condition and each use of possibly is one); "
                                    f"at most {MAX_FORMULA_ATOMS} are supported")
            ret.py_expr = "[" + ", ".join(f"bool({a})" for a in atoms) + "]"
        ret.expr.py_expr = ret.py_expr
        ret.expr.source_info.CopyFrom(get_source_info(formula_returns[0][1]))

    def get_py_str(self, child):
        return self.input_stream.getText(child.start.start, child.stop.stop)

    # Visit a parse tree produced by FizzParser#invariants_suite.
    def visitInvariants_suite(self, ctx:FizzParser.Invariants_suiteContext):
        invariants = []
        for i, child in enumerate(ctx.getChildren()):
            if hasattr(child, 'toStringTree'):
                childProto = self.visit(child)
                if isinstance(childProto, ast.Invariant):
                    invariants.append(childProto)
                else:
                    print("visitInvariants_suite childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto)
                    raise Exception("visitInvariants_suite childProto (unknown) type")

        return invariants

    def log_symbol(self, child):
        print("log_symbol SymbolName",FizzParser.symbolicNames[child.getSymbol().type])
        print("log_symbol getSymbol",child.__class__.__name__,child.getSymbol(), dir(child))
        print("log_symbol symbol dir",dir(child.getSymbol()))
        print("log_symbol symbol type",child.getSymbol().type)

    def log_childtree(self, child):
        print("log_childtree child",child.__class__.__name__,child.getText())
        print("log_childtree child",dir(child))
        print("log_childtree child",child.getChildCount())
        print("log_childtree child",child.getRuleIndex())
        print("log_childtree child",child.getRuleContext())
        print("log_childtree child payload",child.getPayload())
        print("log_childtree child full text\n", self.get_py_str(child))
        print("---")

def get_source_info(ctx):
    start = ast.Position(line=ctx.start.line, column=ctx.start.column+1)
    # Token only has start column, not the end column.
    # So using the length to calculate the end column, it works for all cases
    # except multi line strings.
    # TODO: Handle multi line strings
    print(dir(ctx.stop))
    endTokenStart = ctx.stop.column
    endTokenLength = len(ctx.stop.text)
    endTokenEnd = endTokenStart + endTokenLength

    end = ast.Position(line=ctx.stop.line, column=endTokenEnd)
    source_info = ast.SourceInfo(start=start, end=end)
    return source_info


def formula_error(ctx, message):
    return SyntaxError(f"line {ctx.start.line}:{ctx.start.column + 1}: {message}")


def count_returns(block):
    count = 0
    for stmt in block.stmts:
        if stmt.HasField("return_stmt"):
            count += 1
        for field in ("block",):
            if stmt.HasField(field):
                count += count_returns(getattr(stmt, field))
        if stmt.HasField("if_stmt"):
            for branch in stmt.if_stmt.branches:
                count += count_returns(branch.block)
        if stmt.HasField("for_stmt"):
            count += count_returns(stmt.for_stmt.block)
        if stmt.HasField("while_stmt"):
            count += count_returns(stmt.while_stmt.block)
    return count


def uses_implies(ctx):
    """Whether a formula's parse tree uses implies as an operator, outside
    the expressions that are its atoms."""
    if isinstance(ctx, FizzParser.ComparisonContext):
        return False
    if isinstance(ctx, FizzParser.Formula_binopContext):
        return ctx.getText() == "implies"
    return any(uses_implies(c) for c in ctx.getChildren() if isinstance(c, ParserRuleContext))


def check_left_operand(f, op, input_stream):
    """A prefix temporal operator binds tighter than any binary, so in
    `always a implies b` it applies to `a` alone. Require parentheses there,
    since it rarely means what it reads as."""
    n = f
    while not n.paren:
        if n.kind in ("always", "eventually", "possibly"):
            arg = n.args[0]
            arg_text = input_stream.getText(arg.start.start.start, arg.stop.stop.stop)
            raise formula_error(n.ctx, f"`{n.kind}` here applies only to `{arg_text}`, not to the `{op}` after it; "
                                f"add parentheses to say which you mean")
        if n.kind == "not" or (n.kind in Formula.BINARY and n.args):
            n = n.args[-1]
            continue
        break


MAX_FORMULA_ATOMS = 64


class Formula:
    """A temporal formula while it is being built. Subtrees without a
    temporal operator or `possibly` collapse into atoms: Starlark
    expressions, evaluated in each state, with `implies` written out."""
    TEMPORAL = ("always", "eventually", "until", "leadsto")
    BINARY = ("and", "or", "implies", "until", "leadsto")
    OPS = {
        "not": ast.Formula.NOT, "and": ast.Formula.AND, "or": ast.Formula.OR,
        "implies": ast.Formula.IMPLIES, "always": ast.Formula.ALWAYS,
        "eventually": ast.Formula.EVENTUALLY, "until": ast.Formula.UNTIL,
        "leadsto": ast.Formula.LEADSTO, "possibly": ast.Formula.POSSIBLY,
    }

    def __init__(self, kind, args, ctx, text=""):
        self.kind = kind
        self.args = args
        # ctx is a parse tree node, or a (first, last) pair of them for an
        # atom merged from adjacent operands.
        self.start, self.stop = ctx if isinstance(ctx, tuple) else (ctx, ctx)
        self.ctx = self.start
        self.text = text  # Starlark text, for atoms
        self.paren = False
        # Whether the formula has a linear-time operator. possibly isn't
        # one: `possibly Q` is a condition on a state, like an atom, but it
        # isn't a Starlark expression, so it never merges into one.
        self.temporal = kind in Formula.TEMPORAL or any(a.temporal for a in args)

    @staticmethod
    def atom(text, ctx):
        return Formula("atom", [], ctx, text)

    def parenthesized(self):
        f = Formula(self.kind, self.args, (self.start, self.stop), self.text)
        if f.kind == "atom":
            f.text = f"({self.text})"
        f.paren = True
        return f

    @staticmethod
    def combine(op, operands, ctx):
        if op in Formula.TEMPORAL or op == "possibly":
            return Formula(op, operands, ctx)
        if op == "not":
            if operands[0].kind == "atom":
                return Formula.atom(f"not ({operands[0].text})", ctx)
            return Formula(op, operands, ctx)
        if op == "implies":
            if all(o.kind == "atom" for o in operands):
                return Formula.atom(f"(not ({operands[0].text})) or ({operands[1].text})", ctx)
            return Formula(op, operands, ctx)
        # and / or: merge each run of adjacent atoms into one atom, keeping
        # their order so short-circuiting inside the atom is unchanged.
        merged = []
        for o in operands:
            if o.kind == "atom" and merged and merged[-1].kind == "atom":
                merged[-1] = Formula.atom(f"({merged[-1].text}) {op} ({o.text})", (merged[-1].start, o.stop))
            else:
                merged.append(o)
        if len(merged) == 1:
            return merged[0]
        return Formula(op, merged, ctx)

    def classic_operators(self):
        """The prefix operators and body when this formula is one of the
        forms prefixed assertions already support, else None."""
        if self.kind == "always" and self.args[0].kind == "atom":
            return ["always"], self.args[0]
        if self.kind in ("always", "eventually") and self.args[0].kind in ("always", "eventually") \
                and self.args[0].kind != self.kind and self.args[0].args[0].kind == "atom":
            return [self.kind, self.args[0].kind], self.args[0].args[0]
        return None

    def count(self, kind):
        return (self.kind == kind) + sum(a.count(kind) for a in self.args)

    def to_proto(self, atoms):
        source_info = get_source_info(self.start)
        source_info.end.CopyFrom(get_source_info(self.stop).end)
        p = ast.Formula(source_info=source_info)
        if self.kind == "atom":
            p.op = ast.Formula.ATOM
            if self.text not in atoms:
                atoms.append(self.text)
            p.atom = atoms.index(self.text)
            return p
        p.op = Formula.OPS[self.kind]
        for a in self.args:
            p.args.append(a.to_proto(atoms))
        return p
