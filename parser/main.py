
import re
import sys
from antlr4 import *
# from parser.FizzLexer import FizzLexer
# from parser.FizzParser import FizzParser
# from parser.FizzParserVisitor import FizzParserVisitor
# from antlr4.error.Errors import RecognitionException
# from antlr4.error.ErrorListener import ErrorListener
from proto.fizz_ast_pb2 import File
import proto.fizz_ast_pb2 as ast
# from parser.BuildAstVisitor import BuildAstVisitor
# from parser.ErrorListener import MyErrorListener
from parser.parser import writeJsonToFile, parse_file
from google.protobuf.json_format import MessageToJson
from pathlib import Path


def main(argv):
    if len(sys.argv) > 1:
        filename = sys.argv[1]
        with open(filename, 'r') as file:
            content = file.read()
    else:
        content = sys.stdin.read()
        filename = "stdin"

    answer = parse_file(filename, content)
    json_obj = MessageToJson(answer)
    print("json:\n", json_obj)
    writeJsonToFile(sys.argv[1], json_obj)



if __name__ == '__main__':
    main(sys.argv)

