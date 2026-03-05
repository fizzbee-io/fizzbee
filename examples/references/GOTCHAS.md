# FizzBee Gotchas and Common Issues

Known pitfalls, language quirks, and their workarounds.

---

## 1. Cross-Role Function Return to `self.x` Crashes

**Symptom**: `fizz functions can be called only in the following ways` error.

**Cause**: Direct assignment `self.x = otherRole.someFn()` is not supported.

**Workaround**: Use a local variable as an intermediary.

```python
# CRASHES
atomic action ReadDoc:
    self.cached = db.ReadDoc(doc_id)

# WORKS
atomic action ReadDoc:
    content = db.ReadDoc(doc_id)
    self.cached = content
```

This applies to any cross-role function call assigned directly to `self.*`.
Local-to-local (`x = otherRole.fn()`) and same-role (`self.x = self.fn()`)
both work fine.

---

## 2. No Tuple Unpacking in For Loops

**Symptom**: Parse error or unexpected behavior with `for k, v in dict.items()`.

**Cause**: FizzBee/Starlark does not support tuple unpacking in `for` loop headers.

**Workaround**: Iterate over keys and index into the dict.

```python
# FAILS
for k, v in my_dict.items():
    process(k, v)

# WORKS
for k in my_dict:
    v = my_dict[k]
    process(k, v)
```

Note: Dict comprehensions with tuple unpacking **do** work:
`{k: v for k, v in items}` is fine.

---

## 3. Role Init Cannot Access Globals Being Assigned

**Symptom**: Role's Init action fails to see a global variable that is
being assigned in the same top-level Init block.

**Cause**: When a role is instantiated with `Role()`, its Init runs
immediately. If the global hasn't been assigned yet in the enclosing
Init, the role's Init can't see it.

**Workaround**: Assign globals before creating roles that depend on them.

```python
# FAILS: db is not yet assigned when Employee() runs
action Init:
    emp = Employee()   # Employee.Init might reference db
    db = Server()

# WORKS: db exists before Employee is created
action Init:
    db = Server()
    emp = Employee()
```

---

## 4. `None` Works in FizzBee

Despite being based on Starlark, FizzBee supports the `None` keyword.

```python
# All of these work
x = None
if x == None:       # True
if x != None:       # False
```

Note: There is no `is` operator. Use `== None` and `!= None`.

---

## 5. Lists Break Symmetric Role Reduction

**Symptom**: State space is much larger than expected with symmetric roles.

**Cause**: Lists preserve insertion order, which distinguishes otherwise
equivalent states. `[A, B]` and `[B, A]` are different list states even
if A and B are symmetric.

**Fix**: Always use `bag()` or `set()` to hold symmetric role instances.

```python
# BAD: list preserves order, breaks symmetry
workers = []
workers.append(Worker())

# GOOD: bag is unordered, symmetry works
workers = bag()
workers.add(Worker())
```

See reference example [16-03-list-vs-bag-pitfall](16-03-list-vs-bag-pitfall/).

---

## 6. `require` Is a Guard, Not an Assertion

**Symptom**: Model passes even though you expected a failure.

**Cause**: `require condition` silently disables the action when the
condition is false. It does not report a failure.

```python
# This does NOT check that balance is always >= 0.
# It just prevents Withdraw from running when balance < amount.
atomic action Withdraw:
    require self.balance >= amount
    self.balance = self.balance - amount

# To check the invariant, use an assertion:
always assertion BalanceNonNegative:
    return account.balance >= 0
```

---

## 7. `any` on Empty Collection Disables the Action

**Symptom**: Action never fires.

**Cause**: `any []` (empty iterable) disables the action, similar to
`require False`. This is by design — there's nothing to choose from.

**Implication**: You don't need `require len(xs) > 0` before `any xs`:

```python
# Redundant guard — any already handles empty case
require len(items) > 0
chosen = any items

# Equivalent and simpler
chosen = any items
```

---

## 8. Symmetric Value Arithmetic Can Exceed Domain Limits

**Symptom**: State space explodes with interval or ordinal symmetry
despite having a small limit.

**Cause**: Arithmetic on symmetric values (`val + 1`, `val - 1`) creates
new values without checking the domain's limit. Only `fresh()` checked
the limit. (Fixed in commit after Feb 2026.)

**Affected**: interval and ordinal symmetry types.
**Not affected**: rotational (wraps mod limit) and nominal (no arithmetic).

**Mitigation**: Update to the latest FizzBee version which includes the
limit check in `CheckSymmetryConstraints`. After the fix, transitions
that would create more distinct values than the limit are automatically
pruned.

---

## 9. Starlark Differences from Python

FizzBee uses Starlark, which is a Python subset with some differences:

- **No `is` operator**: Use `==` instead of `is`
- **No `del` statement**: Reassign to remove (e.g., filter a list)
- **No tuple unpacking in `for`**: See gotcha #2 above
- **No `try/except`**: Errors terminate the model checker
- **No classes**: Use `role` for stateful objects, `record` for data
- **No `import`**: Everything is in one file (hermetic)
- **Integers only**: No floats (use scaled integers for ratios)
- **Dicts are insertion-ordered**: But don't rely on this for symmetry
- **`set()` requires hashable elements**: Use `genericset()` for
  non-hashable (dicts, lists) but only in local scope

---

## 10. Dynamic Role Removal Pattern

Removing a role from a bag requires rebuilding the bag with a filter:

```python
# Remove a specific role instance by __id__
employees = bag([e for e in employees if e.__id__ != target_id])
```

There is no `bag.remove()` method. The list comprehension creates a
new bag without the removed role.

For symmetric roles, this works naturally — a removed role's slot is
reusable because new instances are interchangeable with old ones.

---

## 11. `any` Keyword vs. Python `any()` Function Collision

**Symptom**: Parse error or unexpected behavior when using `any(...)` to
check if at least one element in a collection satisfies a condition.

**Cause**: `any` is a FizzBee keyword for nondeterministic choice
(deprecated — prefer `oneof`). The parser tries to interpret `any` as the
nondeterministic choice operator first. When used in an assignment like
`x = any([...])`, the parser sees it as a nondeterministic choice over a
list literal, not a function call, causing a parse error or wrong semantics.

In some contexts (e.g., directly inside `if` or `require`) the parser
falls back to treating `any` as an identifier, so the Python `any()`
function *does* work — but this is context-dependent and fragile.

```python
# FAILS: parser treats this as nondeterministic choice, not function call
is_valid = any([x > 0 for x in items])

# WORKS in these contexts (any treated as identifier/function):
if not any([x > 0 for x in items]):
    return
require any([x > 0 for x in items])
```

**Workarounds** (prefer these — they work unambiguously in all contexts):

```python
# Option 1: use all() with negation (symmetric, always safe)
require all([x > 0 for x in items])        # require all positive
is_valid = all([x > 0 for x in items])     # assign works fine

# Option 2: use len() > 0 on a filtered comprehension
is_valid = len([x for x in items if x > 0]) > 0

# Option 3: inline into if/require directly (works but couples logic)
if not any([x > 0 for x in items]):
    return
```

**Rule of thumb**: never write `x = any(...)`. Use `all(...)` or
`len([...]) > 0` for assignments. For guards, `require all([not ...])` is
the idiomatic FizzBee style anyway (see Performance Guide).

**Note**: The `oneof` keyword (preferred over `any` for nondeterministic
choice) does not have this collision — `oneof` cannot be used as an
identifier, so `oneof([...])` is always a parse error. Use `oneof` to
avoid the ambiguity entirely:

```python
x = oneof items          # preferred (no collision risk)
x = any items            # deprecated (emits DeprecationWarning)
```

---

## 12. Function Calls Only from Atomic Context or Roles

**Symptom**: `fizz functions can be called only in the following ways` error.

**Cause**: Functions (`func`) can only be called from:
1. Inside an `atomic` block or `atomic action`
2. Inside a role's action
3. From another function

They cannot be called from a top-level serial (non-atomic) action.

```python
# FAILS: serial action calling a function
action Process:
    result = compute()  # Error!

# WORKS: atomic action
atomic action Process:
    result = compute()

# WORKS: role action (implicitly has a frame)
role Worker:
    action Process:
        result = compute()
```
