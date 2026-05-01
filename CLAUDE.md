# Claude Code Instructions for Fizzbee

## Build Commands

- **Build the model checker**: `bazel build //:fizzbee`
- **Run the model checker**: `./fizz path/to/spec/file.fizz` (use the wrapper script, not the bazel binary directly)

## Important Guidelines

- **Do NOT use gazelle** to update BUILD files
- The `./fizz` wrapper script handles compilation and execution - always use it instead of `./bazel-bin/fizzbee_/fizzbee`

## Testing

- Reference examples are in `examples/references/` organized by feature
- Many examples have `baseline.txt` files for comparing output metrics (Nodes, Valid Nodes, Unique States, PASSED/FAILED status)
- Generated files (`.json` AST files and `out/` directories) can be deleted after testing

### Running the test suite

```bash
python3 test_references.py
```

Runs all 94 reference examples (those with a `baseline.txt`) and all lovable specs in `examples/requirements/lovable/`. Compares output semantically (strips timestamps, elapsed times, file paths). Reports PASS/FAIL per example and a summary.

To update a baseline after an intentional change:
```bash
./fizz examples/references/NN-name/Spec.fizz > examples/references/NN-name/baseline.txt 2>&1
```

## Project Structure

- `lib/` - Core library code (starlark types, symmetry, JSON marshalling)
- `modelchecker/` - Model checking engine (processor, threads, cloning, state visitor)
- `parser/` - Fizz language parser
- `proto/` - Protocol buffer definitions
- `examples/` - Example specifications
  - `references/` - Reference examples organized by feature (01-xx through 16-xx)
  - `tutorials/` - To be removed. Old tutorial specifications, some of which may be outdated and may not even work
- `samples/` - Larger sample specifications. Again, not organized and may be outdated and may not even work, but kept for reference. These are not merged into git.
