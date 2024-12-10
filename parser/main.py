
import re
import sys
from antlr4 import *
from parser.FizzLexer import FizzLexer
from parser.FizzParser import FizzParser
from parser.FizzParserVisitor import FizzParserVisitor
from antlr4.error.Errors import RecognitionException
from antlr4.error.ErrorListener import ErrorListener
from proto.fizz_ast_pb2 import File
import proto.fizz_ast_pb2 as ast
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
        filename = sys.argv[1]
        with open(filename, 'r') as file:
            content = file.read()
    else:
        content = sys.stdin.read()
        filename = "stdin"

    initial_spaces, yaml_frontmatter, content_without_frontmatter = extract_yaml_frontmatter(content)
    # Output or store the YAML frontmatter as needed
    print("YAML Frontmatter:", len(yaml_frontmatter.splitlines()))
    print(yaml_frontmatter)
    print("FizzBee code:", len(content_without_frontmatter.splitlines()))
    print(content_without_frontmatter)
    if initial_spaces == '' and yaml_frontmatter == '':
        print("No YAML frontmatter found")
        num_lines = 0
    else:
        num_lines = len(initial_spaces.splitlines()) + len(yaml_frontmatter.splitlines()) + 2
    # prefix content_without_frontmatter with the empty new lines so the line numbers match
    content_without_frontmatter = "\n" * num_lines + content_without_frontmatter
    yaml_frontmatter = "\n" * len(initial_spaces.splitlines()) + yaml_frontmatter
    # Continue with ANTLR parsing
    stream = InputStream(content_without_frontmatter)
    lexer = FizzLexer(stream)
    tokens = CommonTokenStream(lexer)

    # if len(sys.argv) > 1:
    #     filename = sys.argv[1]
    #     stream = FileStream(filename)
    # else:
    #     stream = InputStream(sys.stdin.readline())
    #     filename = "stdin"
    # lexer = FizzLexer(stream)
    # tokens = CommonTokenStream(lexer)
#    tokens.fill()
    parser = FizzParser(tokens)
    parser.addErrorListener( MyErrorListener() )
    print('calling parser.root()')
    try:
        tree = parser.root()
    except RecognitionException as e:
        exit(1)

    print('calling BuildAstVisitor() dir', dir(BuildAstVisitor(stream, file_path=filename)))
    answer = BuildAstVisitor(stream, file_path=filename).visit(tree)
    print("proto:\n", answer)
    # answer.front_matter = ast.FrontMatter(yaml=yaml_frontmatter)
    answer.front_matter.yaml = yaml_frontmatter
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


# Define a function to extract YAML frontmatter
def extract_yaml_frontmatter(text):
    # Regex pattern to capture the frontmatter
    yaml_frontmatter_pattern = re.compile(r'^(\s*)---[^\n]*\n([\s\S]*?)---[^\n]*\n', re.MULTILINE)

    match = yaml_frontmatter_pattern.search(text)
    if match:
        initial_text = match.group(1)
        yaml_frontmatter = match.group(2)  # Add newline to preserve format
        remaining_text = text[match.end():]  # Keep the remaining text after the frontmatter

        return initial_text, yaml_frontmatter, remaining_text
    return '', '', text


if __name__ == '__main__':
    main(sys.argv)

