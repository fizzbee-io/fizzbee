---
description: >
  Run the FizzBee model checker or simulator on a .fizz spec. Use when the
  user wants to verify a spec, run the model checker, simulate behavior,
  check a guided trace, or interpret model checker output.
---

# FizzBee Model Checker

## Installation

```bash
brew tap fizzbee-io/fizzbee && brew install fizzbee
```

## Running

```bash
fizz spec.fizz                       # full model checking (exhaustive)
fizz -x --max_runs 1 --seed 42 spec.fizz   # simulation (single random path)
```

Note: `fizz` is the installed binary. If building from source, use `./fizz` (wrapper script in the repo root).

---

## All Flags

| Flag | Default | Description |
|---|---|---|
| `-x` / `--simulation` | off | Simulation mode: single random path, fast |
| `--seed N` | 0 | Random seed for reproducible simulation |
| `--max_runs N` | 0 (unlimited) | Number of simulation runs |
| `--exploration_strategy` | `bfs` | State exploration: `bfs`, `dfs`, or `random` |
| `--trace "line1\nline2"` | — | Guided trace: follow specific action sequence |
| `--trace-file FILE` | — | Load guided trace from file |
| `--trace-extend N` | 0 | After trace, explore N more steps (shows enabled actions) |
| `--preinit-hook "STMT"` | — | Override constants before Init runs |
| `--preinit-hook-file FILE` | — | Load preinit hook from `.cfg` file |
| `--output-dir DIR` | auto-timestamped | Where to write output |
| `--no-copy-ast` | off | Don't copy AST to output dir |

---

## Understanding Output

```
Model checking specs/counter.json
StateSpaceOptions: options:{max_actions:100 max_concurrent_actions:2}
Nodes: 12, queued: 0, elapsed: 2.1ms
Valid Nodes: 12  Unique states: 8
IsLive: true
PASSED: Model checker completed successfully
```

- **Nodes**: total graph nodes explored
- **Valid Nodes**: nodes that passed all safety assertions
- **Unique states**: distinct system states (after symmetry reduction)
- **IsLive**: liveness assertions passed
- **PASSED / FAILED**: overall result

Output files go to `<spec-dir>/out/run_<timestamp>/`:
- `graph.dot` — state graph (auto-generated for < 250 nodes)
- `communication.dot` — role interaction diagram
- Node/link JSON files for the explorer

Convert to SVG for visual inspection:
```bash
dot -Tsvg out/*/graph.dot -o graph.svg && open graph.svg
```

---

## Simulation Mode

Good for: sanity-checking, exploring behavior, testing at realistic scale.

```bash
fizz -x --max_runs 1 --seed 42 spec.fizz
```

Extract the transition sequence from the output:
```bash
grep -o 'label="[^"]*"' out/*/graph.dot | sed 's/.*label="//;s/"//'
```

Try several seeds to find interesting behaviors:
```bash
for seed in 1 2 3 7 13 42; do
    fizz -x --max_runs 1 --seed $seed spec.fizz
done
```

---

## Guided Traces

Force a specific action sequence to test a scenario:

```bash
fizz --trace "Server.Write
Any:key=1
Client.Read" spec.fizz
```

**Trace format rules:**
- One transition per line
- `#` comments and blank lines are ignored
- Init is automatic — don't include it
- The `[fn.call]` suffix from simulation output can be omitted
- `Any:varname=value` lines specify nondeterministic choices

**Explore what comes next with `--trace-extend`:**
```bash
fizz --trace "Server.Write
Any:key=1" --trace-extend 2 spec.fizz
# Shows all enabled actions 2 steps beyond the trace
```

**Use a file for longer traces:**
```bash
# mytrace.txt
# Step 1: acquire lock
Node#0.RequestLock
# Step 2: do work
Node#0.DoWork
# Step 3: release
Node#0.ReleaseLock

fizz --trace-file mytrace.txt spec.fizz
```

**Incomplete trace warning:** If you see `WARNING: Trace execution incomplete`, a `require` guard blocked execution. This is often a bug in the spec.

---

## Configuration Overrides

### Spec options (deadlock_detection, max_concurrent_actions, etc.)

Checker options go in YAML frontmatter inside the `.fizz` file or in a separate `fizz.yaml`. Prefer frontmatter to keep config and spec together, unless sharing config across multiple specs:

```
---
deadlock_detection: false
options:
  max_concurrent_actions: 1
  crash_on_yield: true
---
# rest of the spec...
```

### Constant overrides (preinit-hook)

Override constants without editing the spec — useful for testing different configurations:

```bash
# Inline override
fizz --preinit-hook "MAX_ITEMS = 3
NUM_CLIENTS = 2" spec.fizz

# From file (.cfg extension — NOT .star, which auto-imports as a module)
fizz --preinit-hook-file small.cfg spec.fizz
```

All builtins available in the hook: `symmetry`, `record`, `enum`, `bag`, `math`, etc.

**Strategy: use small configs for model checking, large configs for simulation.**

```bash
# Model check with minimal config (isolate each concern)
fizz --preinit-hook "NUM_CLIENTS=2
NUM_SERVERS=1" spec.fizz

# Simulate with realistic config (no state explosion)
fizz -x --max_runs 1 --seed 42 --preinit-hook "NUM_CLIENTS=100
NUM_SERVERS=10" spec.fizz
```

---

## Common Workflows

### Verify a new spec

```bash
# 1. Quick simulation sanity check
fizz -x --max_runs 1 --seed 42 spec.fizz

# 2. Check key scenarios with guided traces
fizz --trace "..." spec.fizz

# 3. Full model check with small config
fizz --preinit-hook "N=2" spec.fizz

# 4. If FAILED: reduce config and visualize
fizz --preinit-hook "N=1" spec.fizz
dot -Tsvg out/*/graph.dot -o graph.svg && open graph.svg
```

### Baseline comparison

Reference examples include `baseline.txt` files with expected state counts.
Compare your output's `Nodes`, `Valid Nodes`, `Unique states` against the baseline.

---

## Verification Checklist

1. Simulate: `fizz -x --max_runs 1 --seed N spec.fizz` — try 3-5 seeds
2. Guided traces for key scenarios (happy path, race conditions, edge cases)
3. Config variants: small configs for model checking, large for simulation
4. Full model check: verify PASSED + state counts make sense
5. If < 250 nodes: visualize with `dot -Tsvg`

**Full guide**: `~/.claude/skills/fizzbee-docs/VERIFICATION_GUIDE.md`
