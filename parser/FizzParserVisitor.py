# Generated from FizzParser.g4 by ANTLR 4.13.1
from antlr4 import *
if "." in __name__:
    from .FizzParser import FizzParser
else:
    from FizzParser import FizzParser

# This class defines a complete generic visitor for a parse tree produced by FizzParser.

class FizzParserVisitor(ParseTreeVisitor):

    # Visit a parse tree produced by FizzParser#root.
    def visitRoot(self, ctx:FizzParser.RootContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#single_input.
    def visitSingle_input(self, ctx:FizzParser.Single_inputContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#file_input.
    def visitFile_input(self, ctx:FizzParser.File_inputContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#eval_input.
    def visitEval_input(self, ctx:FizzParser.Eval_inputContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#stmt.
    def visitStmt(self, ctx:FizzParser.StmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#labeled_stmt.
    def visitLabeled_stmt(self, ctx:FizzParser.Labeled_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#if_stmt.
    def visitIf_stmt(self, ctx:FizzParser.If_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#while_stmt.
    def visitWhile_stmt(self, ctx:FizzParser.While_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#for_stmt.
    def visitFor_stmt(self, ctx:FizzParser.For_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#try_stmt.
    def visitTry_stmt(self, ctx:FizzParser.Try_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#with_stmt.
    def visitWith_stmt(self, ctx:FizzParser.With_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#class_or_func_def_stmt.
    def visitClass_or_func_def_stmt(self, ctx:FizzParser.Class_or_func_def_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#any_stmt.
    def visitAny_stmt(self, ctx:FizzParser.Any_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#init_stmt.
    def visitInit_stmt(self, ctx:FizzParser.Init_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#invariants_stmt.
    def visitInvariants_stmt(self, ctx:FizzParser.Invariants_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#assertion_stmt.
    def visitAssertion_stmt(self, ctx:FizzParser.Assertion_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#action_stmt.
    def visitAction_stmt(self, ctx:FizzParser.Action_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#function_stmt.
    def visitFunction_stmt(self, ctx:FizzParser.Function_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#flow_stmt.
    def visitFlow_stmt(self, ctx:FizzParser.Flow_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#suite.
    def visitSuite(self, ctx:FizzParser.SuiteContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#invariants_suite.
    def visitInvariants_suite(self, ctx:FizzParser.Invariants_suiteContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#invariant_stmt.
    def visitInvariant_stmt(self, ctx:FizzParser.Invariant_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#decorator.
    def visitDecorator(self, ctx:FizzParser.DecoratorContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#elif_clause.
    def visitElif_clause(self, ctx:FizzParser.Elif_clauseContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#else_clause.
    def visitElse_clause(self, ctx:FizzParser.Else_clauseContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#finally_clause.
    def visitFinally_clause(self, ctx:FizzParser.Finally_clauseContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#with_item.
    def visitWith_item(self, ctx:FizzParser.With_itemContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#except_clause.
    def visitExcept_clause(self, ctx:FizzParser.Except_clauseContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#classdef.
    def visitClassdef(self, ctx:FizzParser.ClassdefContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#funcdef.
    def visitFuncdef(self, ctx:FizzParser.FuncdefContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#actiondef.
    def visitActiondef(self, ctx:FizzParser.ActiondefContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#fairness.
    def visitFairness(self, ctx:FizzParser.FairnessContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#functiondef.
    def visitFunctiondef(self, ctx:FizzParser.FunctiondefContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#assertiondef.
    def visitAssertiondef(self, ctx:FizzParser.AssertiondefContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#typedargslist.
    def visitTypedargslist(self, ctx:FizzParser.TypedargslistContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#args.
    def visitArgs(self, ctx:FizzParser.ArgsContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#kwargs.
    def visitKwargs(self, ctx:FizzParser.KwargsContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#def_parameters.
    def visitDef_parameters(self, ctx:FizzParser.Def_parametersContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#def_parameter.
    def visitDef_parameter(self, ctx:FizzParser.Def_parameterContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#named_parameter.
    def visitNamed_parameter(self, ctx:FizzParser.Named_parameterContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#simple_stmt.
    def visitSimple_stmt(self, ctx:FizzParser.Simple_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#func_call_stmt.
    def visitFunc_call_stmt(self, ctx:FizzParser.Func_call_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#expr_stmt.
    def visitExpr_stmt(self, ctx:FizzParser.Expr_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#print_stmt.
    def visitPrint_stmt(self, ctx:FizzParser.Print_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#del_stmt.
    def visitDel_stmt(self, ctx:FizzParser.Del_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#pass_stmt.
    def visitPass_stmt(self, ctx:FizzParser.Pass_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#break_stmt.
    def visitBreak_stmt(self, ctx:FizzParser.Break_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#continue_stmt.
    def visitContinue_stmt(self, ctx:FizzParser.Continue_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#return_stmt.
    def visitReturn_stmt(self, ctx:FizzParser.Return_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#raise_stmt.
    def visitRaise_stmt(self, ctx:FizzParser.Raise_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#yield_stmt.
    def visitYield_stmt(self, ctx:FizzParser.Yield_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#import_stmt.
    def visitImport_stmt(self, ctx:FizzParser.Import_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#from_stmt.
    def visitFrom_stmt(self, ctx:FizzParser.From_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#global_stmt.
    def visitGlobal_stmt(self, ctx:FizzParser.Global_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#exec_stmt.
    def visitExec_stmt(self, ctx:FizzParser.Exec_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#assert_stmt.
    def visitAssert_stmt(self, ctx:FizzParser.Assert_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#nonlocal_stmt.
    def visitNonlocal_stmt(self, ctx:FizzParser.Nonlocal_stmtContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#testlist_star_expr.
    def visitTestlist_star_expr(self, ctx:FizzParser.Testlist_star_exprContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#star_expr.
    def visitStar_expr(self, ctx:FizzParser.Star_exprContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#assign_part.
    def visitAssign_part(self, ctx:FizzParser.Assign_partContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#exprlist.
    def visitExprlist(self, ctx:FizzParser.ExprlistContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#import_as_names.
    def visitImport_as_names(self, ctx:FizzParser.Import_as_namesContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#import_as_name.
    def visitImport_as_name(self, ctx:FizzParser.Import_as_nameContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#dotted_as_names.
    def visitDotted_as_names(self, ctx:FizzParser.Dotted_as_namesContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#dotted_as_name.
    def visitDotted_as_name(self, ctx:FizzParser.Dotted_as_nameContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#test.
    def visitTest(self, ctx:FizzParser.TestContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#varargslist.
    def visitVarargslist(self, ctx:FizzParser.VarargslistContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#vardef_parameters.
    def visitVardef_parameters(self, ctx:FizzParser.Vardef_parametersContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#vardef_parameter.
    def visitVardef_parameter(self, ctx:FizzParser.Vardef_parameterContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#varargs.
    def visitVarargs(self, ctx:FizzParser.VarargsContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#varkwargs.
    def visitVarkwargs(self, ctx:FizzParser.VarkwargsContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#logical_test.
    def visitLogical_test(self, ctx:FizzParser.Logical_testContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#comparison.
    def visitComparison(self, ctx:FizzParser.ComparisonContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#expr.
    def visitExpr(self, ctx:FizzParser.ExprContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#atom.
    def visitAtom(self, ctx:FizzParser.AtomContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#dictorsetmaker.
    def visitDictorsetmaker(self, ctx:FizzParser.DictorsetmakerContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#testlist_comp.
    def visitTestlist_comp(self, ctx:FizzParser.Testlist_compContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#testlist.
    def visitTestlist(self, ctx:FizzParser.TestlistContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#dotted_name.
    def visitDotted_name(self, ctx:FizzParser.Dotted_nameContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#name.
    def visitName(self, ctx:FizzParser.NameContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#number.
    def visitNumber(self, ctx:FizzParser.NumberContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#integer.
    def visitInteger(self, ctx:FizzParser.IntegerContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#yield_expr.
    def visitYield_expr(self, ctx:FizzParser.Yield_exprContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#yield_arg.
    def visitYield_arg(self, ctx:FizzParser.Yield_argContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#trailer.
    def visitTrailer(self, ctx:FizzParser.TrailerContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#arguments.
    def visitArguments(self, ctx:FizzParser.ArgumentsContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#arglist.
    def visitArglist(self, ctx:FizzParser.ArglistContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#argument.
    def visitArgument(self, ctx:FizzParser.ArgumentContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#subscriptlist.
    def visitSubscriptlist(self, ctx:FizzParser.SubscriptlistContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#subscript.
    def visitSubscript(self, ctx:FizzParser.SubscriptContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#sliceop.
    def visitSliceop(self, ctx:FizzParser.SliceopContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#comp_for.
    def visitComp_for(self, ctx:FizzParser.Comp_forContext):
        return self.visitChildren(ctx)


    # Visit a parse tree produced by FizzParser#comp_iter.
    def visitComp_iter(self, ctx:FizzParser.Comp_iterContext):
        return self.visitChildren(ctx)



del FizzParser