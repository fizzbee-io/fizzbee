
from antlr4 import *
import sys

from parser.FizzParser import FizzParser
from parser.FizzParserVisitor import FizzParserVisitor
import proto.fizz_ast_pb2 as ast


class BuildAstVisitor(FizzParserVisitor):
    def __init__(self, input_stream):
        super().__init__()
        self.input_stream = input_stream

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

        file = ast.File()

        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitFile_input child index",i,child.getText())
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

    def is_list_of_type(lst, item_type):
        if not isinstance(lst, list):
            return False
        # Check if all elements in the list are instances of ast.Invariant
        return all(isinstance(item, item_type) for item in lst)

    def visitInit_stmt(self, ctx:FizzParser.Init_stmtContext):
        init_str = self.get_py_str(ctx)
        py_str = BuildAstVisitor.transform_code(init_str, 1)
        return ast.StateVars(code=py_str)

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

        action = ast.Action()
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
        fairness = ast.Fairness()
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

        invariant = ast.Invariant()
        py_code = "def "
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitAssertiondef child index",i,child.getText())
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

                print("visitAssertiondef childProto",childProto)
            elif hasattr(child, 'getSymbol'):

                if (child.getSymbol().type == FizzParser.LINE_BREAK
                        or child.getSymbol().type == FizzParser.ASSERTION
                        or child.getSymbol().type == FizzParser.COLON
                ):
                    continue
                if child.getSymbol().type == FizzParser.EVENTUALLY or child.getSymbol().type == FizzParser.ALWAYS:
                    invariant.temporal_operators.append(child.getText())
                    continue

                self.log_symbol(child)
            else:
                print("visitAssertiondef child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitAssertiondef child (unknown) type")

        block_str = self.get_py_str(ctx)
        py_code += '\n'.join(block_str.split('\n')[1:])
        invariant.block.flow = ast.Flow.FLOW_ATOMIC
        invariant.py_code = py_code
        print("assertion", invariant)
        return invariant

    # Visit a parse tree produced by FizzParser#functiondef.
    def visitFunctiondef(self, ctx:FizzParser.FunctiondefContext):
        print("\n\nvisitFunctiondef",ctx.__class__.__name__)
        print("visitFunctiondef",ctx.getText())
        print("visitFunctiondef",dir(ctx))
        print("visitFunctiondef children count",ctx.getChildCount())
        print("visitFunctiondef full text\n", self.get_py_str(ctx))

        function = ast.Function()

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

        func_call = ast.CallStmt()
        has_assign = False
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
                    else:
                        func_call.name = child.getText()
                    continue
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
        argument = ast.Argument()
        py_str = self.get_py_str(ctx)
        argument.py_expr = BuildAstVisitor.transform_code(py_str)
        return argument

    # Visit a parse tree produced by FizzParser#expr_stmt.
    def visitExpr_stmt(self, ctx:FizzParser.Expr_stmtContext):
        py_str = self.get_py_str(ctx)
        print("visitExpr_stmt full text\n",py_str)
        py_str = BuildAstVisitor.transform_code(py_str)
        return ast.PyStmt(code=py_str)

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
            block = ast.Block()
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
        block = ast.Block()
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitSuite child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                self.log_childtree(child)
                childProto = self.visit(child)
                if isinstance(childProto, ast.Statement):
                    block.stmts.append(childProto)
                    continue

                print("visitSuite childProto",childProto)
                raise Exception("visitSuite childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto)
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
            if count != 1:
                raise Exception("visitStmt child count != 1", count, ctx.getText())
        childProto = self.visit(childStmt)
        if childProto is None:
            return None
        if isinstance(childProto, ast.PyStmt):
            return ast.Statement(py_stmt=childProto)
        elif isinstance(childProto, ast.Block):
            return ast.Statement(block=childProto)
        elif isinstance(childProto, ast.IfStmt):
            return ast.Statement(if_stmt=childProto)
        elif isinstance(childProto, ast.AnyStmt):
            return ast.Statement(any_stmt=childProto)
        elif isinstance(childProto, ast.ForStmt):
            return ast.Statement(for_stmt=childProto)
        elif isinstance(childProto, ast.WhileStmt):
            return ast.Statement(while_stmt=childProto)
        elif isinstance(childProto, ast.BreakStmt):
            return ast.Statement(break_stmt=childProto)
        elif isinstance(childProto, ast.ContinueStmt):
            return ast.Statement(continue_stmt=childProto)
        elif isinstance(childProto, ast.ReturnStmt):
            return ast.Statement(return_stmt=childProto)
        elif isinstance(childProto, ast.CallStmt):
            return ast.Statement(call_stmt=childProto)

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
        elif BuildAstVisitor.is_list_of_type(childProto, ast.Invariant):
            return childProto

        raise Exception("visitStmt childProto (unknown) type", childProto.__class__.__name__, dir(childProto), childProto)

    # Visit a parse tree produced by FizzParser#if_stmt.
    def visitIf_stmt(self, ctx:FizzParser.If_stmtContext):
        print("\n\nvisitIf_stmt",ctx.__class__.__name__)
        print("visitIf_stmt\n",ctx.getText())
        if_stmt = ast.IfStmt()
        branch = ast.Branch()
        if_stmt.branches.append(branch)

        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitIf_stmt child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                self.log_childtree(child)
                if isinstance(child, FizzParser.TestContext):
                    if_stmt.branches[0].condition = self.get_py_str(child)
                    continue
                childProto = self.visit(child)
                print("visitIf_stmt childProto",childProto, childProto.__class__.__name__, child.__class__.__name__)
                if isinstance(childProto, ast.Block):
                    if_stmt.branches[0].block.CopyFrom(childProto)
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
        branch = ast.Branch()
        branch.condition = self.get_py_str(ctx.getChild(1))
        branch.block.CopyFrom(self.visit(ctx.getChild(3)))
        print("visitElif_clause branch", branch)
        return branch


    # Visit a parse tree produced by FizzParser#else_clause.
    def visitElse_clause(self, ctx:FizzParser.Else_clauseContext):
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitElse_clause child index",i,child.getText())
            branch = ast.Branch()
            branch.condition = "True"
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
        any_stmt = ast.AnyStmt()
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitAny_stmt child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.ExprlistContext):
                    any_stmt.loop_vars.extend(self.visitExprlist(child))
                    continue
                if isinstance(child, FizzParser.TestlistContext):
                    any_stmt.py_expr = self.get_py_str(child)
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
                ):
                    continue
                self.log_symbol(child)
            else:
                print("visitAny_stmt child (unknown) type",child.__class__.__name__, dir(child))
                raise Exception("visitAny_stmt child (unknown) type")

        print("visitAny_stmt any_stmt", any_stmt)
        return any_stmt

    # Visit a parse tree produced by FizzParser#for_stmt.
    def visitFor_stmt(self, ctx:FizzParser.For_stmtContext):
        print("\n\nvisitFor_stmt",ctx.__class__.__name__)
        print("visitFor_stmt\n",ctx.getText())
        for_stmt = ast.ForStmt()
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
        while_stmt = ast.WhileStmt()
        flow = ast.Flow.FLOW_UNKNOWN
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitWhile_stmt child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.TestContext):
                    while_stmt.py_expr = self.get_py_str(child)
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

    # Visit a parse tree produced by FizzParser#break_stmt.
    def visitBreak_stmt(self, ctx:FizzParser.Break_stmtContext):
        return ast.BreakStmt()


    # Visit a parse tree produced by FizzParser#continue_stmt.
    def visitContinue_stmt(self, ctx:FizzParser.Continue_stmtContext):
        return ast.ContinueStmt()


    # Visit a parse tree produced by FizzParser#return_stmt.
    def visitReturn_stmt(self, ctx:FizzParser.Return_stmtContext):
        return_stmt = ast.ReturnStmt()
        for i, child in enumerate(ctx.getChildren()):
            print()
            print("visitReturn_stmt child index",i,child.getText())
            if hasattr(child, 'toStringTree'):
                if isinstance(child, FizzParser.TestlistContext):
                    return_stmt.py_expr = self.get_py_str(child)
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
        invariant = ast.Invariant()
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
        print("log_childtree child payloand",child.getPayload())
        print("log_childtree child full text\n", self.get_py_str(child))
        print("---")

