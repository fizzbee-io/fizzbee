import argparse
import sys
import os

from mbt.generator.filenames import go_filenames, base_pascal_case, path_for_java_package, normalize_basename
from mbt.generator.filenames import rust_filenames
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
            choices=["go", "java", "rust"],  # Extend later with "typescript", python"
            required=True,
            help="Target language for code generation"
        )
        parser.add_argument(
            "--out-dir",
            default=None,  # Set dynamically based on lang
            help="Output directory for generated interface code "
                 "(default: src/fizztest/java for Java, fizztests for Go)"
        )
        parser.add_argument(
            "--go-package",
            default=None,
            help="Go package name for generated interface code (required if lang=go)"
        )
        parser.add_argument(
            "--java-package",
            default=None,
            help="Java package name for generated interface code (required if lang=java)"
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

        # Set defaults based on language
        if args.out_dir is None:
            if args.lang == "go":
                args.out_dir = "fizztests"
            elif args.lang == "java":
                args.out_dir = os.path.join("src", "fizztest", "java")
            elif args.lang == "rust":
                args.out_dir = "fizztests"

        # Validation
        if args.lang == "go":
            if not args.go_package:
                parser.error("--go-package is required when --lang=go")

        if args.lang == "java":
            if not args.java_package:
                parser.error("--java-package is required when --lang=java")

        if not args.input_spec.endswith(".fizz"):
            print("Warning: input_spec does not end with .fizz", file=sys.stderr)

        return args

def main(argv):
    args = parse_args()
    print("args: ", args)

    if args.lang not in ["go", "java", "rust"]:
        print(f"Unsupported language: {args.lang}", file=sys.stderr)
        sys.exit(1)

    current_dir = os.path.dirname(__file__)
    print("current_dir", current_dir)
    data_path = os.path.join(current_dir, "templates")
    print("data_path", data_path)

    filename = args.input_spec
    with open(filename, 'r') as file:
        content = file.read()

    parsedAst = parse_file(filename, content)
    print("Parsed AST:", parsedAst)

    if args.lang == "go":
        generate_go(args, filename, parsedAst, data_path)
    elif args.lang == "java":
        generate_java(args, filename, parsedAst, data_path)
    elif args.lang == "rust":
        generate_rust(args, filename, parsedAst, data_path)

def generate_go(args, filename, parsedAst, data_path):
    # Compute relative path to project root
    abs_spec_path = os.path.abspath(filename)
    abs_project_root = os.path.abspath(args.rel_root)
    try:
        rel_spec_path = os.path.relpath(abs_spec_path, abs_project_root)
    except ValueError:
        # Fallback if different drives on Windows
        rel_spec_path = abs_spec_path


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
            file=parsedAst,
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

def generate_rust(args, filename, parsedAst, data_path):
    # Compute relative path to project root
    abs_spec_path = os.path.abspath(filename)
    abs_project_root = os.path.abspath(args.rel_root)
    try:
        rel_spec_path = os.path.relpath(abs_spec_path, abs_project_root)
    except ValueError:
        # Fallback if different drives on Windows
        rel_spec_path = abs_spec_path

    # Compute base name for module folder (e.g., 'counter' for Counter.fizz)
    model_base_name = normalize_basename(filename)
    out_dir_model = os.path.join(args.out_dir, "")

    # Ensure output directory exists
    os.makedirs(out_dir_model, exist_ok=True)

    env = Environment(loader=FileSystemLoader(os.path.join(data_path, "rust"))) # Use 'rust' subdirectory in templates

    templates = [
        ("mod.rs.j2", "mod.rs", True, True),
        ("traits.rs.j2", "traits.rs", True, True),
        ("adapters.rs.j2", "adapters.rs", args.gen_adapter, False), # adapters are scaffolded only if requested
        ("test.rs.j2", "test.rs", True, True),
    ]

    for tpl_name, out_file, enabled, overwrite in templates:
        if not enabled:
            continue

        template = env.get_template(tpl_name)
        output = template.render(
            file=parsedAst,
            model_name=base_pascal_case(filename),
            model_base_name=model_base_name, # snake_case name for module paths
            source_path=rel_spec_path,
        )

        out_path = os.path.join(out_dir_model, out_file)

        if os.path.exists(out_path) and not overwrite:
            print(f"File {out_path} already exists. Delete the file or not use --gen-adapter to skip.", file=sys.stderr)
            sys.exit(1)

        with open(out_path, "w") as f:
            f.write(output)
        print(f"Generated {out_file} in {out_dir_model} from {tpl_name}")

def generate_java(args, filename, parsedAst, data_path):
    # Compute relative path to project root
    abs_spec_path = os.path.abspath(filename)
    abs_project_root = os.path.abspath(args.rel_root)
    try:
        rel_spec_path = os.path.relpath(abs_spec_path, abs_project_root)
    except ValueError:
        rel_spec_path = abs_spec_path

    # Compute output directory based on package name
    package_path = path_for_java_package(args.java_package)
    out_dir = os.path.join(args.out_dir, package_path)
    os.makedirs(out_dir, exist_ok=True)

    # Initialize Jinja2 environment for Java templates
    env = Environment(loader=FileSystemLoader(os.path.join(data_path, "java")))

    # ------------------------
    # 1. Generate model interface
    # ------------------------
    model_template = env.get_template("model_interface.java.j2")
    model_name = base_pascal_case(filename)
    model_output = model_template.render(
        package_name=args.java_package,
        file=parsedAst,
        model_name=model_name,
        source_path=rel_spec_path
    )
    model_file_path = os.path.join(out_dir, f"{model_name}Model.java")
    with open(model_file_path, "w") as f:
        f.write(model_output)
    print(f"Generated {model_file_path}")

    # ------------------------
    # 2. Generate test base class
    # ------------------------
    test_base_template = env.get_template("test_base.java.j2")
    test_base_output = test_base_template.render(
        package_name=args.java_package,
        file=parsedAst,
        model_name=model_name,
        source_path=rel_spec_path
    )
    test_base_file_path = os.path.join(out_dir, f"{model_name}TestBase.java")
    with open(test_base_file_path, "w") as f:
        f.write(test_base_output)
    print(f"Generated {test_base_file_path}")

    # ------------------------
    # 3. Generate one role interface per role
    # ------------------------
    role_template = env.get_template("role_interface.java.j2")
    for role in parsedAst.roles:
        role_name = role.name
        role_output = role_template.render(
            package_name=args.java_package,
            role=role,
            model_name=model_name,
            source_path=rel_spec_path
        )
        role_file_path = os.path.join(out_dir, f"{role_name}Role.java")
        with open(role_file_path, "w") as f:
            f.write(role_output)
        print(f"Generated {role_file_path}")


    # ------------------------
    # 4. Generate adapter scaffolding (only if --gen-adapter)
    # ------------------------
    if args.gen_adapter:
        adapter_files = []

        model_adapter_path = os.path.join(out_dir, f"{model_name}ModelAdapter.java")
        test_impl_path = os.path.join(out_dir, f"{model_name}Test.java")
        role_adapter_paths = [
            os.path.join(out_dir, f"{role.name}RoleAdapter.java")
            for role in parsedAst.roles
        ]

        adapter_files.extend([model_adapter_path, test_impl_path])
        adapter_files.extend(role_adapter_paths)

        # Check if any adapter files already exist
        existing_files = [f for f in adapter_files if os.path.exists(f)]
        if existing_files:
            print("The following adapter files already exist:", file=sys.stderr)
            for f in existing_files:
                print(f"  {f}", file=sys.stderr)
            print(
                "Delete the files above or do not use --gen-adapter to skip adapter generation.",
                file=sys.stderr
            )
            sys.exit(1)

        # Generate model adapter
        model_adapter_template = env.get_template("model_adapter.java.j2")
        model_adapter_output = model_adapter_template.render(
            package_name=args.java_package,
            file=parsedAst,
            model_name=model_name,
            source_path=rel_spec_path
        )
        with open(model_adapter_path, "w") as f:
            f.write(model_adapter_output)
        print(f"Generated {model_adapter_path}")

        # Generate role adapters
        role_adapter_template = env.get_template("role_adapter.java.j2")
        for role in parsedAst.roles:
            role_adapter_output = role_adapter_template.render(
                package_name=args.java_package,
                role=role,
                model_name=model_name,
                source_path=rel_spec_path
            )
            role_adapter_path = os.path.join(out_dir, f"{role.name}RoleAdapter.java")
            with open(role_adapter_path, "w") as f:
                f.write(role_adapter_output)
            print(f"Generated {role_adapter_path}")

        # Generate test implementation
        test_impl_template = env.get_template("test_impl.java.j2")
        test_impl_output = test_impl_template.render(
            package_name=args.java_package,
            model_name=model_name,
            source_path=rel_spec_path
        )
        with open(test_impl_path, "w") as f:
            f.write(test_impl_output)
        print(f"Generated {test_impl_path}")

if __name__ == '__main__':
    main(sys.argv)
