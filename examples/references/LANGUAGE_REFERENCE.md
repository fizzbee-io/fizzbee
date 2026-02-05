# FizzBee Language Reference

Complete reference guide for the FizzBee specification language for modeling distributed systems and concurrent algorithms.

**Version**: 0.2.0
**Last Updated**: January 2026

---

## Table of Contents

1. [Introduction](#introduction)
2. [File Structure](#file-structure)
3. [Keywords and Reserved Words](#keywords-and-reserved-words)
4. [Actions](#actions)
5. [Functions](#functions)
6. [Roles](#roles)
7. [Flow Modifiers](#flow-modifiers)
8. [Fairness](#fairness)
9. [Assertions](#assertions)
10. [Data Structures](#data-structures)
11. [Control Flow](#control-flow)
12. [Nondeterminism](#nondeterminism)
13. [State Management](#state-management)
14. [Fault Injection](#fault-injection)
15. [Configuration](#configuration)
16. [Symmetry Reduction](#symmetry-reduction)
17. [Best Practices](#best-practices)

---

## Introduction

FizzBee is a Python-like formal specification language for modeling distributed systems. It is based on Starlark (a Python subset) and provides explicit model checking with automatic fault injection capabilities.

### Key Differentiators from TLA+

1. **Python-like syntax**: Easy to learn and read
2. **Implicit fault injection**: Automatic crash and message loss simulation
3. **Non-atomic actions**: Natural modeling of multi-step operations
4. **Role-based modeling**: Object-oriented approach to distributed actors

### Language Foundation

FizzBee uses Starlark as its expression and statement engine:
- Python-compatible syntax for expressions
- Deterministic execution
- No arbitrary Python imports (hermetic environment)
- Built-in support for common data structures

---

## File Structure

### YAML Frontmatter (Optional)

FizzBee files can include YAML frontmatter for configuration:

```yaml
---
# Global options
options:
  max_actions: 100
  max_concurrent_actions: 2
  crash_on_yield: true

# Per-action options
action_options:
  ActionName:
    max_actions: 50
    crash_on_yield: false

# Other settings
deadlock_detection: true
liveness: strict
---
```

### Fizz Specification

After the frontmatter (or at the start if no frontmatter), the FizzBee specification contains:

1. **Top-level constants**: Global constant definitions
2. **Functions**: Pure or stateful functions (atomic or serial)
3. **Actions**: Model checker invoked operations
4. **Roles**: Class-like structures for actors/processes
5. **Assertions**: Safety and liveness properties

---

## Keywords and Reserved Words

### Core Keywords

- `action` - Defines an action (model checker entry point)
- `func` - Defines a function
- `role` - Defines a role (class-like structure)
- `symmetric` - Modifier for roles or values (enables symmetry reduction)
- `assertion` - Defines a property to check

### Flow Modifiers

- `atomic` - Execute without yielding
- `serial` - Execute with yields between statements (default)
- `parallel` - Execute statements concurrently
- `oneof` - Choose one of multiple branches

### Fairness Modifiers

- `fair` - Weak fairness (same as `fair<weak>`)
- `fair<weak>` - Eventually executes if continuously enabled
- `fair<strong>` - Eventually executes if infinitely often enabled

### Assertion Modifiers

- `always` - Safety property (holds in all states)
- `exists` - Existential property (holds in at least one state)
- `eventually always` - Liveness property (eventually reaches stable state)
- `always eventually` - Progress property (always makes progress)
- `transition` - Property about state transitions

### Control Flow

- `if`, `elif`, `else` - Conditional branching
- `for` - Iteration over collections
- `while` - Conditional iteration
- `break` - Exit loop early
- `continue` - Skip to next iteration
- `return` - Return from function/action
- `pass` - No-op (explicitly enables action)
- `require` - Guard clause (disables action if false)
- `any` - Nondeterministic choice
- `` `checkpoint` `` - Visualization breakpoint (backtick syntax)

### State and Scope

- `self` - Reference to role instance (implicit in role methods)
- `global` - NOT a keyword (state variables are automatically global)
- `@state` - Decorator for role state (durable/ephemeral)

### Built-in Types

- `enum` - Enumeration definition
- `record` - Mutable named structure
- `struct` - Immutable named structure (Starlark)
- `set`, `dict`, `list` - Standard collections
- `genericset`, `genericmap` - Collections for non-hashable types
- `bag` - Multiset (collection with duplicates)
- `symmetric_values` - Creates interchangeable values for symmetry reduction

---

## Actions

### Definition

Actions are the entry points for the model checker. They are automatically invoked by the model checker to explore the state space.

```python
action Init:
    # Initialize state variables
    counter = 0
    items = []

atomic action Increment:
    if counter >= 10:
        return
    counter = counter + 1
```

### Action Syntax

Actions can have optional modifiers that control execution flow and fairness:

```
[flow_modifier] [fairness_modifier] action ActionName:
```

**Modifier Order** (both optional):
1. **Flow modifier**: `atomic` or `serial` (default: `serial`)
2. **Fairness modifier**: `fair`, `fair<weak>`, or `fair<strong>` (default: unfair)
3. **`action` keyword**
4. **Action name**

**Examples**:

```python
# No modifiers - serial, unfair
action Process:
    counter = counter + 1

# Flow only - atomic, unfair
atomic action AtomicUpdate:
    x = x + 1
    y = y + 1

# Fairness only - serial, weakly fair
fair action Progress:
    done = True

# Both modifiers - atomic, weakly fair
atomic fair action ProduceMessage:
    require len(queue) < MAX_SIZE
    queue.append(message)

# Both modifiers - atomic, strongly fair
atomic fair<strong> action SelectValue:
    value = fair<strong> any options
```

**Common mistake**: Using `fair atomic action` instead of `atomic fair action` (syntax error)

### Characteristics

1. **No parameters**: Actions cannot take parameters (no parentheses)
2. **Automatic invocation**: Model checker calls actions automatically
3. **Special Init action**: Called once at the start
4. **Default flow**: Serial (yields between statements)
5. **State scope**: Variables in `Init` become global state

### Init Action

The `Init` action is special:
- Called exactly once at the beginning
- Initializes global state variables
- All variables defined in `Init` scope are part of the state
- Must be named exactly `Init`

```python
action Init:
    # These become state variables
    x = 0
    items = []
    config = {"key": "value"}
```

### Enabling Conditions

Actions can be conditionally enabled:

```python
action DoSomething:
    # Option 1: if-return pattern
    if not condition:
        return
    # Action body

action DoSomethingElse:
    # Option 2: require statement
    require condition
    # Action body
```

**Difference**:
- `if-return`: Action executes but does nothing
- `require`: Action is disabled (not executed at all)

### Action Enabling Semantics

**How actions become enabled**:

1. **Start disabled**: Every action starts with `enabled=False`
2. **Enabled by execution**: Action becomes enabled when it executes a simple Python statement (assignment, method call)
3. **Control flow doesn't enable**: `if`, `for`, `while`, `require` alone don't enable
4. **Disabled by require**: `require False` disables and exits immediately

**Examples**:

```python
action Example1:
    if condition:
        x = 1  # This enables the action
    # Action is enabled if condition is True

action Example2:
    require condition  # Disables if False
    # Action never becomes enabled if condition is False

action Example3:
    for i in range(10):
        pass  # for loop doesn't enable, but pass does
    # Action is enabled

action Example4:
    if False:
        x = 1  # Never executed
    # Action stays disabled (no statement executed)
```

**Simple Python statements** that enable actions:
- Assignments: `x = 1`, `list.append(2)`
- Function calls with assignment: `result = func()`
- `pass` statement

**Statements that DON'T enable**:
- Control flow: `if`, `elif`, `else`, `for`, `while`
- Flow control: `break`, `continue`, `return`
- Guards: `require`
- Non-determinism: `any` (but the assignment `x = any ...` does enable)

**Critical pitfall**:

```python
# ⚠️ This action is ALWAYS enabled (even when condition is False)
action Pitfall:
    is_ready = (condition == True)  # Assignment enables!
    if is_ready:
        process()

# ✅ Correct version - only enabled when condition is True
action Fixed:
    require condition
    process()
```

**Advanced pattern**: `require` in the middle of an action (after a yield point) can coordinate with other actions. See "Action Coordination with Require and Yield" in Quick Reference.

### Pass Statement

Use `pass` to explicitly enable an action without changing state:

```python
atomic action NoOp:
    pass  # Unconditionally enabled, does nothing
```

---

## Functions

### Definition

Functions are reusable code blocks that can be called from actions or other functions.

```python
atomic func add(x, y):
    return x + y

serial func process_items():
    for item in items:
        # Process each item
        result = transform(item)
```

### Characteristics

1. **Parameters**: Functions can take parameters
2. **Return values**: Can return values
3. **Flow modifiers**: Can be `atomic` or `serial` (default)
4. **Call restrictions**: Must be called from atomic context or roles
5. **Top-level or role-scoped**: Can be global or part of a role

### Atomic vs Serial Functions

**Atomic functions**:
- Execute without yielding
- All statements run as one indivisible step
- Faster, smaller state space

**Serial functions** (default):
- Yield between statements
- Allows interleaving with other actions
- More realistic for modeling sequential operations
- Must be called from atomic context or roles

### Calling Restrictions

```python
# ❌ WRONG: Calling function from non-atomic action
action BadExample:
    result = some_function()  # ERROR!

# ✅ CORRECT: Calling from atomic action
atomic action GoodExample:
    result = some_function()  # OK

# ✅ CORRECT: Calling from role
role MyRole:
    action DoWork:
        result = some_function()  # OK in roles
```

### Function Call Syntax Limitations

Fizz functions have restricted call syntax. They must follow this pattern:

```
[variable =] [role.]function([parameters])
```

**Cannot use fizz functions in complex expressions**:

```python
# ❌ WRONG: Function call in conditional
if replica.get_state() == State.ACTIVE:
    process()

# ✅ CORRECT: Extract to variable first
state = replica.get_state()
if state == State.ACTIVE:
    process()

# ❌ WRONG: Function call in arithmetic
result = compute() + 10

# ✅ CORRECT: Extract first
value = compute()
result = value + 10
```

**Reason**: Fizz function calls create yield points and must be tracked separately by the model checker.

**Note**: Standard Python/Starlark functions (built-ins like `len()`, `str()`, etc.) have no restrictions.

---

## Roles

### Definition

Roles are class-like structures representing actors, processes, or components in a distributed system.

```python
role Server:
    action Init:
        self.data = {}
        self.version = 0

    action Process:
        self.version = self.version + 1

    func get_data(key):
        return self.data.get(key, None)
```

### Characteristics

1. **Instance-based**: Create multiple instances
2. **State encapsulation**: Each instance has independent state
3. **Implicit self**: Like Java's `this`, no need to pass `self`
4. **Role Init**: Constructor-like initialization
5. **Actions and functions**: Can have both

### Role Init Action

The `Init` action in a role acts like a constructor:

```python
role Account:
    action Init:
        # Initialize instance state
        self.balance = 0
        self.owner = None

# Create instance with parameters
action Init:
    account = Account(INITIAL_BALANCE=100, ACCOUNT_ID=1)
```

### State Variables

Only `self.*` variables are part of role state:

```python
role Example:
    action Init:
        self.persisted = 0  # Part of state
        temp = 100          # NOT part of state (local variable)
```

### Special Role Fields

**`self.__id__`** - Auto-generated unique identifier:

```python
role Server:
    action Init:
        self.name = "server"
        # self.__id__ is automatically set by FizzBee

    action LogStatus:
        print(f"Server {self.__id__}: {self.status}")

action Init:
    servers = []
    for i in range(3):
        servers.append(Server())
    # servers[0].__id__ != servers[1].__id__ != servers[2].__id__
```

**Use cases**:
- Distinguishing role instances in logs
- Using instance ID in message routing
- Debugging and visualization

**Note**: `__id__` is read-only and unique across all role instances

### Durable vs Ephemeral State

Use `@state` decorator to mark ephemeral fields:

```python
@state(ephemeral=['cache'])
role Server:
    action Init:
        self.disk_data = 0  # Durable (persists across crashes)
        self.cache = 0      # Ephemeral (lost on crash)
```

**Alternatively, mark durable fields**:

```python
@state(durable=['disk_data'])
role Server:
    action Init:
        self.disk_data = 0  # Durable
        self.cache = 0      # Ephemeral (not in durable list)
```

**Rules**:
- Cannot specify both `durable` and `ephemeral`
- Default: all state is durable
- On crash: ephemeral fields reset to Init values

### Role Communication

Roles communicate by calling each other's functions:

```python
role Client:
    action SendRequest:
        response = server.process()  # RPC call

role Server:
    func process():
        return "OK"
```

**Automatic fault injection**: Message can be lost (see Fault Injection section)

---

## Flow Modifiers

Flow modifiers control how statements execute and when they yield control.

### Atomic

Executes all statements indivisibly, without yielding:

```python
atomic action UpdateBoth:
    x = x + 1
    y = y + 1
    # Both updates happen atomically
```

**Use cases**:
- Database transactions
- Critical sections
- Operations that must complete together

### Serial (Default)

Yields between statements, allowing interleaving:

```python
action UpdateBoth:  # serial is default
    x = x + 1  # Yield point after this
    y = y + 1  # Yield point after this
```

**Use cases**:
- Multi-step operations
- Modeling real-world sequencing
- Exploring crash scenarios

### Yield Points

Yield points are where the model checker can schedule other actions or inject faults.

**Yield points occur**:
- After each simple Python statement (in serial context)
- At the end of atomic blocks
- Before and after fizz function calls
- Examples: assignments, `pass`, method calls

**NO yield points after**:
- Control flow keywords: `if`, `elif`, `else`
- Loop keywords: `for`, `while`
- Flow control: `break`, `continue`, `return`
- Guards: `require`
- Non-determinism: `any`, `oneof` (yield after the full statement)
- Inside atomic blocks

**Examples**:

```python
action SerialExample:
    x = 1       # Yield point after
    y = 2       # Yield point after
    if x > 0:   # NO yield point
        z = 3   # Yield point after

atomic action AtomicExample:
    x = 1       # NO yield point (inside atomic)
    y = 2       # NO yield point
    # Yield point here (end of atomic block)

action MixedExample:
    x = 1       # Yield point
    atomic:
        y = 2   # NO yield (inside atomic)
        z = 3   # NO yield
    # Yield point here (end of atomic)
    w = 4       # Yield point
```

**Important for**:
- Crash injection (crashes happen at yield points)
- Action interleaving (other actions can run)
- Understanding execution model

### Parallel

Executes statements concurrently:

```python
atomic action UpdateConcurrently:
    parallel:
        x = x + 1
        y = y + 1
    # Explores all interleavings of the two updates
```

**Use cases**:
- Concurrent operations
- Parallel threads
- Race condition detection

### Oneof

Nondeterministically chooses one branch:

```python
atomic action MakeChoice:
    oneof:
        x = 1
        x = 2
        x = 3
    # Explores all three possibilities
```

**Use cases**:
- Modeling failures (success or failure)
- Non-deterministic choices
- Alternative execution paths

### Nested Modifiers

Flow modifiers can be nested:

```python
action Example:
    serial:
        step1 = True  # Yield after this

        atomic:
            # These execute together
            step2 = True
            step3 = True
```

---

## Fairness

Fairness ensures actions eventually execute under certain conditions.

### Unfair (Default)

No guarantees - action may starve:

```python
action MayStarve:
    counter = counter + 1
    # May never execute even if always enabled
```

### Weak Fairness (fair or fair<weak>)

Eventually executes if **continuously enabled**:

```python
fair action EventuallyRuns:
    if condition:
        done = True
    # Will execute if condition stays true
```

**Use case**: Actions that should run when persistently enabled

### Strong Fairness (fair<strong>)

Eventually executes if **infinitely often enabled**:

```python
fair<strong> action RunsEventually:
    if toggle:
        processed = True
    # Will execute even if toggle alternates
```

**Use case**: Actions enabled intermittently

### Fairness and Liveness

Fairness is required for liveness properties:

```python
fair action Progress:
    counter = counter + 1

eventually always assertion EventuallyDone:
    return counter >= 10
    # Passes because fair action ensures progress
```

### Combining Flow and Fairness Modifiers

Fairness can be combined with flow modifiers (remember: flow modifier comes first):

```python
# Atomic + weak fairness
atomic fair action ProduceMessage:
    require len(queue) < MAX_SIZE
    message = create_message()
    queue.append(message)
    # Executes atomically AND eventually runs if continuously enabled

# Atomic + strong fairness
atomic fair<strong> action ConsumeMessage:
    require len(queue) > 0
    message = queue.pop(0)
    process(message)
    # Executes atomically AND eventually runs if infinitely often enabled

# Serial (default) + weak fairness
fair action BackgroundTask:
    step1 = prepare()  # Yield point
    step2 = execute()  # Yield point
    # Can crash at yield points, but will eventually run if continuously enabled
```

**Important**: The modifier order is always `atomic fair action`, never `fair atomic action`.

---

## Assertions

Assertions specify properties the model checker should verify.

### Always (Safety)

Must hold in all reachable states:

```python
always assertion Invariant:
    return counter >= 0
```

**Use cases**: Invariants, safety properties

### Exists

Must hold in at least one reachable state:

```python
exists assertion Reachability:
    return counter == 10
```

**Use cases**: Reachability, possibility

### Transition

Must hold for all state transitions:

```python
transition assertion MonotonicIncrease(before, after):
    if after.counter != before.counter:
        return after.counter > before.counter
    return True  # Stutter step
```

**Stutter Invariance** (critical requirement):

Transition assertions **must allow stutter steps** (transitions where state doesn't change):

```python
# ✅ CORRECT: Allows stuttering
transition assertion NoDecrease(before, after):
    if after.value != before.value:
        return after.value >= before.value
    return True  # Stutter allowed

# ❌ WRONG: Requires change every step
transition assertion AlwaysIncreases(before, after):
    return after.value > before.value
    # Fails on stutter steps!
```

**Why stutter invariance matters**:
- Model checker may have steps where only thread-local variables change
- Some actions may execute without changing state variables
- FizzBee automatically passes assertion if no state variables changed

**Automatic handling**: If no state variables changed, assertion assumed satisfied (even if thread-local variables changed)

**Pattern**: Always check `before != after` before asserting a property about the change

### Eventually Always (Liveness)

Eventually reaches a stable state:

```python
eventually always assertion EventuallyStable:
    return done == True
```

**Requires**: Fairness on relevant actions

### Always Eventually (Progress)

Always makes progress:

```python
always eventually assertion AlwaysProgresses:
    return processed_count > 0
```

**Requires**: Strong fairness typically

---

## Data Structures

### Lists

Ordered, mutable collections:

```python
items = []
items.append(1)
items.insert(0, 2)
items.remove(1)
item = items.pop()
```

### Sets

Unordered collections of unique hashable elements:

```python
s = set()
s.add(1)
s.discard(2)
union = s1.union(s2)
intersection = s1.intersection(s2)
```

### Dicts

Key-value pairs (keys must be hashable):

```python
d = {}
d["key"] = "value"
value = d.get("key", default)
keys = d.keys()
values = d.values()
```

### Generic Collections

For non-hashable elements:

```python
# Generic set (allows dicts, lists as elements)
gs = genericset()
gs.add([1, 2, 3])
gs.add({"key": "value"})  # Dicts allowed

# Generic map (allows dicts, lists as keys)
gm = genericmap()
gm[[1, 2]] = "value"
gm[{"id": 1}] = "data"    # Dict keys allowed
```

**Limitations - Use in local scope only**:

```python
# ❌ WRONG: Generic collections in global state
action Init:
    global_set = genericset()  # Bad - state hashing issue

# ✅ CORRECT: Generic collections in local scope
atomic action Process:
    local_set = genericset()   # OK - local variable
    local_set.add([1, 2])
    process(local_set)
```

**Why this limitation**:
- Model checker needs to hash states for comparison
- Non-hashable types can't be hashed efficiently
- Use for temporary computation only

**When to use each**:
- Use `set`/`dict` when elements/keys are hashable (numbers, strings, tuples)
- Use `genericset`/`genericmap` when you need non-hashable elements (lists, dicts, records)
- Keep generic collections in local function/action scope

### Bags (Multisets)

Collections allowing duplicates:

```python
b = bag()
b.add(1)
b.add(1)  # Can add duplicates
count = b.count(1)  # Returns 2
```

### Enums

Named constants:

```python
State = enum('INIT', 'RUNNING', 'DONE')
current = State.INIT

# Iterate over enum values
for state in dir(State):
    # Process each state
```

### Records

Mutable named structures:

```python
msg = record(id=1, payload="data")
msg.id = 2  # Mutable
```

### Structs

Immutable named structures (Starlark):

```python
config = struct(timeout=30, retries=3)
# config.timeout = 60  # ERROR: immutable
```

---

## Control Flow

### If-Elif-Else

```python
if condition1:
    # Branch 1
elif condition2:
    # Branch 2
else:
    # Default branch
```

### For Loops

Can be atomic, serial, or parallel:

```python
# Atomic: all iterations together
atomic for i in range(10):
    counter = counter + 1

# Serial: yield between iterations
serial for item in items:
    process(item)

# Parallel: concurrent iterations
parallel for i in range(3):
    results[i] = compute(i)
```

### While Loops

```python
while counter < 10:
    counter = counter + 1
    # Yields after each iteration (if serial context)
```

### Break and Continue

```python
for i in range(10):
    if i == 5:
        break  # Exit loop
    if i % 2 == 0:
        continue  # Skip to next iteration
    process(i)
```

### Checkpoints

Checkpoints are visualization breakpoints for debugging and exploration:

```python
action Process:
    step1 = prepare()
    `checkpoint`  # Explicit breakpoint
    step2 = execute()
    `checkpoint`  # Another breakpoint
    step3 = finalize()
```

**Default checkpoints** (automatically created):
- At the start of each action
- At non-deterministic choices (`any`, `oneof`)
- At yield points

**Use cases**:
- Step-through debugging in explorer
- Creating meaningful visualization points
- Breaking down complex actions into observable steps

**Note**: Checkpoints don't affect execution semantics, only visualization

**Reference**: [99-01-checkpoints](99-01-checkpoints/) for a complete example

---

## Nondeterminism

### Any Statement

Nondeterministically chooses from a collection:

```python
# Choose any value
value = any [1, 2, 3, 4, 5]

# Choose with condition
value = any [x for x in options if x != previous]

# Choose from range
index = any range(10)
```

**Behavior**: Model checker explores all possible choices

**Important**: If no elements match the condition, the action is **disabled**:

```python
atomic action ProcessLargeValues:
    # If all values are <= 10, this action is disabled
    x = any [1, 2, 3, 4, 5] if x > 10
    # This line never executes
```

**Use case**: Conditional action enabling based on available choices

### Fairness with Any

```python
# Fair choice (eventually tries all)
fair action Choose:
    value = fair any options
```

### Concise vs Block Form

`any` has two forms. **Prefer the concise form** — it keeps code flat and readable:

```python
# ✅ Concise form (preferred): assigns and continues
n = any nodes
require status[n] == "active"
status[n] = "done"

# Block form: runs indented block for chosen value
any n in nodes:
    require status[n] == "active"
    status[n] = "done"
```

Both are semantically equivalent. The block form is the older pattern; the concise form avoids unnecessary indentation.

### Any vs Oneof

```python
# Any: chooses a value
x = any [1, 2, 3]

# Oneof: chooses a branch
oneof:
    x = 1
    x = 2
    x = 3
```

**Difference**: `any` is an expression, `oneof` is a statement block

---

## State Management

### Global State Variables

Variables defined in top-level `Init` action are automatically global:

```python
action Init:
    # These are global state variables
    counter = 0
    items = []
    config = {}

atomic action Increment:
    # Access global state directly
    counter = counter + 1
```

**No `global` keyword needed** - state variables are automatically accessible everywhere.

### Top-Level Constants

Variables defined at the top level (outside actions) are read-only constants:

```python
# Constants (read-only)
MAX_VALUE = 100
TIMEOUT = 30

action Init:
    # State variables (read-write)
    counter = 0
```

### Local Variables

Variables in functions/actions (not in Init) are local:

```python
atomic func process():
    temp = 100  # Local variable (not in state)
    return temp * 2
```

### Role State

Only `self.*` variables in roles are part of state:

```python
role Example:
    action Init:
        self.state_var = 0  # Part of state
        local_var = 100     # NOT in state
```

---

## Fault Injection

**FizzBee's key differentiator**: Automatic fault injection at yield points.

### Crash on Yield

By default, crashes can happen at any yield point:

```python
action MultiStep:
    step1 = True  # Yield - crash possible
    step2 = True  # Yield - crash possible
    step3 = True  # Yield - crash possible
```

**Model checker explores**:
- Normal execution (all steps complete)
- Crash after step1
- Crash after step2
- All combinations

### Enabling Crash Injection

```yaml
---
options:
  crash_on_yield: true  # Default
---
```

### Disabling Crash Injection

**Globally**:
```yaml
---
options:
  crash_on_yield: false
---
```

**Per-action**:
```yaml
---
action_options:
  CriticalPath:
    crash_on_yield: false
---
```

### Message Loss in RPC

When roles call each other, messages can be lost:

```python
role Client:
    action SendRequest:
        response = server.process()
        # Message can be lost:
        # 1. Request never sent
        # 2. Request sent, server processes, response lost
        # 3. Success (both ways)
```

**Automatic simulation** of:
- Network failures
- Process crashes
- Timeouts

### Ephemeral vs Durable State

```python
@state(ephemeral=['cache'])
role Server:
    action Init:
        self.disk_data = 0  # Durable
        self.cache = 0      # Ephemeral

    action Process:
        self.disk_data = self.disk_data + 1
        self.cache = self.cache + 1
        # If crash here:
        # - disk_data preserved
        # - cache reset to 0
```

### When Faults Are Injected

**Injected automatically**:
1. Message loss (role-to-role communication)
2. Thread crash (at yield points)
3. Process crash (ephemeral state loss)

**NOT injected (model explicitly)**:
1. Byzantine faults
2. Message duplication
3. Disk corruption
4. Message reordering

---

## Configuration

### Options

Global configuration in YAML frontmatter:

```yaml
---
# Global options
options:
  max_actions: 100              # Max total action executions
  max_concurrent_actions: 2     # Max actions running in parallel
  crash_on_yield: true          # Enable crash fault injection (default: true)

# Deadlock detection
deadlock_detection: true        # Enable deadlock checking (default: true)
                                # Set to false if deadlock is expected/acceptable

# Liveness checking
liveness: "false"               # Disable liveness checking
# liveness: "strict"            # Enable strict liveness checking
# liveness: true                # Enable liveness checking

# Custom seed for reproducibility
seed: 12345                     # Optional: for reproducing specific runs
---
```

**Available global options**:

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `max_actions` | int | unlimited | Total action executions before stopping |
| `max_concurrent_actions` | int | unlimited | Maximum actions running in parallel |
| `crash_on_yield` | bool | `true` | Enable automatic crash injection at yield points |
| `deadlock_detection` | bool | `true` | Check for deadlock states |
| `liveness` | string/bool | `false` | Liveness checking mode: `"false"`, `true`, `"strict"` |
| `seed` | int | random | Random seed for reproducible runs |

**Common configurations**:

```yaml
# Fast exploration (no crashes)
---
options:
  crash_on_yield: false
  max_concurrent_actions: 1
---

# Thorough testing
---
options:
  crash_on_yield: true
  max_concurrent_actions: 3
liveness: "strict"
deadlock_detection: true
---

# Debugging specific scenario
---
options:
  max_actions: 50
seed: 12345
---
```

### Action-Level Options

Override options per action:

```yaml
---
action_options:
  HighFrequency:
    max_actions: 50             # Limit this action

  CriticalPath:
    crash_on_yield: false       # No crashes for this

  BackgroundTask:
    max_concurrent_actions: 1   # Only one at a time
---
```

**For role actions**, use special notation:

```yaml
---
action_options:
  # Limit across ALL Server instances combined
  "Server.Process":
    max_actions: 10

  # Limit PER Server instance
  "Server#.Process":
    max_concurrent_actions: 1   # Each instance runs at most 1

  # Thread example: disable crashes, limit concurrency per thread
  "Thread.Run":
    crash_on_yield: false
  "Thread#.Run":
    max_concurrent_actions: 1   # Only 1 Run per Thread instance
---
```

**Notation**:
- `"Role.Action"` - Limits apply **across all instances** of the role
- `"Role#.Action"` - Limits apply **per individual instance** of the role
- Top-level actions - Use action name directly (no quotes needed)

**Example**:

```python
role Worker:
    action Process:
        # Do work
        pass

action Init:
    workers = [Worker(), Worker(), Worker()]

# With "Worker.Process": max_actions: 5
# → Total of 5 Process actions across all 3 workers

# With "Worker#.Process": max_actions: 5
# → Each worker can execute Process 5 times (15 total possible)
```

### Configuration Precedence

1. Action-specific options (highest priority)
2. Global options
3. Default values (lowest priority)

---

## Symmetry Reduction

**Symmetry reduction** is a powerful technique to reduce state space by exploiting symmetries in the model. When values or role instances are truly interchangeable, the model checker can treat permutations as equivalent, dramatically reducing the number of states to explore.

### Symmetric Values (Legacy)

> **Recommendation**: Prefer the [Symmetry Module API](#symmetry-module-api-advanced) for new models. It provides finer-grained control (nominal, ordinal, interval, rotational) and additional features (reflection, divergence, segments). The legacy `symmetric_values()` is equivalent to materialized nominal symmetry:
> ```python
> # Legacy
> KEYS = symmetric_values('k', 3)
> # Equivalent new API
> KEYS = symmetry.nominal(name="k", limit=3, materialize=True).values()
> ```

Use `symmetric_values()` to create interchangeable IDs:

```python
# Create symmetric values
KEYS = symmetric_values('k', 3)  # Creates k0, k1, k2
PROCESS_IDS = symmetric_values(5)  # Creates 0, 1, 2, 3, 4

action Init:
    switches = {}
    for k in KEYS:
        switches[k] = 'OFF'

atomic action TurnOn:
    key = any KEYS
    switches[key] = 'ON'
```

**Benefits**: States where `k0=ON,k1=OFF` and `k1=ON,k0=OFF` are recognized as equivalent.

**Example reduction**: With 3 processes, instead of exploring all permutations, only canonical states are explored.

**Reference**: [16-01-symmetric-values](16-01-symmetric-values/)

### Symmetric Roles

Mark roles as symmetric when instances are indistinguishable:

```python
NUM_WORKERS = 3

symmetric role Worker:
    action Init:
        self.status = Status.IDLE

    atomic action DoWork:
        self.status = Status.BUSY

action Init:
    # CRITICAL: Use bag() or set(), NOT list()
    workers = bag()
    for i in range(NUM_WORKERS):
        workers.add(Worker())
```

**Benefits**: Model checker recognizes role instance permutations as equivalent.

**Reduction factor**: For N symmetric role instances, state space reduces by factor of N! (factorial).

**Examples**:
- 2 workers: 2! = 2x reduction (~50%)
- 3 workers: 3! = 6x reduction (~83%)
- 4 workers: 4! = 24x reduction (~96%)

**Reference**: [16-02-symmetric-roles](16-02-symmetric-roles/)

### Critical Pitfall: Lists Break Symmetry

**WRONG - defeats symmetry**:
```python
symmetric role Node:
    # ...

action Init:
    nodes = []  # ❌ Lists are order-dependent!
    for i in range(NUM_NODES):
        nodes.append(Node())
```

**CORRECT - preserves symmetry**:
```python
symmetric role Node:
    # ...

action Init:
    nodes = bag()  # ✅ Bags are order-independent
    for i in range(NUM_NODES):
        nodes.add(Node())
```

**Why**: Lists preserve element order, so `[n0, n1]` ≠ `[n1, n0]`. This breaks symmetry reduction even with `symmetric role`.

**Fix**: Always use `bag()` or `set()` with symmetric roles, never `list()`.

**Reference**: [16-03-list-vs-bag-pitfall](16-03-list-vs-bag-pitfall/)

### When to Use Symmetry

**Use symmetric values/roles when**:
- All instances start in identical states
- Instances are truly indistinguishable
- Identity doesn't matter for correctness
- Order is not semantically meaningful

**Don't use when**:
- Instances have different initial states
- Some instances have special roles (leader, coordinator)
- Instance identity is semantically meaningful
- Order matters for the algorithm

### Order-Independent Collections

For maximum symmetry reduction:

```python
# Prefer bags over lists when order doesn't matter
nodes = bag()  # vs nodes = []

# Use sets for unique elements
completed = set()  # vs completed = []

# Dictionaries with symmetric keys work well
data = {}
for k in symmetric_values('k', 3):
    data[k] = initial_value
```

**Reference**: Compare state spaces in [16-04-symmetry-comparison](16-04-symmetry-comparison/)

### Measuring Impact

To see symmetry reduction in action:

1. Run without symmetry:
   ```python
   role Process:  # Regular role
       # ...

   action Init:
       processes = []  # List
   ```

2. Run with symmetry:
   ```python
   symmetric role Process:  # Symmetric role
       # ...

   action Init:
       processes = bag()  # Bag
   ```

3. Compare state/node counts in output

**Reference**: [16-04-symmetry-comparison](16-04-symmetry-comparison/) provides side-by-side comparison

### Symmetry Module API (Advanced)

The `symmetry` module provides fine-grained control over symmetric value domains with four symmetry types, each preserving different mathematical structure. Unlike `symmetric_values()` (which is nominal-only), the module lets you declare what operations are meaningful for your domain, enabling more aggressive state space reduction.

#### Quick Reference

| Type | Allowed Ops | Canonicalization | Typical Use |
|:---|:---|:---|:---|
| `nominal` | `==`, `!=` | Permutation of IDs | User IDs, session tokens |
| `ordinal` | `==`, `!=`, `<`, `>`, `<=`, `>=` | Rank squashing (0,1,2,...) | Logical timestamps, priorities |
| `interval` | `==`, `!=`, `<`, `>`, `<=`, `>=`, `+int`, `-int`, `val-val` | Zero-shifting (subtract min) | Sequence numbers, counters |
| `rotational` | `==`, `!=`, `+int`, `-int`, `val-val` | Rotate to lex-smallest set | Ring positions, clock arithmetic |

#### Constructors

```python
symmetry.nominal(name, limit, materialize=False)
symmetry.ordinal(name, limit, reflection=False, materialize=False)
symmetry.interval(name, divergence=None, limit=None, start=0, reflection=False, materialize=False)
symmetry.rotational(name, limit, materialize=False, reflection=False)
```

**Parameters**:
- `name` (string): Domain identifier, used in value display (e.g., name="ts" produces ts0, ts1, ...)
- `limit` (int): Maximum number of values in the domain
- `divergence` (int, interval only): Maximum allowed spread (max - min). If only `divergence` given, `limit = divergence + 1`. If only `limit` given, `divergence = limit - 1`.
- `start` (int, interval only): Starting value for first allocation (default 0)
- `reflection` (bool): Enable mirror-state equivalence (not available for nominal)
- `materialize` (bool): Pre-populate all `limit` values at declaration time. When true, `fresh()` is disallowed.

#### Methods by Type

| Method | Nominal | Ordinal | Interval | Rotational | Description |
|:---|:---:|:---:|:---:|:---:|:---|
| `fresh()` | Y | Y | Y | Y | Allocate new canonical value |
| `values()` | Y | Y | Y | Y | List active values (sorted) |
| `choose()` | Y | - | - | Y | Deterministic default value (like TLA+ CHOOSE) |
| `choices()` | Y | - | - | Y | `values()` + one `fresh()` |
| `min()` | - | Y | Y | - | Smallest active value or fresh |
| `max()` | - | Y | Y | - | Largest active value or fresh |
| `segments(after?, before?)` | - | Y | - | - | Gaps between active values |

#### Nominal Symmetry

Values are unordered and interchangeable. Only equality testing is allowed. The model checker treats any permutation of IDs as equivalent.

```python
IDS = symmetry.nominal(name="id", limit=3)

action Init:
    cache = {}

atomic action Put:
    id = any IDS.choices()  # existing IDs + one fresh
    cache[id] = "data"
```

`fresh()` allocates the smallest unused ID (canonical form). `choices()` returns all active values plus one fresh value if the limit allows -- use with `any` for nondeterministic selection. `choose()` returns a deterministic default value (like TLA+'s CHOOSE) -- use it when you need an initial value, not for nondeterministic selection.

**Reference**: [16-05-nominal-symmetry](16-05-nominal-symmetry/)

#### Ordinal Symmetry

Values are ordered but only relative rank matters. Distances between values are not preserved. The model checker squeezes values to dense ranks (0, 1, 2, ...).

```python
TIMES = symmetry.ordinal(name="ts", limit=4)

action Init:
    events = []

atomic action RecordEvent:
    # fresh() appends after max; min()/max() return the extremes.
    t = TIMES.fresh()
    events = events + [t]

atomic action ProcessEvent:
    require len(events) > 0
    events = events[1:]     # frees a slot for fresh() again
```

`fresh()` always allocates a value greater than all existing values (tail append). Use `segments()` to insert between existing values (see below).

**Reference**: [16-06-ordinal-symmetry](16-06-ordinal-symmetry/)

##### Ordinal Segments (Gap Insertion)

`segments()` returns gap objects between active values. Each gap has a `fresh()` method to allocate a value within that range.

```python
TIMES = symmetry.ordinal(name="ts", limit=6)

action Init:
    t_start = TIMES.fresh()
    t_end = TIMES.fresh()

atomic action InsertBetween:
    # segments(after=v, before=v) filters to gaps in range.
    # Ordinal gaps are always non-empty (the domain is dense).
    gaps = TIMES.segments(after=t_start, before=t_end)
    gap = any gaps
    t = gap.fresh()  # guaranteed: t_start < t < t_end
```

With N active values, `segments()` returns N+1 gaps: a head gap (before first), body gaps (between consecutive pairs), and a tail gap (after last). Ordinal gaps are always non-empty since the domain is dense (infinitely divisible in theory).

**Reference**: [16-07-ordinal-segments](16-07-ordinal-segments/)

#### Interval Symmetry

Values are ordered and distances are meaningful. Supports arithmetic on values. The model checker normalizes by subtracting the minimum (zero-shifting), so `{5,7,8}` and `{0,2,3}` are equivalent.

```python
TICKS = symmetry.interval(name="t", limit=6)

action Init:
    t1 = TICKS.min()
    t2 = TICKS.min()   # same value as t1

atomic fair action Tick1:
    t1 = t1 + 1        # val + int -> val

atomic fair action Tick2:
    t2 = t2 + 1
```

**Arithmetic**: `val + int` and `val - int` produce new symmetric values. `val1 - val2` produces a plain `int` (the distance). Only the gap pattern matters -- the model checker recognizes that `{t1=0,t2=3}` is equivalent to `{t1=100,t2=103}`.

**Reference**: [16-08-interval-symmetry](16-08-interval-symmetry/)

##### Divergence (Bounding Spread)

The `divergence` parameter limits `max - min` across all active values. Transitions that would exceed this bound are pruned from the state space.

```python
# max spread of 3: states where (max - min) > 3 are unreachable
SEQ = symmetry.interval(name="s", divergence=3)

action Init:
    head = SEQ.fresh()     # s0
    tail = SEQ.fresh()     # s1

atomic fair action AdvanceHead:
    head = head + 1        # pruned if head - tail > 3

atomic fair action AdvanceTail:
    require tail < head
    tail = tail + 1
```

**Derivation rules**: Provide `divergence`, `limit`, or both. If only one is given, the other is derived: `limit = divergence + 1` or `divergence = limit - 1`.

**Reference**: [16-09-interval-divergence](16-09-interval-divergence/)

#### Rotational Symmetry

Values are integers mod `limit` (ring positions). Arithmetic wraps around. No ordering operators (`<`, `>` not supported) since the domain is circular.

```python
RING = symmetry.rotational(name="pos", limit=5)

action Init:
    positions = set()

atomic action Place:
    p = RING.fresh()
    positions.add(p)

atomic action Advance:
    p = any positions
    next_p = p + 1          # wraps: 4 + 1 = 0 on ring of 5
    if next_p not in positions:
        positions.remove(p)
        positions.add(next_p)
```

The model checker rotates all values by a constant to find the lexicographically smallest set. So `{0,2}`, `{1,3}`, `{2,4}`, `{3,0}`, `{4,1}` are all the same state (gap pattern = {0,2}).

`val1 - val2` returns a plain `int`: `(a - b) % limit`.

**Reference**: [16-10-rotational-symmetry](16-10-rotational-symmetry/)

#### Reflection Symmetry

Setting `reflection=True` makes mirror-image states equivalent. Available for ordinal, interval, and rotational symmetry (not nominal, since nominal has no structure to reflect).

**Interval reflection**: `{t1=0, t2=3}` is equivalent to `{t1=3, t2=0}`. The model checker considers both `v - min` and `max - v` normalizations and picks the canonical one.

```python
# With reflection: "t1 ahead by 3" = "t2 ahead by 3"
TICKS = symmetry.interval(name="t", limit=6, reflection=True)
```

**Rotational reflection**: Clockwise and counterclockwise orientations are equivalent. On a ring of 6, `{0,1}` (gap=1 clockwise) and `{0,5}` (gap=1 counterclockwise) collapse to one state.

```python
# Undirected ring: CW and CCW are equivalent
RING = symmetry.rotational(name="pos", limit=6, reflection=True)
```

**Impact**: For the two-ticker interval example with `limit=6`: without reflection = 11 states, with reflection = 6 states (~45% reduction).

**Reference**: [16-11-reflection](16-11-reflection/)

#### Materialize

With `materialize=True`, all `limit` values exist from the start. `fresh()` is not allowed. Useful when the value set is fixed and known upfront.

```python
NODES = symmetry.rotational(name="n", limit=4, materialize=True)

action Init:
    status = {}
    for n in NODES.values():  # all 4 values available immediately
        status[n] = "idle"
```

Works with all symmetry types. For interval, values start at `start`: e.g., `symmetry.interval(name="x", limit=3, start=10, materialize=True)` creates values 10, 11, 12.

**Reference**: [16-12-materialize](16-12-materialize/)

#### Gotchas and Tips

1. **`fresh()` is deterministic, not nondeterministic.** It always returns the canonical next value. For nondeterministic choice, use `any domain.choices()` (nominal/rotational) or `any domain.values()`.

2. **Domain methods cannot be called from assertions.** The symmetry context is only available during action execution. Store values in state variables and check those in assertions instead.

3. **Limit enforcement is automatic.** When `fresh()` would exceed the limit, the transition is disabled (pruned from the state graph), not an error.

4. **Divergence enforcement is automatic.** For interval symmetry, transitions that would make `(max - min) > divergence` are pruned, including arithmetic results.

5. **Use `set()` or `bag()` to store symmetric values**, not `list()`. Lists preserve insertion order, which breaks symmetry reduction (same pitfall as with `symmetric role`).

6. **Choosing the right type**:
   - Only need identity? Use **nominal** (maximum reduction).
   - Need ordering but not distances? Use **ordinal**.
   - Need ordering AND distances/arithmetic? Use **interval**.
   - Values wrap around (mod N)? Use **rotational**.

7. **`require` is a guard, not an assertion.** `require cond` disables the transition when `cond` is false -- the action simply doesn't execute. It does **not** report a failure. To check properties, use `always assertion`. Prefer `require` over `if not cond: return` for enabling conditions -- they have the same effect but `require` is more concise and idiomatic.

---

## Best Practices

### State Space Management

**Always bound your state**:
```python
# ❌ BAD: Unbounded
action Increment:
    counter = counter + 1  # Grows forever!

# ✅ GOOD: Bounded
action Increment:
    if counter >= MAX_VALUE:
        return
    counter = counter + 1
```

**Use modulo for cyclic behavior**:
```python
counter = (counter + 1) % MAX_VALUE
```

### Guard Clauses

Use early returns for enabling conditions:

```python
action Process:
    if not ready:
        return
    if items_empty:
        return
    # Main logic here
```

### Multiple Assignment for Atomicity

Use tuple assignment to update multiple variables atomically:

```python
# ❌ Creates 3 yield points in serial context
action Update:
    a = 1  # Yield point
    b = 2  # Yield point
    c = 3  # Yield point
    # Other actions can run between these

# ✅ Single statement = atomic in serial context
action Update:
    a, b, c = 1, 2, 3  # Single yield point after
    # All three updated together
```

**Use cases**:
- Swapping values: `x, y = y, x`
- Coordinated updates: `min_val, max_val = 0, 100`
- Avoiding intermediate states visible to other actions

**Note**: This applies to serial contexts. In atomic blocks, all statements are atomic anyway.

### Variable Extraction Changes Behavior

Extracting local variables can alter execution order and atomicity:

```python
# Original: Function called every iteration
action Process:
    for i in range(get_count()):  # get_count() called each iteration
        process(i)

# Extracted: Function called once
action Process:
    count = get_count()  # Called once, creates yield point
    for i in range(count):
        process(i)
```

**Impact**:
- Changes when function is evaluated
- Creates additional yield points
- May change behavior if state changes between calls

**Best practice**: Be deliberate about extraction for clarity, but understand the semantic difference.

### Atomic vs Serial

**Prefer atomic when**:
- Operations must complete together
- Modeling transactions
- Reducing state space

**Use serial when**:
- Modeling real sequencing
- Testing crash scenarios
- Exploring interleavings

### Function Calls

**Remember restrictions**:
```python
# Functions must be called from:
# 1. Atomic context
atomic action CallFunc:
    result = my_function()  # OK

# 2. Roles
role MyRole:
    action DoWork:
        result = my_function()  # OK

# NOT from serial actions
action BadExample:
    result = my_function()  # ERROR!
```

### Role Design

**Encapsulate related state**:
```python
# ✅ GOOD: Cohesive role
role Account:
    action Init:
        self.balance = 0
        self.transactions = []

    func deposit(amount):
        self.balance += amount
```

**Mark ephemeral state explicitly**:
```python
@state(ephemeral=['cache', 'temp_results'])
role Worker:
    # Durable state persists across crashes
    # Ephemeral state is lost
```

### Assertions

**Write clear invariants**:
```python
# ✅ GOOD: Clear condition
always assertion BalanceNonNegative:
    return account.balance >= 0

# ❌ BAD: Complex, unclear
always assertion ComplexCheck:
    return (a > 0 and b < 10) or (c == d and e != f)
```

**Use appropriate assertion types**:
- `always`: Safety (invariants)
- `eventually always`: Liveness (termination)
- `exists`: Reachability
- `transition`: State transition properties

### Fairness

**Add fairness for liveness**:
```python
# Without fairness - may starve
action Progress:
    counter = counter + 1

# With fairness - will execute
fair action Progress:
    counter = counter + 1

eventually always assertion EventuallyDone:
    return counter >= 10  # Requires fairness
```

### Crash Injection

**Use selectively**:
```python
# Enable for fault-sensitive paths
options:
  crash_on_yield: true

# Disable for performance modeling
action_options:
  ComputeMetrics:
    crash_on_yield: false
```

---

## Quick Reference

### Common Patterns

**Mutex**:
```python
action AcquireLock:
    require lock_holder == None
    lock_holder = "me"
```

**Producer-Consumer**:
```python
action Produce:
    if len(buffer) >= CAPACITY:
        return
    buffer.append(item)

action Consume:
    require len(buffer) > 0
    item = buffer.pop(0)
```

**Retry with Backoff**:
```python
action Retry:
    if retries >= MAX_RETRIES:
        return
    if attempt():
        success = True
    else:
        retries = retries + 1
```

**Action Coordination with Require and Yield** ([Example 0083](15-02-action-coordination/)):

Use `require` in the middle of an action to wait for conditions set by other actions:

```python
---
deadlock_detection: false
options:
    crash_on_yield: false
---

action Init:
    a = 0

action Step1:
    require a == 0     # Only start when a is 0
    a = 1              # Update a, then yield point
    require a == 2     # Wait here until a becomes 2
    a = 3              # Continue when condition met

action Step2:
    require a == 1     # Only enabled when a is 1
    a = 2              # Set a to 2, unblocking Step1
```

**How this works**:

1. Step1 starts (a == 0), sets a = 1
2. **Yield point** after `a = 1`
3. Step1 thread tries to continue, hits `require a == 2` which **fails**
4. Step1 continuation is **disabled** (not scheduled)
5. Model checker schedules another action
6. Step2 is now **enabled** (a == 1), runs and sets a = 2
7. Step1 can now be **rescheduled**, `require a == 2` passes
8. Step1 continues to set a = 3

**Effect**: Step1 effectively "waits" for Step2 to update the state before continuing

**Use cases**:
- Synchronization between actions
- Handshake protocols
- Multi-phase coordination
- Waiting for preconditions without busy-waiting

**Key insight**: `require` in the middle of an action + yield points = coordination mechanism

**Note**: This pattern explores interleavings where actions wait for each other, useful for modeling synchronization primitives

### Debugging Tips

1. **Start small**: Test with small bounds first
2. **Check state space**: Monitor node/state counts
3. **Use assertions**: Verify invariants as you go
4. **Disable crashes**: Test happy path first with `crash_on_yield: false`
5. **Add NoOp actions**: Prevent deadlocks at terminal states

---

## Known Issues and Limitations

### Current Limitations

**Line numbers in error messages**: Line numbers may be incorrect in some error messages (known issue, to be fixed)

**Assertions within roles**: Assertions within role definitions are not yet fully supported. Use top-level assertions instead.

**Interactive commands**: Commands requiring interactive input (e.g., `git rebase -i`, `git add -i`) are not supported in model checking context

**Python features not supported**:
- `del` statement
- Arbitrary Python imports (hermetic environment)
- Some advanced Python features

### Workarounds

**For role assertions**: Define assertions at top level and reference role state:

```python
role Server:
    action Init:
        self.value = 0

# ✅ Top-level assertion instead of role assertion
always assertion ServerValuePositive:
    for s in servers:
        if s.value < 0:
            return False
    return True
```

**For complex expressions with fizz functions**: Extract to variables first (see Function Call Syntax Limitations)

**For generic collections in state**: Use regular collections with hashable types, or restructure to keep generic collections in local scope

---

## Version History

- **0.2.0** (January 2026): Added fault injection documentation
- **0.1.0** (Initial): Core language features

---

## See Also

- [Example Reference](README.md) - 83 runnable examples
- [FizzBee Documentation](https://fizzbee.io/docs/)
- [GitHub Repository](https://github.com/fizzbee-io/fizzbee)

---

*This reference guide covers FizzBee v0.2.0. For the latest updates, visit https://fizzbee.io*
