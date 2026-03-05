---
description: >
  Write, edit, or review FizzBee (.fizz) specifications for model checking
  distributed systems. Use when the user asks to model a system, write a
  formal spec, define invariants or liveness properties, or when working
  with a .fizz file.
---

# FizzBee Spec Writing

FizzBee is a Python-like formal spec language built on Starlark. Specs model distributed systems; the model checker exhaustively verifies safety and liveness properties.

**Full reference**: `~/.claude/skills/fizzbee-docs/LANGUAGE_REFERENCE.md`
**Examples index**: `~/.claude/skills/fizzbee-docs/README.md`

## Installation

```bash
brew tap fizzbee-io/fizzbee && brew install fizzbee
# Then run: fizz spec.fizz
```

---

## File Structure

```yaml
---
# Optional YAML frontmatter
options:
  max_actions: 100
  max_concurrent_actions: 2   # 1 for single-user apps
  crash_on_yield: true        # enable fault injection

action_options:
  "RoleName.ActionName":
    max_actions: 10

deadlock_detection: true      # false for interactive apps
liveness: strict
---
```

Then the Fizz spec:
1. Top-level constants
2. Functions (pure helpers)
3. Roles (actors/processes)
4. Top-level actions (Init + others)
5. Assertions

---

## Core Syntax

### Constants and Global State

```python
MAX = 10                        # constant
State = enum('IDLE', 'RUNNING') # enum

action Init:
    counter = 0      # becomes global state variable
    items = []
    server = Server()  # create role instance
```

### Actions

```python
# Modifier order: [flow] [fairness] action Name
atomic action Increment:        # atomic = no interleaving
    counter = counter + 1

fair action Progress:           # fair = weak fairness
    require not done
    done = True

atomic fair action Produce:     # both modifiers
    require len(queue) < MAX
    queue.append(item)
```

**Key rules:**
- No parameters on actions
- `Init` runs once; all variables defined in it become global state
- `require condition` disables the action (doesn't execute it)
- An action is "enabled" only after it executes a simple statement (`=`, method call, `pass`)

### Roles

```python
@state(ephemeral=['cache'])     # cache resets on crash
role Server:
    action Init:
        self.data = {}          # role state (persists)
        self.cache = {}         # ephemeral (resets on crash)

    atomic func Read(key):      # function: can have params + return
        return self.data.get(key, None)

    action Write:
        key = any self.pending_keys
        self.data[key] = self.pending[key]
```

**Key rules:**
- `self.*` = role state; local vars are not part of state
- `self.__id__` = auto-generated unique ID (read-only)
- Functions can only be called from atomic context or inside roles
- `result = role.fn()` works; `self.x = role.fn()` crashes — use a local var first

### Flow Modifiers

```python
atomic action AllAtOnce:        # one indivisible step
    x = 1; y = 2

action SerialSteps:             # yields between statements (default)
    x = 1                       # yield point here
    y = 2                       # yield point here

action Concurrent:
    parallel:                   # explores all interleavings
        x = x + 1
        y = y + 1

action Choose:
    oneof:                      # nondeterministically pick one branch
        status = "success"
        status = "failure"
```

### Nondeterminism

```python
x = any [1, 2, 3]              # pick any element
x = any range(10)              # pick any integer 0..9
x = any items                  # pick any element from collection
# Note: any on empty collection disables the action
```

### Assertions

```python
always assertion SafetyProp:           # must hold in ALL states
    return counter >= 0

exists assertion Reachable:            # holds in AT LEAST ONE state
    return counter == MAX

transition assertion Monotonic(before, after):  # on every transition
    if after.counter != before.counter:
        return after.counter > before.counter
    return True   # must allow stutter steps (no-change transitions)

eventually always assertion Stabilizes:  # eventually stable (needs fairness)
    return done == True

always eventually assertion Progresses: # always makes progress (needs strong fairness)
    return processed > 0
```

### Symmetry Reduction

Dramatically reduces state space for interchangeable values/actors:

```python
# Unique IDs (nominal = only identity matters)
IDS = symmetry.nominal(name="id", limit=3)   # max 3 live at once
id = IDS.fresh()

# Interchangeable pool (e.g., task content)
TEXTS = symmetry.nominal(name="task", limit=3)
v = any TEXTS.choices()   # grows naturally: 1 option first, then more

# Symmetric roles (N! reduction for N instances)
symmetric role Worker:
    action Init:
        self.status = "idle"

workers = bag()
workers.add(Worker())   # use bag(), not list, for symmetric roles
```

**Symmetry limits**: set to max *coexisting* values, not total ever created.

---

## Data Types

| Type | Use | Example |
|---|---|---|
| list | ordered, mutable | `items = []`, `items.append(x)` |
| set | unordered, unique, hashable | `s = set()`, `s.add(x)` |
| dict | key-value, hashable keys | `d = {}`, `d[k] = v` |
| bag | multiset | `b = bag()`, `b.add(x)` |
| record | mutable named struct | `r = record(x=1, y=2)` |
| struct | immutable named struct | `s = struct(x=1)` |
| enum | named constants | `E = enum('A','B')`, `E.A` |
| genericset/genericmap | non-hashable elements (local scope only!) | `gs = genericset()` |

---

## Fault Injection

```yaml
---
options:
  crash_on_yield: true   # roles can crash at yield points
---
```

```python
@state(ephemeral=['cache'])   # cache lost on crash, data persists
role Node:
    action Init:
        self.data = 0    # durable
        self.cache = 0   # ephemeral
```

Cross-role function calls can also be lost (message loss simulation) — model checker explores both delivery and loss.

---

## Gotchas

1. **Cross-role return**: `self.x = role.fn()` crashes → use `tmp = role.fn(); self.x = tmp`
2. **No tuple unpacking in for**: `for k, v in d.items()` fails → `for k in d: v = d[k]`
3. **Role Init ordering**: create globals before creating roles that depend on them
4. **`require` ≠ assertion**: `require x >= 0` disables action; use `always assertion` for invariants
5. **Lists break symmetry**: use `bag()` not `[]` to hold symmetric role instances
6. **`any` on empty**: disables the action (no `require len > 0` needed)
7. **`any` keyword vs Python `any()` function**: `x = any([cond for ...])` is a parse error — `any` is the nondeterministic choice keyword. For assignments use `all([...])` or `len([x for x in xs if cond]) > 0`. In guards, `require all([not cond for ...])` is the idiomatic form.
8. **Function calls**: can only call fizz functions from atomic context or inside roles; extract to variable before using in expressions
9. **`None` works**: use `== None`, not `is None` (no `is` operator)

---

## Common Patterns

### Server + Client roles

```python
role Server:
    action Init:
        self.store = {}

    atomic func Write(key, value):
        self.store[key] = value

    atomic func Read(key):
        return self.store.get(key, None)

role Client:
    action Init:
        self.result = None

    action DoRead:
        v = server.Read("key")   # local var first!
        self.result = v

action Init:
    server = Server()
    client = Client()
```

### Message queue

```python
action Init:
    queue = []

atomic fair action Send:
    require len(queue) < MAX
    queue.append(record(id=next_id, body="msg"))

atomic fair action Receive:
    require len(queue) > 0
    msg = queue.pop(0)
    processed = processed + [msg.id]
```

### Dynamic roles (hire/fire)

```python
symmetric role Employee:
    action Init:
        self.active = True

action Init:
    employees = bag()

action Hire:
    employees.add(Employee())

action Fire:
    target = any [e for e in employees if e.active]
    employees = bag([e for e in employees if e.__id__ != target.__id__])
```

---

## Reference Examples (by topic)

Installed example specs (after `fizz install-skills`):
- `~/.claude/skills/fizzbee-docs/examples/01-counter.fizz` — basics: init, atomic action, assertion
- `~/.claude/skills/fizzbee-docs/examples/09-assertions.fizz` — always/exists/transition assertions
- `~/.claude/skills/fizzbee-docs/examples/11-roles.fizz` — two-role communication
- `~/.claude/skills/fizzbee-docs/examples/13-two-phase-commit.fizz` — distributed protocol
- `~/.claude/skills/fizzbee-docs/examples/14-fault-injection.fizz` — crash-on-yield, ephemeral state
- `~/.claude/skills/fizzbee-docs/examples/16-symmetry.fizz` — nominal symmetry, symmetric roles

Full examples index: `~/.claude/skills/fizzbee-docs/README.md`
All 100+ examples on GitHub: https://github.com/fizzbee-io/fizzbee/tree/main/examples/references
