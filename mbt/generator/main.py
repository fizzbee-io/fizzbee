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
            "--project-root",
            default=os.getcwd(),
            help="Project root to make source file paths relative (default: current working directory)"
        )

        args = parser.parse_args()

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
    abs_project_root = os.path.abspath(args.project_root)
    try:
        rel_spec_path = os.path.relpath(abs_spec_path, abs_project_root)
    except ValueError:
        # Fallback if different drives on Windows
        rel_spec_path = abs_spec_path

    answer = parse_file(filename, content)
    print("Parsed AST:", answer)

    # Ensure output directory exists
    os.makedirs(args.out_dir, exist_ok=True)
    # Generate Go file names based on the input filename
    iface_file, adapter_file = go_filenames(filename, ["_interfaces", "_adapters"])

    env = Environment(loader=FileSystemLoader(data_path))
    template = env.get_template("go/interfaces.go.j2")

    interface_output = template.render(
        package_name=args.go_package,
        file=answer,  # your parsed File proto instance
        model_name=base_pascal_case(filename),  # Convert filename to PascalCase for Go
        source_path=rel_spec_path,
    )
    # Write interfaces.go
    iface_path = os.path.join(args.out_dir, iface_file)
    with open(iface_path, "w") as f:
        f.write(interface_output)
    print(f"Generated Go interface file: {iface_path}")

    if args.gen_adapter:
        adapter_template = env.get_template("go/adapters.go.j2")
        adapter_output = adapter_template.render(
            package_name=args.go_package,
            file=answer,
            model_name=base_pascal_case(filename),  # Convert filename to PascalCase for Go
            source_path=rel_spec_path,
        )
        # Write adapter.go
        adapter_path = os.path.join(args.out_dir, adapter_file)
        with open(adapter_path, "w") as f:
            f.write(adapter_output)
        print(f"Generated Go adapter file: {adapter_path}")



if __name__ == '__main__':
    main(sys.argv)
