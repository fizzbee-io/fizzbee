import sys
from antlr4 import *
from parser.FizzLexer import FizzLexer
from parser.FizzParser import FizzParser
from parser.FizzParserVisitor import FizzParserVisitor
from antlr4.error.Errors import RecognitionException
from antlr4.error.ErrorListener import ErrorListener
from proto.fizz_ast_pb2 import File
from parser.BuildAstVisitor import BuildAstVisitor
from google.protobuf.json_format import MessageToJson
from pathlib import Path

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


def main(argv):
    if len(sys.argv) > 1:
        stream = FileStream(sys.argv[1])
    else:
        stream = InputStream(sys.stdin.readline())
    lexer = FizzLexer(stream)
    tokens = CommonTokenStream(lexer)
#    tokens.fill()
    parser = FizzParser(tokens)
    parser.addErrorListener( MyErrorListener() )
    print('calling parser.root()')
    try:
        tree = parser.root()
    except RecognitionException as e:
        exit(1)


    print('calling BuildAstVisitor() dir', dir(BuildAstVisitor(stream)))
    answer = BuildAstVisitor(stream).visit(tree)
    print("proto:\n", answer)
    json_obj = MessageToJson(answer)
    print("json:\n", json_obj)
    writeJsonToFile(sys.argv[1], json_obj)
#    for token in tokens.getTokens(0, 100):
#      print(token)

    print(tree.getChildCount())
    i = 0
    for child in tree.getChildren():
      print(i, dir(child))
      if hasattr(child, 'toStringTree'):
        print(child.toStringTree(recog=parser))
        # print(child.toStringTree())
        print(child.getRuleIndex())
        print(child.getRuleContext())
        print(child.getPayload())
      else:
        print(child.getSymbol())
      i += 1

    print(tree.toStringTree(recog=parser))
#    for token in tokens.getTokens(0, 100):
#      print(token)


def writeJsonToFile(input_filename, jsondata):
    # Use pathlib to manipulate the path and change the extension
    input_path = Path(input_filename)
    output_path = input_path.with_suffix(".json")

    # Write jsondata to the new file
    with output_path.open('w') as json_file:
        json_file.write(jsondata)

if __name__ == '__main__':
    main(sys.argv)

