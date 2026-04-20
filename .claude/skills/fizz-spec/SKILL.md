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
        key = oneof self.pending_keys
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
x = oneof [1, 2, 3]            # pick one element (nondeterministic)
x = oneof range(10)            # pick one integer 0..9
x = oneof items                # pick one element from collection
# Note: oneof on empty collection disables the action

oneof x in items:              # for-each style: pick one x, execute body
    process(x)

# `any` is a deprecated alias for `oneof` (emits DeprecationWarning)
x = any items                  # old form — still works but deprecated
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
v = oneof TEXTS.choices()   # grows naturally: 1 option first, then more

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
6. **`oneof`/`any` on empty**: disables the action (no `require len > 0` needed)
7. **`any` keyword vs Python `any()` function**: `x = any([cond for ...])` is a parse error — `any` is the deprecated nondeterministic choice keyword. Prefer `oneof` (no collision: `oneof` can't be an identifier). For boolean checks use `all([...])` or `len([x for x in xs if cond]) > 0`.
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
    target = oneof [e for e in employees if e.active]
    employees = bag([e for e in employees if e.__id__ != target.__id__])
```

---

## Modeling E2E Behavior and Requirements

Use this pattern when modeling user-facing apps, UI flows, or product requirements — not just distributed protocols. The spec becomes both a formal model and a contract for model-based testing (MBT).

### Role Design

| App type | Roles |
|---|---|
| Single-user local app (TodoMVC) | `role App` — one role for everything |
| Client-server or localStorage | `role System` + `role User` |
| Marketplace | `role System` + `role Seller` + `role Buyer` |
| Multi-role service | `role System` + one role per user type |

**`System`** holds the authoritative application state (what the DB / localStorage / server knows). It is implementation-agnostic — the same spec works whether state lives in localStorage, a REST API, or a database.

**User roles** hold what the user sees and controls: their current view, selections, form inputs. Each user type gets its own role.

**Single `App` role** is fine for purely local apps with no meaningful gap between storage and UI state (e.g., TodoMVC where shown_todos is always derived from the store).

### Front matter for interactive apps

```yaml
---
liveness: "false"          # no liveness assertions needed for UI specs
deadlock_detection: false  # interactive apps don't deadlock
options:
  max_concurrent_actions: 1  # one user at a time
---
```

### MBT-ready spec conventions

Four conventions make a spec directly usable for model-based testing:

**1. Explicit view state** — store what the user sees as a separate field, not recomputed on the fly:

```python
role System:
    action Init:
        self.todos = {}        # authoritative store (localStorage / DB)
        self.shown_todos = []  # derived view — maps to visible DOM items

    atomic func refresh():
        self.shown_todos = [self.todos[id] for id in self.todos]
```

**2. Index-based picks** — actions that select an item use an index into the view list, not an ID. This matches how the adapter calls the action (args[0] = index into rendered list):

```python
atomic action DeleteTodo:
    require len(self.shown_todos) > 0
    idx = oneof range(len(self.shown_todos))   # args[0] in adapter
    t = self.shown_todos[idx]
    self.todos.pop(t.id)
    self.refresh()
```

**3. IDs for generated records** — if the spec creates records with opaque IDs (todos, tickets, bookings), prefer symmetry values over integer counters. This keeps the state space compact without sacrificing correctness:
- `symmetry.nominal` — only identity matters, no ordering (UUID v4-style IDs)
- `symmetry.ordinal` — relative order matters but not distance (UUID v7-style IDs, insertion order)
- `symmetry.interval` — numeric distance matters (sequence numbers, timestamps, version counters)

```python
IDS = symmetry.nominal(name="id", limit=MAX_ITEMS)  # limit = max coexisting IDs
id = IDS.fresh()                                     # allocate a new unique ID
```

**4. Named input constants** — collect all user-supplied strings into top-level constants so the adapter's `OverridesProvider` can replace them with fuzz-generated values:

```python
TODO_STRINGS = ("Buy milk", "Walk dog", "Write spec")   # overridable
WHITESPACE_STRINGS = (" ", "")                           # leave as-is

atomic action AddTodo:
    raw = oneof TODO_STRINGS + WHITESPACE_STRINGS
    title = raw.strip()
    ...
```

### Full example skeleton

```python
---
liveness: "false"
deadlock_detection: false
options:
  max_concurrent_actions: 1
---

MAX_ITEMS = 3
IDS = symmetry.nominal(name="id", limit=MAX_ITEMS)
INPUT_STRINGS = ("hello", "world", "foo")

role System:
    action Init:
        self.items = {}        # id → record
        self.view = []         # filtered/sorted view shown to user

    atomic func refresh():
        self.view = [self.items[id] for id in self.items]

role User:
    action Init:
        self.selected = None

    atomic action AddItem:
        require len(system.items) < MAX_ITEMS
        raw = oneof INPUT_STRINGS
        id = IDS.fresh()
        system.items[id] = record(id=id, text=raw)
        system.refresh()

    atomic action SelectItem:
        require len(system.view) > 0
        idx = oneof range(len(system.view))   # adapter: args[0]
        self.selected = system.view[idx].id

action Init:
    system = System()
    user = User()

always assertion ItemsBounded:
    return len(system.items) <= MAX_ITEMS
```

Once the spec is ready, use the **`fizz-mbt` skill** to generate adapter code (TypeScript/Playwright, Go, Rust, or Java) that connects the spec to the real system under test.

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
