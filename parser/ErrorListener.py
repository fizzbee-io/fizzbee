
from antlr4.error.ErrorListener import ErrorListener

class MyErrorListener( ErrorListener ):

    def __init__(self):
        super(MyErrorListener, self).__init__()

    def syntaxError(self, recognizer, offendingSymbol, line, column, msg, e):
        # print(f"Error: {line}:{column} Unexpected: {FizzParser.symbolicNames[offendingSymbol.type]} {msg}", file=sys.stderr)
        # # print("offendingSymbol", offendingSymbol)
        # # print("offendingSymbol type", type(offendingSymbol))
        # # print("offendingSymbol text", offendingSymbol.text)
        # # print("offendingSymbol type", offendingSymbol.type)
        # #
        # # print("line", line)
        # # print("column", column)
        # # print("msg", msg)
        # # print("syntaxError", e)
        # raise e
        # raise Exception("Oh no!!")
        if e is not None:
            raise e
        pass

    def reportAmbiguity(self, recognizer, dfa, startIndex, stopIndex, exact, ambigAlts, configs):
        print('reportAmbiguity', startIndex, stopIndex, exact, ambigAlts, configs)
        # raise Exception("Oh no!!")

    def reportAttemptingFullContext(self, recognizer, dfa, startIndex, stopIndex, conflictingAlts, configs):
        print('reportAttemptingFullContext', startIndex, stopIndex, conflictingAlts, configs)
        # raise Exception("Oh no!!", startIndex, stopIndex, conflictingAlts, configs)

    def reportContextSensitivity(self, recognizer, dfa, startIndex, stopIndex, prediction, configs):
        raise Exception("reportContextSensitivity!!")
