---
description: >
  Debug a FizzBee spec that fails, produces unexpected results, is too slow,
  or has an incorrect state space. Use when the model checker reports FAILED,
  a trace is incomplete, state counts are wrong, or the spec is timing out.
---

# FizzBee Debugging Guide

**Full references**: `~/.claude/skills/fizzbee-docs/VERIFICATION_GUIDE.md`, `~/.claude/skills/fizzbee-docs/GOTCHAS.md`, `~/.claude/skills/fizzbee-docs/PERFORMANCE_GUIDE.md`

---

## When Model Checker Reports FAILED

A FAILED result means a safety assertion was violated. The output includes a counterexample trace.

**Step 1: Read the failing trace**

The output shows the sequence of actions that led to the violation. Look at:
- Which assertion failed (`AlwaysFoo`, `BalanceNonNegative`, etc.)
- What state variables were when it failed
- What action just ran before the failure

**Step 2: Reproduce with a guided trace**

Copy the failing action sequence and replay it:
```bash
fizz --trace "Node#0.RequestLock
Node#1.RequestLock
Node#0.DoWork" spec.fizz
```

Add `--trace-extend 1` to see what enabled actions exist at each step:
```bash
fizz --trace "Node#0.RequestLock" --trace-extend 1 spec.fizz
```

**Step 3: Reduce config to minimum**

Shrink the state space so you can visualize it:
```bash
fizz --preinit-hook "N=1" spec.fizz
dot -Tsvg out/*/graph.dot -o graph.svg && open graph.svg
# Graph auto-generated when < 250 nodes
```

In the SVG, look for:
- The state where the assertion fails (highlighted)
- What changed in the step before failure

---

## When Trace Is Incomplete

```
WARNING: Trace execution incomplete. Expected 8 links, executed 6 links.
```

A transition in your trace was blocked. Most common causes:
1. A `require` guard failed — a condition you expected to be true wasn't
2. A symmetry limit was exceeded (e.g., `limit=2` but trace creates 3 distinct values)
3. The action name in the trace doesn't match (case-sensitive, exact match)

Debug by running the trace up to the failing step and using `--trace-extend 1` to see what's actually enabled:
```bash
fizz --trace "Step1
Step2" --trace-extend 1 spec.fizz
```

---

## When Behavior Seems Wrong (But PASSED)

The model may be over-constraining (silent pruning) or assertions may be tautological.

**Check with simulation:**
```bash
for seed in 1 2 3 7 13 42; do
    fizz -x --max_runs 1 --seed $seed spec.fizz
    # Extract transitions
    grep -o 'label="[^"]*"' out/*/graph.dot | sed 's/.*label="//;s/"//'
done
```

**Ask:** Are all expected actions appearing? If an action never fires:
- Check `require` guards — is the condition too tight?
- Check enabling conditions — does the action body execute a simple statement?
- Remember: `any empty_list` disables the action

**Write a guided trace for the missing scenario:**
```bash
fizz --trace "ActionThatShouldHappen" spec.fizz
# If incomplete: the action's require guard is blocking it
```

---

## When State Count Is Unexpectedly Large

**Check symmetry configuration:**
- Are symmetric role instances stored in `bag()` or `list()`? Lists break symmetry — use `bag()`.
- Are symmetry limits too loose? Limit = max *coexisting* values, not total ever created.
- Are old symmetric values being cleaned up?

```python
# Wrong: list breaks symmetry
workers = []
workers.append(Worker())   # BAD

# Right: bag preserves symmetry
workers = bag()
workers.add(Worker())      # GOOD
```

**Apply performance techniques:**

Replace for-loops with list comprehensions (35-45% fewer nodes):
```python
# Slow: multiple statements, multiple yield points
result = []
for item in items:
    if item.active:
        result.append(item)

# Fast: single expression, single yield point
result = [item for item in items if item.active]
```

Replace loop+require with `require all([...])`:
```python
# Slow
for a in appointments:
    require not (a.slot == slot and a.day == day)

# Fast
require all([not (a.slot == slot and a.day == day) for a in appointments])
```

Use smaller configs for model checking:
```bash
fizz --preinit-hook "N=2" spec.fizz          # instead of N=5
```

---

## When Model Checker Times Out or Uses Too Much Memory

**Reduce config first:**
```bash
fizz --preinit-hook "N=1
SLOTS=1
WINDOW=1" spec.fizz
```

**Check for state space explosion causes:**
1. Non-atomic loops — convert to comprehensions or `atomic for`
2. Loose symmetry limits — tighten them
3. Too many concurrent actions — add `max_concurrent_actions: 1` for single-user models
4. Missing cleanup of old symmetric values

**Use DFS instead of BFS to find bugs faster:**
```bash
fizz --exploration_strategy dfs spec.fizz
```
DFS finds counterexamples faster; BFS finds the shortest ones.

**Profile the spec:**
- Run with `--max_runs 1` simulation first to get a baseline time
- Comment out assertions one by one to find which one is slow
- Use `fizz --internal_profile spec.fizz` for internal profiling

---

## Common Gotchas Checklist

| Symptom | Likely Cause | Fix |
|---|---|---|
| `fizz functions can only be called...` | `self.x = role.fn()` | Use `tmp = role.fn(); self.x = tmp` |
| Action never fires | `any empty_list` or `require` too tight | Check guards |
| Symmetry not working | Symmetric roles in a `list` | Use `bag()` |
| Role Init fails to see global | Global created after role | Reorder: create globals first |
| Transition assertion fails on startup | Missing stutter-step check | Add `if before.x != after.x:` |
| `for k, v in dict.items()` error | Tuple unpacking not supported | `for k in d: v = d[k]` |
| Model passes but wrong | `require` used instead of assertion | Use `always assertion` for invariants |

**Full gotchas list**: `~/.claude/skills/fizzbee-docs/GOTCHAS.md`

---

## Debugging Workflow Summary

```bash
# 1. Reproduce failure with minimal config
fizz --preinit-hook "N=1" spec.fizz

# 2. Visualize (works when < 250 nodes)
dot -Tsvg out/*/graph.dot -o graph.svg && open graph.svg

# 3. Guided trace to isolate the path
fizz --trace "Step1\nStep2\nStep3" spec.fizz

# 4. Explore beyond the trace
fizz --trace "Step1\nStep2" --trace-extend 2 spec.fizz

# 5. Simulate to explore behavior
fizz -x --max_runs 1 --seed 42 spec.fizz
```
