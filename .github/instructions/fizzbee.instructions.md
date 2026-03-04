---
applyTo: "**/*.fizz"
---

# FizzBee Specification Language

FizzBee is a Python-like formal spec language (built on Starlark) for modeling distributed systems. The model checker exhaustively verifies safety and liveness properties.

**Full reference**: `examples/references/LANGUAGE_REFERENCE.md`
**Examples index**: `examples/references/README.md`
**Gotchas**: `examples/references/GOTCHAS.md`
**Performance**: `examples/references/PERFORMANCE_GUIDE.md`
**Verification**: `examples/references/VERIFICATION_GUIDE.md`

## Installation & Running

```bash
brew tap fizzbee-io/fizzbee && brew install fizzbee
fizz spec.fizz                                    # full model checking
fizz -x --max_runs 1 --seed 42 spec.fizz          # simulation
fizz --trace "Action1\nAction2" spec.fizz          # guided trace
fizz --preinit-hook "CONST=2" spec.fizz            # override constants
```

---

## File Structure

```
---
# Optional YAML frontmatter
options:
  max_actions: 100
  max_concurrent_actions: 2   # 1 for single-user apps
  crash_on_yield: true
deadlock_detection: true      # false for interactive apps
---

# Spec: constants, functions, roles, top-level actions, assertions
```

---

## Core Language

### Actions (model checker entry points — no parameters)

```python
# Modifier order: [flow] [fairness] action Name
action Init:                   # runs once; variables become global state
    counter = 0
    server = Server()

atomic action Increment:       # atomic = single indivisible step
    counter = counter + 1

fair action Progress:          # fair = weak fairness guarantee
    require not done
    done = True

atomic fair action Produce:    # flow first, then fairness
    require len(queue) < MAX
    queue.append(msg)
```

### Roles (actors/processes)

```python
@state(ephemeral=['cache'])    # cache resets on crash
role Server:
    action Init:
        self.data = {}         # durable state (persists)
        self.cache = {}        # ephemeral (resets on crash)

    atomic func Read(key):     # functions: have params + return values
        return self.data.get(key, None)

    action Write:
        key = any self.pending
        self.data[key] = self.pending[key]
```

### Flow Modifiers

```python
atomic action A:               # all statements in one step (no interleaving)
    x = 1; y = 2

action B:                      # serial (default): yields between statements
    x = 1                      # yield point
    y = 2                      # yield point

action C:
    parallel:                  # explores all interleavings
        x = x + 1
        y = y + 1

action D:
    oneof:                     # nondeterministically pick one branch
        status = "ok"
        status = "fail"
```

### Assertions

```python
always assertion Safe:                     # holds in ALL states
    return balance >= 0

exists assertion Reachable:                # holds in AT LEAST ONE state
    return done == True

transition assertion Monotonic(b, a):      # on every transition
    if a.counter != b.counter:
        return a.counter > b.counter
    return True                            # MUST allow stutter steps

eventually always assertion Stable:        # eventually permanently true (needs fair action)
    return done == True

always eventually assertion Progress:      # always makes progress (needs strong fairness)
    return processed > 0
```

### Nondeterminism

```python
x = any [1, 2, 3]             # pick any element (disables action if empty)
x = any items                  # pick any element from collection
```

### Guards

```python
require condition              # disables action if false (not an assertion!)
```

---

## Data Types

| Type | Example |
|---|---|
| list | `xs = []`, `xs.append(v)`, `xs.pop(0)` |
| set | `s = set()`, `s.add(x)`, `s.discard(x)` |
| dict | `d = {}`, `d[k] = v`, `d.get(k, default)` |
| bag (multiset) | `b = bag()`, `b.add(x)`, `b.count(x)` |
| record (mutable struct) | `r = record(x=1, y=2)`, `r.x = 3` |
| struct (immutable) | `s = struct(x=1)` |
| enum | `E = enum('A','B')`, `E.A` |

For non-hashable elements (local scope only): `genericset()`, `genericmap()`

---

## Symmetry Reduction

Dramatically reduces state space for interchangeable entities:

```python
# Unique IDs
IDS = symmetry.nominal(name="id", limit=3)    # limit = max coexisting
id = IDS.fresh()

# Interchangeable pool (task content, colors, etc.)
TEXTS = symmetry.nominal(name="task", limit=3)
v = any TEXTS.choices()    # grows naturally: 1 choice first, then more

# Symmetric roles (N! reduction for N instances)
symmetric role Worker:
    action Init:
        self.status = "idle"

workers = bag()            # MUST use bag(), not list, for symmetric roles
workers.add(Worker())
```

---

## Fault Injection

```yaml
options:
  crash_on_yield: true   # roles can crash at yield points
```

```python
@state(ephemeral=['cache'])   # only cache is lost on crash
role Node:
    action Init:
        self.data = 0    # durable
        self.cache = 0   # ephemeral — resets to Init value on crash
```

---

## Critical Gotchas

1. **Cross-role return**: `self.x = role.fn()` crashes — use `tmp = role.fn(); self.x = tmp`
2. **No tuple unpacking in for**: `for k, v in d.items()` fails — use `for k in d: v = d[k]`
3. **Ordering in Init**: create globals before roles that depend on them
4. **`require` ≠ assertion**: use `always assertion` for invariants
5. **Lists break symmetry**: use `bag()` not `[]` for symmetric role instances
6. **Function call syntax**: extract to variable before using in expressions
7. **No `is` operator**: use `== None`, not `is None`
8. **`any` on empty**: disables the action (no `require len > 0` needed)
9. **Modifier order**: `atomic fair action` — never `fair atomic action`
10. **Functions from top-level serial actions**: not allowed — wrap in `atomic` or move into a role

---

## Performance Tips

1. List comprehensions instead of for-loops (~35-45% fewer nodes)
2. `require all([...])` instead of loop+require (single statement)
3. Tight symmetry limits (= max *coexisting* values, not total ever)
4. `bag()` for symmetric role instances
5. `max_concurrent_actions: 1` for single-user app models
6. `deadlock_detection: false` for interactive apps

---

## Common Patterns

### Two-role communication

```python
role Client:
    action DoWork:
        result = server.Process(self.request)  # local var first!
        self.last_result = result

role Server:
    atomic func Process(req):
        return handle(req)

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
    queue.append(record(id=next_id, body="data"))

atomic fair action Receive:
    require len(queue) > 0
    msg = queue.pop(0)
```

### Dynamic actors

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
