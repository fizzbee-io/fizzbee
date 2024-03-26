# Generated from FizzParser.g4 by ANTLR 4.13.1
from antlr4 import *
if "." in __name__:
    from .FizzParser import FizzParser
else:
    from FizzParser import FizzParser

# This class defines a complete listener for a parse tree produced by FizzParser.
class FizzParserListener(ParseTreeListener):

    # Enter a parse tree produced by FizzParser#root.
    def enterRoot(self, ctx:FizzParser.RootContext):
        pass

    # Exit a parse tree produced by FizzParser#root.
    def exitRoot(self, ctx:FizzParser.RootContext):
        pass


    # Enter a parse tree produced by FizzParser#single_input.
    def enterSingle_input(self, ctx:FizzParser.Single_inputContext):
        pass

    # Exit a parse tree produced by FizzParser#single_input.
    def exitSingle_input(self, ctx:FizzParser.Single_inputContext):
        pass


    # Enter a parse tree produced by FizzParser#file_input.
    def enterFile_input(self, ctx:FizzParser.File_inputContext):
        pass

    # Exit a parse tree produced by FizzParser#file_input.
    def exitFile_input(self, ctx:FizzParser.File_inputContext):
        pass


    # Enter a parse tree produced by FizzParser#eval_input.
    def enterEval_input(self, ctx:FizzParser.Eval_inputContext):
        pass

    # Exit a parse tree produced by FizzParser#eval_input.
    def exitEval_input(self, ctx:FizzParser.Eval_inputContext):
        pass


    # Enter a parse tree produced by FizzParser#stmt.
    def enterStmt(self, ctx:FizzParser.StmtContext):
        pass

    # Exit a parse tree produced by FizzParser#stmt.
    def exitStmt(self, ctx:FizzParser.StmtContext):
        pass


    # Enter a parse tree produced by FizzParser#labeled_stmt.
    def enterLabeled_stmt(self, ctx:FizzParser.Labeled_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#labeled_stmt.
    def exitLabeled_stmt(self, ctx:FizzParser.Labeled_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#if_stmt.
    def enterIf_stmt(self, ctx:FizzParser.If_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#if_stmt.
    def exitIf_stmt(self, ctx:FizzParser.If_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#while_stmt.
    def enterWhile_stmt(self, ctx:FizzParser.While_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#while_stmt.
    def exitWhile_stmt(self, ctx:FizzParser.While_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#for_stmt.
    def enterFor_stmt(self, ctx:FizzParser.For_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#for_stmt.
    def exitFor_stmt(self, ctx:FizzParser.For_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#try_stmt.
    def enterTry_stmt(self, ctx:FizzParser.Try_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#try_stmt.
    def exitTry_stmt(self, ctx:FizzParser.Try_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#with_stmt.
    def enterWith_stmt(self, ctx:FizzParser.With_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#with_stmt.
    def exitWith_stmt(self, ctx:FizzParser.With_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#class_or_func_def_stmt.
    def enterClass_or_func_def_stmt(self, ctx:FizzParser.Class_or_func_def_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#class_or_func_def_stmt.
    def exitClass_or_func_def_stmt(self, ctx:FizzParser.Class_or_func_def_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#any_stmt.
    def enterAny_stmt(self, ctx:FizzParser.Any_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#any_stmt.
    def exitAny_stmt(self, ctx:FizzParser.Any_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#init_stmt.
    def enterInit_stmt(self, ctx:FizzParser.Init_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#init_stmt.
    def exitInit_stmt(self, ctx:FizzParser.Init_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#invariants_stmt.
    def enterInvariants_stmt(self, ctx:FizzParser.Invariants_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#invariants_stmt.
    def exitInvariants_stmt(self, ctx:FizzParser.Invariants_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#assertion_stmt.
    def enterAssertion_stmt(self, ctx:FizzParser.Assertion_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#assertion_stmt.
    def exitAssertion_stmt(self, ctx:FizzParser.Assertion_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#action_stmt.
    def enterAction_stmt(self, ctx:FizzParser.Action_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#action_stmt.
    def exitAction_stmt(self, ctx:FizzParser.Action_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#function_stmt.
    def enterFunction_stmt(self, ctx:FizzParser.Function_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#function_stmt.
    def exitFunction_stmt(self, ctx:FizzParser.Function_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#flow_stmt.
    def enterFlow_stmt(self, ctx:FizzParser.Flow_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#flow_stmt.
    def exitFlow_stmt(self, ctx:FizzParser.Flow_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#suite.
    def enterSuite(self, ctx:FizzParser.SuiteContext):
        pass

    # Exit a parse tree produced by FizzParser#suite.
    def exitSuite(self, ctx:FizzParser.SuiteContext):
        pass


    # Enter a parse tree produced by FizzParser#invariants_suite.
    def enterInvariants_suite(self, ctx:FizzParser.Invariants_suiteContext):
        pass

    # Exit a parse tree produced by FizzParser#invariants_suite.
    def exitInvariants_suite(self, ctx:FizzParser.Invariants_suiteContext):
        pass


    # Enter a parse tree produced by FizzParser#invariant_stmt.
    def enterInvariant_stmt(self, ctx:FizzParser.Invariant_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#invariant_stmt.
    def exitInvariant_stmt(self, ctx:FizzParser.Invariant_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#decorator.
    def enterDecorator(self, ctx:FizzParser.DecoratorContext):
        pass

    # Exit a parse tree produced by FizzParser#decorator.
    def exitDecorator(self, ctx:FizzParser.DecoratorContext):
        pass


    # Enter a parse tree produced by FizzParser#elif_clause.
    def enterElif_clause(self, ctx:FizzParser.Elif_clauseContext):
        pass

    # Exit a parse tree produced by FizzParser#elif_clause.
    def exitElif_clause(self, ctx:FizzParser.Elif_clauseContext):
        pass


    # Enter a parse tree produced by FizzParser#else_clause.
    def enterElse_clause(self, ctx:FizzParser.Else_clauseContext):
        pass

    # Exit a parse tree produced by FizzParser#else_clause.
    def exitElse_clause(self, ctx:FizzParser.Else_clauseContext):
        pass


    # Enter a parse tree produced by FizzParser#finally_clause.
    def enterFinally_clause(self, ctx:FizzParser.Finally_clauseContext):
        pass

    # Exit a parse tree produced by FizzParser#finally_clause.
    def exitFinally_clause(self, ctx:FizzParser.Finally_clauseContext):
        pass


    # Enter a parse tree produced by FizzParser#with_item.
    def enterWith_item(self, ctx:FizzParser.With_itemContext):
        pass

    # Exit a parse tree produced by FizzParser#with_item.
    def exitWith_item(self, ctx:FizzParser.With_itemContext):
        pass


    # Enter a parse tree produced by FizzParser#except_clause.
    def enterExcept_clause(self, ctx:FizzParser.Except_clauseContext):
        pass

    # Exit a parse tree produced by FizzParser#except_clause.
    def exitExcept_clause(self, ctx:FizzParser.Except_clauseContext):
        pass


    # Enter a parse tree produced by FizzParser#classdef.
    def enterClassdef(self, ctx:FizzParser.ClassdefContext):
        pass

    # Exit a parse tree produced by FizzParser#classdef.
    def exitClassdef(self, ctx:FizzParser.ClassdefContext):
        pass


    # Enter a parse tree produced by FizzParser#funcdef.
    def enterFuncdef(self, ctx:FizzParser.FuncdefContext):
        pass

    # Exit a parse tree produced by FizzParser#funcdef.
    def exitFuncdef(self, ctx:FizzParser.FuncdefContext):
        pass


    # Enter a parse tree produced by FizzParser#actiondef.
    def enterActiondef(self, ctx:FizzParser.ActiondefContext):
        pass

    # Exit a parse tree produced by FizzParser#actiondef.
    def exitActiondef(self, ctx:FizzParser.ActiondefContext):
        pass


    # Enter a parse tree produced by FizzParser#fairness.
    def enterFairness(self, ctx:FizzParser.FairnessContext):
        pass

    # Exit a parse tree produced by FizzParser#fairness.
    def exitFairness(self, ctx:FizzParser.FairnessContext):
        pass


    # Enter a parse tree produced by FizzParser#functiondef.
    def enterFunctiondef(self, ctx:FizzParser.FunctiondefContext):
        pass

    # Exit a parse tree produced by FizzParser#functiondef.
    def exitFunctiondef(self, ctx:FizzParser.FunctiondefContext):
        pass


    # Enter a parse tree produced by FizzParser#assertiondef.
    def enterAssertiondef(self, ctx:FizzParser.AssertiondefContext):
        pass

    # Exit a parse tree produced by FizzParser#assertiondef.
    def exitAssertiondef(self, ctx:FizzParser.AssertiondefContext):
        pass


    # Enter a parse tree produced by FizzParser#typedargslist.
    def enterTypedargslist(self, ctx:FizzParser.TypedargslistContext):
        pass

    # Exit a parse tree produced by FizzParser#typedargslist.
    def exitTypedargslist(self, ctx:FizzParser.TypedargslistContext):
        pass


    # Enter a parse tree produced by FizzParser#args.
    def enterArgs(self, ctx:FizzParser.ArgsContext):
        pass

    # Exit a parse tree produced by FizzParser#args.
    def exitArgs(self, ctx:FizzParser.ArgsContext):
        pass


    # Enter a parse tree produced by FizzParser#kwargs.
    def enterKwargs(self, ctx:FizzParser.KwargsContext):
        pass

    # Exit a parse tree produced by FizzParser#kwargs.
    def exitKwargs(self, ctx:FizzParser.KwargsContext):
        pass


    # Enter a parse tree produced by FizzParser#def_parameters.
    def enterDef_parameters(self, ctx:FizzParser.Def_parametersContext):
        pass

    # Exit a parse tree produced by FizzParser#def_parameters.
    def exitDef_parameters(self, ctx:FizzParser.Def_parametersContext):
        pass


    # Enter a parse tree produced by FizzParser#def_parameter.
    def enterDef_parameter(self, ctx:FizzParser.Def_parameterContext):
        pass

    # Exit a parse tree produced by FizzParser#def_parameter.
    def exitDef_parameter(self, ctx:FizzParser.Def_parameterContext):
        pass


    # Enter a parse tree produced by FizzParser#named_parameter.
    def enterNamed_parameter(self, ctx:FizzParser.Named_parameterContext):
        pass

    # Exit a parse tree produced by FizzParser#named_parameter.
    def exitNamed_parameter(self, ctx:FizzParser.Named_parameterContext):
        pass


    # Enter a parse tree produced by FizzParser#simple_stmt.
    def enterSimple_stmt(self, ctx:FizzParser.Simple_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#simple_stmt.
    def exitSimple_stmt(self, ctx:FizzParser.Simple_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#func_call_stmt.
    def enterFunc_call_stmt(self, ctx:FizzParser.Func_call_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#func_call_stmt.
    def exitFunc_call_stmt(self, ctx:FizzParser.Func_call_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#expr_stmt.
    def enterExpr_stmt(self, ctx:FizzParser.Expr_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#expr_stmt.
    def exitExpr_stmt(self, ctx:FizzParser.Expr_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#print_stmt.
    def enterPrint_stmt(self, ctx:FizzParser.Print_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#print_stmt.
    def exitPrint_stmt(self, ctx:FizzParser.Print_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#del_stmt.
    def enterDel_stmt(self, ctx:FizzParser.Del_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#del_stmt.
    def exitDel_stmt(self, ctx:FizzParser.Del_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#pass_stmt.
    def enterPass_stmt(self, ctx:FizzParser.Pass_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#pass_stmt.
    def exitPass_stmt(self, ctx:FizzParser.Pass_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#break_stmt.
    def enterBreak_stmt(self, ctx:FizzParser.Break_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#break_stmt.
    def exitBreak_stmt(self, ctx:FizzParser.Break_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#continue_stmt.
    def enterContinue_stmt(self, ctx:FizzParser.Continue_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#continue_stmt.
    def exitContinue_stmt(self, ctx:FizzParser.Continue_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#return_stmt.
    def enterReturn_stmt(self, ctx:FizzParser.Return_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#return_stmt.
    def exitReturn_stmt(self, ctx:FizzParser.Return_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#raise_stmt.
    def enterRaise_stmt(self, ctx:FizzParser.Raise_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#raise_stmt.
    def exitRaise_stmt(self, ctx:FizzParser.Raise_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#yield_stmt.
    def enterYield_stmt(self, ctx:FizzParser.Yield_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#yield_stmt.
    def exitYield_stmt(self, ctx:FizzParser.Yield_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#import_stmt.
    def enterImport_stmt(self, ctx:FizzParser.Import_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#import_stmt.
    def exitImport_stmt(self, ctx:FizzParser.Import_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#from_stmt.
    def enterFrom_stmt(self, ctx:FizzParser.From_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#from_stmt.
    def exitFrom_stmt(self, ctx:FizzParser.From_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#global_stmt.
    def enterGlobal_stmt(self, ctx:FizzParser.Global_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#global_stmt.
    def exitGlobal_stmt(self, ctx:FizzParser.Global_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#exec_stmt.
    def enterExec_stmt(self, ctx:FizzParser.Exec_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#exec_stmt.
    def exitExec_stmt(self, ctx:FizzParser.Exec_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#assert_stmt.
    def enterAssert_stmt(self, ctx:FizzParser.Assert_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#assert_stmt.
    def exitAssert_stmt(self, ctx:FizzParser.Assert_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#nonlocal_stmt.
    def enterNonlocal_stmt(self, ctx:FizzParser.Nonlocal_stmtContext):
        pass

    # Exit a parse tree produced by FizzParser#nonlocal_stmt.
    def exitNonlocal_stmt(self, ctx:FizzParser.Nonlocal_stmtContext):
        pass


    # Enter a parse tree produced by FizzParser#testlist_star_expr.
    def enterTestlist_star_expr(self, ctx:FizzParser.Testlist_star_exprContext):
        pass

    # Exit a parse tree produced by FizzParser#testlist_star_expr.
    def exitTestlist_star_expr(self, ctx:FizzParser.Testlist_star_exprContext):
        pass


    # Enter a parse tree produced by FizzParser#star_expr.
    def enterStar_expr(self, ctx:FizzParser.Star_exprContext):
        pass

    # Exit a parse tree produced by FizzParser#star_expr.
    def exitStar_expr(self, ctx:FizzParser.Star_exprContext):
        pass


    # Enter a parse tree produced by FizzParser#assign_part.
    def enterAssign_part(self, ctx:FizzParser.Assign_partContext):
        pass

    # Exit a parse tree produced by FizzParser#assign_part.
    def exitAssign_part(self, ctx:FizzParser.Assign_partContext):
        pass


    # Enter a parse tree produced by FizzParser#exprlist.
    def enterExprlist(self, ctx:FizzParser.ExprlistContext):
        pass

    # Exit a parse tree produced by FizzParser#exprlist.
    def exitExprlist(self, ctx:FizzParser.ExprlistContext):
        pass


    # Enter a parse tree produced by FizzParser#import_as_names.
    def enterImport_as_names(self, ctx:FizzParser.Import_as_namesContext):
        pass

    # Exit a parse tree produced by FizzParser#import_as_names.
    def exitImport_as_names(self, ctx:FizzParser.Import_as_namesContext):
        pass


    # Enter a parse tree produced by FizzParser#import_as_name.
    def enterImport_as_name(self, ctx:FizzParser.Import_as_nameContext):
        pass

    # Exit a parse tree produced by FizzParser#import_as_name.
    def exitImport_as_name(self, ctx:FizzParser.Import_as_nameContext):
        pass


    # Enter a parse tree produced by FizzParser#dotted_as_names.
    def enterDotted_as_names(self, ctx:FizzParser.Dotted_as_namesContext):
        pass

    # Exit a parse tree produced by FizzParser#dotted_as_names.
    def exitDotted_as_names(self, ctx:FizzParser.Dotted_as_namesContext):
        pass


    # Enter a parse tree produced by FizzParser#dotted_as_name.
    def enterDotted_as_name(self, ctx:FizzParser.Dotted_as_nameContext):
        pass

    # Exit a parse tree produced by FizzParser#dotted_as_name.
    def exitDotted_as_name(self, ctx:FizzParser.Dotted_as_nameContext):
        pass


    # Enter a parse tree produced by FizzParser#test.
    def enterTest(self, ctx:FizzParser.TestContext):
        pass

    # Exit a parse tree produced by FizzParser#test.
    def exitTest(self, ctx:FizzParser.TestContext):
        pass


    # Enter a parse tree produced by FizzParser#varargslist.
    def enterVarargslist(self, ctx:FizzParser.VarargslistContext):
        pass

    # Exit a parse tree produced by FizzParser#varargslist.
    def exitVarargslist(self, ctx:FizzParser.VarargslistContext):
        pass


    # Enter a parse tree produced by FizzParser#vardef_parameters.
    def enterVardef_parameters(self, ctx:FizzParser.Vardef_parametersContext):
        pass

    # Exit a parse tree produced by FizzParser#vardef_parameters.
    def exitVardef_parameters(self, ctx:FizzParser.Vardef_parametersContext):
        pass


    # Enter a parse tree produced by FizzParser#vardef_parameter.
    def enterVardef_parameter(self, ctx:FizzParser.Vardef_parameterContext):
        pass

    # Exit a parse tree produced by FizzParser#vardef_parameter.
    def exitVardef_parameter(self, ctx:FizzParser.Vardef_parameterContext):
        pass


    # Enter a parse tree produced by FizzParser#varargs.
    def enterVarargs(self, ctx:FizzParser.VarargsContext):
        pass

    # Exit a parse tree produced by FizzParser#varargs.
    def exitVarargs(self, ctx:FizzParser.VarargsContext):
        pass


    # Enter a parse tree produced by FizzParser#varkwargs.
    def enterVarkwargs(self, ctx:FizzParser.VarkwargsContext):
        pass

    # Exit a parse tree produced by FizzParser#varkwargs.
    def exitVarkwargs(self, ctx:FizzParser.VarkwargsContext):
        pass


    # Enter a parse tree produced by FizzParser#logical_test.
    def enterLogical_test(self, ctx:FizzParser.Logical_testContext):
        pass

    # Exit a parse tree produced by FizzParser#logical_test.
    def exitLogical_test(self, ctx:FizzParser.Logical_testContext):
        pass


    # Enter a parse tree produced by FizzParser#comparison.
    def enterComparison(self, ctx:FizzParser.ComparisonContext):
        pass

    # Exit a parse tree produced by FizzParser#comparison.
    def exitComparison(self, ctx:FizzParser.ComparisonContext):
        pass


    # Enter a parse tree produced by FizzParser#expr.
    def enterExpr(self, ctx:FizzParser.ExprContext):
        pass

    # Exit a parse tree produced by FizzParser#expr.
    def exitExpr(self, ctx:FizzParser.ExprContext):
        pass


    # Enter a parse tree produced by FizzParser#atom.
    def enterAtom(self, ctx:FizzParser.AtomContext):
        pass

    # Exit a parse tree produced by FizzParser#atom.
    def exitAtom(self, ctx:FizzParser.AtomContext):
        pass


    # Enter a parse tree produced by FizzParser#dictorsetmaker.
    def enterDictorsetmaker(self, ctx:FizzParser.DictorsetmakerContext):
        pass

    # Exit a parse tree produced by FizzParser#dictorsetmaker.
    def exitDictorsetmaker(self, ctx:FizzParser.DictorsetmakerContext):
        pass


    # Enter a parse tree produced by FizzParser#testlist_comp.
    def enterTestlist_comp(self, ctx:FizzParser.Testlist_compContext):
        pass

    # Exit a parse tree produced by FizzParser#testlist_comp.
    def exitTestlist_comp(self, ctx:FizzParser.Testlist_compContext):
        pass


    # Enter a parse tree produced by FizzParser#testlist.
    def enterTestlist(self, ctx:FizzParser.TestlistContext):
        pass

    # Exit a parse tree produced by FizzParser#testlist.
    def exitTestlist(self, ctx:FizzParser.TestlistContext):
        pass


    # Enter a parse tree produced by FizzParser#dotted_name.
    def enterDotted_name(self, ctx:FizzParser.Dotted_nameContext):
        pass

    # Exit a parse tree produced by FizzParser#dotted_name.
    def exitDotted_name(self, ctx:FizzParser.Dotted_nameContext):
        pass


    # Enter a parse tree produced by FizzParser#name.
    def enterName(self, ctx:FizzParser.NameContext):
        pass

    # Exit a parse tree produced by FizzParser#name.
    def exitName(self, ctx:FizzParser.NameContext):
        pass


    # Enter a parse tree produced by FizzParser#number.
    def enterNumber(self, ctx:FizzParser.NumberContext):
        pass

    # Exit a parse tree produced by FizzParser#number.
    def exitNumber(self, ctx:FizzParser.NumberContext):
        pass


    # Enter a parse tree produced by FizzParser#integer.
    def enterInteger(self, ctx:FizzParser.IntegerContext):
        pass

    # Exit a parse tree produced by FizzParser#integer.
    def exitInteger(self, ctx:FizzParser.IntegerContext):
        pass


    # Enter a parse tree produced by FizzParser#yield_expr.
    def enterYield_expr(self, ctx:FizzParser.Yield_exprContext):
        pass

    # Exit a parse tree produced by FizzParser#yield_expr.
    def exitYield_expr(self, ctx:FizzParser.Yield_exprContext):
        pass


    # Enter a parse tree produced by FizzParser#yield_arg.
    def enterYield_arg(self, ctx:FizzParser.Yield_argContext):
        pass

    # Exit a parse tree produced by FizzParser#yield_arg.
    def exitYield_arg(self, ctx:FizzParser.Yield_argContext):
        pass


    # Enter a parse tree produced by FizzParser#trailer.
    def enterTrailer(self, ctx:FizzParser.TrailerContext):
        pass

    # Exit a parse tree produced by FizzParser#trailer.
    def exitTrailer(self, ctx:FizzParser.TrailerContext):
        pass


    # Enter a parse tree produced by FizzParser#arguments.
    def enterArguments(self, ctx:FizzParser.ArgumentsContext):
        pass

    # Exit a parse tree produced by FizzParser#arguments.
    def exitArguments(self, ctx:FizzParser.ArgumentsContext):
        pass


    # Enter a parse tree produced by FizzParser#arglist.
    def enterArglist(self, ctx:FizzParser.ArglistContext):
        pass

    # Exit a parse tree produced by FizzParser#arglist.
    def exitArglist(self, ctx:FizzParser.ArglistContext):
        pass


    # Enter a parse tree produced by FizzParser#argument.
    def enterArgument(self, ctx:FizzParser.ArgumentContext):
        pass

    # Exit a parse tree produced by FizzParser#argument.
    def exitArgument(self, ctx:FizzParser.ArgumentContext):
        pass


    # Enter a parse tree produced by FizzParser#subscriptlist.
    def enterSubscriptlist(self, ctx:FizzParser.SubscriptlistContext):
        pass

    # Exit a parse tree produced by FizzParser#subscriptlist.
    def exitSubscriptlist(self, ctx:FizzParser.SubscriptlistContext):
        pass


    # Enter a parse tree produced by FizzParser#subscript.
    def enterSubscript(self, ctx:FizzParser.SubscriptContext):
        pass

    # Exit a parse tree produced by FizzParser#subscript.
    def exitSubscript(self, ctx:FizzParser.SubscriptContext):
        pass


    # Enter a parse tree produced by FizzParser#sliceop.
    def enterSliceop(self, ctx:FizzParser.SliceopContext):
        pass

    # Exit a parse tree produced by FizzParser#sliceop.
    def exitSliceop(self, ctx:FizzParser.SliceopContext):
        pass


    # Enter a parse tree produced by FizzParser#comp_for.
    def enterComp_for(self, ctx:FizzParser.Comp_forContext):
        pass

    # Exit a parse tree produced by FizzParser#comp_for.
    def exitComp_for(self, ctx:FizzParser.Comp_forContext):
        pass


    # Enter a parse tree produced by FizzParser#comp_iter.
    def enterComp_iter(self, ctx:FizzParser.Comp_iterContext):
        pass

    # Exit a parse tree produced by FizzParser#comp_iter.
    def exitComp_iter(self, ctx:FizzParser.Comp_iterContext):
        pass



del FizzParser