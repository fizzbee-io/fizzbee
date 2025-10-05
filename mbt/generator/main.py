import argparse
import sys
import os

from mbt.generator.filenames import go_filenames, base_pascal_case
from parser.parser import parse_file
from jinja2 import Environment, FileSystemLoader

def parse_args():
        parser = argparse.ArgumentParser(
            description="FizzBee Model Based Testing code generator"
        )
        parser.add_argument(
            "input_spec",
            help="Path to the fizzbee specification file (.fizz)"
        )
        parser.add_argument(
            "-l", "--lang",
            choices=["go"],  # Extend later with "java", "rust", "python"
            required=True,
            help="Target language for code generation"
        )
        parser.add_argument(
            "--out-dir",
            default="fizztests",
            help="Output directory for generated interface code (default: out)"
        )
        parser.add_argument(
            "--go-package",
            default=None,
            help="Go package name for generated interface code (required if lang=go)"
        )
        parser.add_argument(
            "--gen-adapter",
            default=False,
            action='store_true',
            help="Generate the adapter and the test runner code (default: False, only interfaces are generated)"
        )
        parser.add_argument(
            "--rel-root",
            default=None,
            help="Base directory to compute relative paths for spec references (default: same as --out-dir)"
        )

        args = parser.parse_args()
        if args.rel_root is None:
            args.rel_root = args.out_dir

        # Validation
        if args.lang == "go":
            if not args.go_package:
                parser.error("--go-package is required when --lang=go")

        if not args.input_spec.endswith(".fizz"):
            print("Warning: input_spec does not end with .fizz", file=sys.stderr)

        return args

def main(argv):
    args = parse_args()
    print("args: ", args)

    if args.lang not in ["go"]:
        print(f"Unsupported language: {args.lang}", file=sys.stderr)
        sys.exit(1)

    current_dir = os.path.dirname(__file__)
    print("current_dir", current_dir)
    data_path = os.path.join(current_dir, "templates")
    print("data_path", data_path)

    filename = args.input_spec
    with open(filename, 'r') as file:
        content = file.read()

    # Compute relative path to project root
    abs_spec_path = os.path.abspath(filename)
    abs_project_root = os.path.abspath(args.rel_root)
    try:
        rel_spec_path = os.path.relpath(abs_spec_path, abs_project_root)
    except ValueError:
        # Fallback if different drives on Windows
        rel_spec_path = abs_spec_path

    answer = parse_file(filename, content)
    print("Parsed AST:", answer)

    # Ensure output directory exists
    os.makedirs(args.out_dir, exist_ok=True)

    env = Environment(loader=FileSystemLoader(data_path))

    templates = [
        ("go/interfaces.go.j2", "_interfaces", True, True),
        ("go/adapters.go.j2", "_adapters", args.gen_adapter, False),
        ("go/test.go.j2", "_test", True, True),
    ]

    for tpl_name, out_file_suffix, enabled, overwrite in templates:
        if not enabled:
            continue
        template = env.get_template(tpl_name)
        output = template.render(
            package_name=args.go_package,
            file=answer,
            model_name=base_pascal_case(filename),
            source_path=rel_spec_path,
        )
        out_file = go_filenames(filename, [out_file_suffix])[0]
        out_path = os.path.join(args.out_dir, out_file)
        if os.path.exists(out_path) and not overwrite:
            print(f"File {out_path} already exists. Delete the file or not use --gen-adapter to skip.", file=sys.stderr)
            sys.exit(1)
        with open(out_path, "w") as f:
            f.write(output)
        print(f"Generated {out_file} from {tpl_name}")


if __name__ == '__main__':
    main(sys.argv)
