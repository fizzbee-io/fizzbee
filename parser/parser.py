import re
from pathlib import Path
from antlr4 import *
from parser.FizzLexer import FizzLexer
from parser.FizzParser import FizzParser
from parser.BuildAstVisitor import BuildAstVisitor
from parser.ErrorListener import MyErrorListener

def parse_file(filename, content):
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

    return answer

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
