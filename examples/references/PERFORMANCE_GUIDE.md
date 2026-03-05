# FizzBee Performance Guide

Practical tips for reducing state space and speeding up model checking.
Each tip is ranked by typical impact.

---

## 1. Use List Comprehensions Instead of For Loops (Constant Multiplier)

FizzBee evaluates list comprehensions as a single internal operation.
An explicit `for` loop with `if` and append creates multiple statements,
each of which becomes a separate node in the state graph.

**Impact**: ~35-45% fewer nodes, ~45% faster (measured on salon-schedule model).

```python
# Slow: 7 statements per iteration
booked = False
for a in db.appointments:
    if a.cal_day == day and a.slot == s:
        booked = True
if not booked:
    available = available + [record(cal_day=day, slot=s)]

# Fast: single expression
if all([not (a.cal_day == day and a.slot == s) for a in db.appointments]):
    available = available + [record(cal_day=day, slot=s)]
```

You can collapse entire nested loops into a single comprehension:

```python
# Slow: nested for loops with conditions
available = []
for d in range(BOOKING_WINDOW):
    for s in range(SLOTS_PER_DAY):
        if schedule_code in db.schedule:
            if d > 0 or s + 1 > clock_phase:
                booked = False
                for a in db.appointments:
                    if a.cal_day == clock_day + d and a.slot == s:
                        booked = True
                if not booked:
                    available = available + [record(cal_day=clock_day + d, slot=s)]

# Fast: single list comprehension
available = [record(cal_day=clock_day + d, slot=s)
    for d in range(BOOKING_WINDOW)
    for s in range(SLOTS_PER_DAY)
    if schedule_code in db.schedule
    if d > 0 or s + 1 > clock_phase
    if all([not (a.cal_day == clock_day + d and a.slot == s) for a in db.appointments])]
```

### Use `require all([...])` Instead of For-Loop Require

A common pattern is checking a condition on all items in a collection:

```python
# Slow: loop creates N statements
for a in self.appointments:
    require not (a.cal_day == cal_day and a.slot == slot_idx)

# Fast: single statement
require all([not (a.cal_day == cal_day and a.slot == slot_idx) for a in self.appointments])
```

Both have the same semantics: if any element violates the condition, the
action is disabled.

The filtered variant works too:

```python
# Slow
for a in self.appointments:
    if a.id == appt_id:
        require a.cal_day > clock_day

# Fast
require all([a.cal_day > clock_day for a in self.appointments if a.id == appt_id])
```

Note: `all([])` returns `True` for an empty list, which is correct
for both patterns (no items means no violations).

---

## 2. Reduce Statement Count (Constant Multiplier)

Every statement in a non-atomic action is a potential yield point and creates
nodes in the state graph. Fewer statements = fewer nodes.

### `any` Already Blocks on Empty Collections

`any iterable` disables the action if the iterable is empty.
No need for a separate `require len(...) > 0`:

```python
# Unnecessary: 3 statements
future = [a for a in self.my_appts if is_future(a)]
require len(future) > 0
appt = any future

# Better: 1 statement
appt = any [a for a in self.my_appts if is_future(a)]
```

### Collapse Intermediate Variables

If a variable is used only once, inline it:

```python
# 3 statements
dow_idx = any range(DAYS_IN_WEEK)
slot = any range(SLOTS_PER_DAY)
schedule_code = dow_idx * SLOTS_PER_DAY + slot

# 1 statement (equivalent: nondeterministically pick one encoded code)
schedule_code = any range(DAYS_IN_WEEK * SLOTS_PER_DAY)
```

### Pass Pre-Computed Values

If both caller and callee need the same derived value, compute it once
and pass it rather than recomputing:

```python
# Caller already computed dow_idx for the schedule check
chosen = any self.view_slots  # view_slots has record(cal_day, slot, dow_idx)
db.BookAppointment(appt_id, chosen.cal_day, chosen.slot, chosen.dow_idx, self.__id__)

# Server uses it directly instead of recomputing
atomic func BookAppointment(appt_id, cal_day, slot_idx, dow_idx, cust_id):
    require dow_idx * SLOTS_PER_DAY + slot_idx in self.schedule
```

---

## 3. Choose Tight Symmetry Limits (Exponential Impact)

The `limit` parameter on symmetry domains controls how many distinct values
can exist simultaneously. Tighter limits = exponentially fewer states.

**Impact**: 4-9x state reduction per unit of limit reduction.

```python
# Too loose: allows 6 distinct days to coexist (most are unreachable)
DAYS = symmetry.interval(name="day", limit=6)    # 14,496 states

# Right: only need BOOKING_WINDOW distinct days
DAYS = symmetry.interval(name="day", limit=1)    # 3,648 states (4x reduction)
```

### How to Choose the Right Limit

The limit should be the **maximum number of distinct values that can coexist
in any reachable state**, not the total number of values ever created.

For interval symmetry, old values get garbage-collected (shifted away by
normalization), so the limit only needs to cover the "active window":

- **Unique IDs** (nominal): max number of live entities (e.g., active appointments)
- **Calendar time** (interval): booking window size (how far ahead you can book)
- **Ring positions** (rotational): number of distinct positions in the ring

### Garbage-Collect to Free Symmetric Slots

When symmetric values go out of scope (removed from all state), their slots
become available for reuse. Explicit cleanup helps:

```python
atomic func CleanupPast():
    self.appointments = [a for a in self.appointments if a.cal_day >= clock_day]
```

This frees interval slots for past days and nominal slots for past appointment IDs.

---

## 4. Merge Equivalent Phases (Linear Reduction)

If two phases/states have identical behavior from the model's perspective,
merge them into one.

**Impact**: ~25% state reduction per merged phase.

```python
# Before: 5 phases per day (PRE, SLOT_0, SLOT_1, SLOT_2, POST)
# PRE and POST are both "no slot active" - identical behavior

# After: 3 phases per day (BETWEEN, SLOT_0, SLOT_1)
# BETWEEN=0 covers both pre-first-slot and post-last-slot
```

The AdvanceClock wraps directly from the last slot phase to BETWEEN
on the next day, eliminating one phase per day cycle.

---

## 5. Use Symmetry Appropriately (Exponential Impact)

### When Symmetry Helps

Symmetry reduction collapses equivalent states. The right symmetry type
for each domain gives the most reduction:

| Domain | Symmetry Type | Why |
|:---|:---|:---|
| Unique IDs (UUID, PK) | `nominal` | Only identity matters, max reduction |
| Calendar time | `interval` | Distances matter, zero-shift normalization |
| Day of week | `rotational` or plain int | Wraps mod 7, but if anchored no reduction |
| Time within day | plain int | Boundary semantics break symmetry |
| Interchangeable actors | `symmetric role` | N! reduction for N instances |

### When Symmetry Doesn't Help

Don't use symmetry types just for their arithmetic convenience if they
won't actually reduce states:

```python
# Rotational for day-of-week: wraps automatically but if anchored,
# no actual reduction. Plain int + % is simpler and equivalent.
clock_dow = 0  # plain int
clock_dow = (clock_dow + 1) % DAYS_IN_WEEK  # manual wrap

# vs.
DOW = symmetry.rotational(name="dow", limit=DAYS_IN_WEEK)
clock_dow = DOW.choose()  # anchored: no reduction, adds complexity
```

### `symmetric role` + `bag()` for Dynamic Actors

When actors can be dynamically created and removed, use symmetric roles
with bags. Symmetry means removed-then-recreated actors map to the same
canonical states, avoiding the need for a hire/create cap:

```python
symmetric role Employee:
    action Init:
        self.status = "active"

# Non-symmetric: each Employee#N is distinct → needs MAX_HIRES cap
# Symmetric: removed employees are interchangeable with new ones → no cap needed
```

---

## 6. Single-User App Optimization

For modeling single-user apps (one human at the keyboard):

```yaml
---
deadlock_detection: false
options:
    max_concurrent_actions: 1
---
```

`max_concurrent_actions: 1` means only one action runs at a time,
which matches single-user interaction. This dramatically reduces
interleavings compared to the default.

`deadlock_detection: false` is usually needed because the user can
always choose to stop interacting (which looks like a deadlock to
the model checker).

---

## Summary: Impact Ranking

| Technique | Impact | Type |
|:---|:---|:---|
| Tight symmetry limits | 4-9x per unit | Exponential |
| Symmetry reduction | N! for N actors | Exponential |
| Merge equivalent phases | ~25% per phase | Linear |
| List comprehensions over loops | ~35-45% nodes | Constant multiplier |
| Reduce statement count | ~10-30% nodes | Constant multiplier |
| `max_concurrent_actions: 1` | Large | Configuration |
