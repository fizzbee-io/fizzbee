# FizzBee Spec Verification Guide

How to sanity-check that a specification is correct, beyond just looking at
"PASSED" and state counts.

---

## The Problem

A spec can pass model checking yet still be wrong: it might model the wrong
thing, miss important behaviors, or have overly tight constraints that
silently prune valid states. With thousands of states, manually inspecting
the full state space is infeasible.

## Verification Workflow

### 1. Run a Simulation to Get a Sample Trace

Simulation mode explores a single random path through the state space.
It's fast and produces a small, readable trace.

```bash
# Single run with a fixed seed (reproducible)
./fizz -x --max_runs 1 --seed 42 path/to/spec.fizz

# Output goes to: path/to/out/run_<timestamp>/graph.dot
```

Extract the link names (transitions) from the trace:

```bash
grep -o 'label="[^"]*"' path/to/out/*/graph.dot | sed 's/.*label="//;s/"//'
```

This gives a sequence like:
```
Employee#0.ToggleScheduleSlot
Any:schedule_code=0[ToggleSchedule.call]
Customer#0.Refresh[RefreshView.call]
Customer#0.BookSlot
Any:chosen=record(cal_day = day0, dow_idx = 0, slot = 0)[BookAppointment.call, RefreshView.call]
AdvanceClock
AdvanceClock
AdvanceClock[CleanupPast.call]
```

**Try several seeds** to find traces that exercise interesting behaviors:

```bash
for seed in 1 2 3 7 13; do
    ./fizz -x --max_runs 1 --seed $seed spec.fizz 2>/dev/null
    trace=$(grep -o 'label="[^"]*"' path/to/out/*/graph.dot | sed 's/.*label="//;s/"//')
    has_book=$(echo "$trace" | grep -c "BookSlot")
    has_cancel=$(echo "$trace" | grep -c "CancelMy")
    echo "seed=$seed: Books=$has_book Cancels=$has_cancel"
done
```

### 2. Read the Trace and Check It Makes Sense

Read through the transition sequence and ask:
- Are the expected actions happening? (bookings, cancels, schedule changes)
- Is the clock advancing properly?
- Do the `Any:` choices show reasonable values?
- Is cleanup happening at the right time?

Convert to SVG for visual inspection:
```bash
dot -Tsvg path/to/out/*/graph.dot -o graph.svg && open graph.svg
```

The SVG shows the full state at each node, including all role fields.
You can trace exactly how `appointments`, `view_slots`, `clock_phase`, etc.
change at each step.

### 3. Write a Guided Trace to Test Specific Scenarios

Guided traces let you force a specific sequence of transitions.
Use them to test edge cases that random simulation might not hit.

```bash
./fizz --trace "Employee#0.ToggleScheduleSlot
Any:schedule_code=0
Customer#0.Refresh
Customer#0.BookSlot
Any:chosen=record(cal_day = day0, dow_idx = 0, slot = 0)
AdvanceClock
AdvanceClock
AdvanceClock" path/to/spec.fizz
```

**See what comes next with `--trace-extend`:**

Add `--trace-extend 1` to see all available actions at the end of your trace:

```bash
./fizz --trace "Employee#0.ToggleScheduleSlot
Any:schedule_code=0
Customer#0.Refresh" --trace-extend 1 path/to/spec.fizz
```

This follows the 3 guided steps, then fans out to show every enabled
action at that point (e.g., BookSlot, AdvanceClock, Refresh, ToggleSchedule).
The graph shows the trace as a single path, then branches at the end.
Use `--trace-extend 2` to explore 2 steps ahead, etc.

This is useful for interactive exploration: run a trace, see what's
possible, pick one action, add it to the trace, and repeat.

**Trace format:**
- One transition per line
- Lines starting with `#` are comments
- Empty lines are skipped
- Init is included automatically (don't list it)
- Use just the action name prefix — the `[fn.call]` suffix from simulation
  output can be omitted

**Key scenarios to test:**
- Book → advance past appointment → verify cleanup
- Book → cancel → verify slot reopens
- Two customers try to book same slot (race condition)
- Schedule change → verify existing bookings unaffected
- Stale view → attempt booking after clock advanced

**What to look for:**

If a trace is **incomplete** (not all links consumed), you get:
```
WARNING: Trace execution incomplete. Expected 8 links, executed 7 links.
```

This means a transition was blocked — the most common cause is a `require`
guard failing or a symmetry limit being exceeded. This is how the
`limit=BOOKING_WINDOW` bug was caught: AdvanceClock was blocked because
`clock_day + 1` exceeded the interval limit while old appointment
references still existed.

### 4. Use a Trace File for Complex Scenarios

For longer traces, use a file:

```bash
./fizz --trace-file mytrace.txt path/to/spec.fizz
```

**mytrace.txt:**
```
# Enable schedule slot 0
Employee#0.ToggleScheduleSlot
Any:schedule_code=0

# Customer sees the slot and books it
Customer#0.Refresh
Customer#0.BookSlot
Any:chosen=record(cal_day = day0, dow_idx = 0, slot = 0)

# Time passes beyond the appointment
AdvanceClock
AdvanceClock
AdvanceClock

# Customer refreshes — should see appointment gone
Customer#0.Refresh
```

### 5. Test Multiple Configurations with `--preinit-hook`

Model checking explores **all** paths, so state spaces grow exponentially
with config size. Often you can't verify the full realistic config
(7-day week, 12 slots, 10 customers) via exhaustive model checking.

The `--preinit-hook` flag overrides constants without editing the spec file.
This lets you test different configurations that each verify a different
aspect of correctness:

```bash
# Override constants inline
./fizz --preinit-hook "DAYS_IN_WEEK = 3
BOOKING_WINDOW = 2
NUM_CUSTOMERS = 1" spec.fizz

# Or from a file (.cfg convention)
./fizz --preinit-hook-file config_race.cfg spec.fizz
```

The hook runs after global constants and symmetry domain definitions, but
before `action Init`. You can override any constant, including symmetry
domains:

```bash
# Override a plain constant
./fizz --preinit-hook 'MAX_ITEMS = 5' spec.fizz

# Override a symmetry domain
./fizz --preinit-hook 'IDS = symmetry.nominal(name="id", limit=5)' spec.fizz
```

All builtins (`symmetry`, `record`, `enum`, `bag`, `math`, etc.) are
available in the hook context.

**Strategy: verify different properties with different configs.**

Full model checking verifies **all** reachable states, but each dimension
multiplies the state space. Instead of one huge config, use several small
ones that each isolate a concern:

| Config | What it verifies |
|:-------|:-----------------|
| 1 customer, 2 slots, 2 days | Schedule/time logic, cleanup correctness |
| 2 customers, 1 slot, 1 day | Double-booking race condition |
| 1 customer, 1 slot, 2-day window | Multi-day booking, clock advancement |
| 2 customers, 2 slots, 2-day window | Full interaction (if tractable) |

```bash
# Verify double-booking with 3 customers on 1 slot
./fizz --preinit-hook-file config_race.cfg spec.fizz

# Verify clock/cleanup with wider schedule
./fizz --preinit-hook-file config_multiday.cfg spec.fizz
```

Config files use `.cfg` extension and live alongside the spec:

**Strategy: simulate with realistic configs.**

Simulation explores a single random path — no state explosion. Use
`--preinit-hook` with full-sized configs to sanity-check that the
spec works at realistic scale:

```bash
# Realistic salon: 7-day week, 12 slots, 10 customers
./fizz -x --max_runs 1 --seed 42 --preinit-hook-file config_large.cfg spec.fizz
```

In simulation mode, symmetry limits are disabled, so config files can
freely override constants like `BOOKING_WINDOW` that affect symmetry
domain limits in the spec.

Check the trace for expected behaviors (bookings, cancellations, clock
advances, cleanup). This catches issues that only manifest at scale —
for example, schedule codes wrapping around with 7-day weeks, or
appointment limits being hit with many customers.

**Combining with guided traces:**

```bash
# Test a specific scenario with a different config
./fizz --preinit-hook-file config_multiday.cfg --trace "Employee#0.ToggleScheduleSlot
Any:schedule_code=0
Customer#0.Refresh
Customer#0.BookSlot" --trace-extend 2 spec.fizz
```

### 6. Reduce Config for Debugging

When investigating a specific issue, reduce the spec to minimal config:
- 1 customer instead of 2
- 1 slot per day instead of 2
- 1 day week instead of 2
- Smallest possible symmetry limits

This makes the full state space small enough to visualize (< 250 nodes
generates graph.dot automatically) and makes traces short enough to read.

Use `--preinit-hook` to reduce config without editing the spec:

```bash
./fizz --preinit-hook "NUM_CUSTOMERS = 1
SLOTS_PER_DAY = 1
DAYS_IN_WEEK = 1" spec.fizz
```

### 7. Visualize Small State Spaces

For specs with < 250 nodes, fizzbee auto-generates `graph.dot`:

```bash
dot -Tsvg path/to/out/*/graph.dot -o graph.svg && open graph.svg
```

The graph shows all reachable states and transitions. Look for:
- Dead ends (nodes with no outgoing edges) — potential deadlocks
- Self-loops (same state repeated) — potential livelocks
- Missing transitions — actions that should be enabled but aren't
- Unexpected branching — actions creating more states than expected

### 8. Guard Against Unbounded State

If your spec passes model checking under `max_actions`, that does **not** prove
the state space is finite. The cap may be silently clipping an infinite path.

**Possible symptoms of a hidden unbounded counter** (any one is suggestive,
none is required):
- The model checker runs out of memory, or `queued` keeps growing under BFS.
- The BFS `queued` count climbs steadily then drops to 0 (consistent with
  `max_actions` clipping rather than natural exhaustion — but BFS can also
  drain gradually in a true finite spec, so this is a hint, not proof).
- Total node count keeps growing as you raise `max_actions`.
- Some quantity that "should" be bounded by your design isn't structurally
  guaranteed by any `require`.

**The technique: bound assertions.**

For every counter, accumulator, or collection you believe is bounded, add an
`always` assertion encoding that bound — with a small safety multiplier
(e.g., 2× the expected max) so a slightly-higher-than-expected natural max
(off-by-one, interleaving slack, retry overhead) doesn't flag false positives:

```python
BOUND_SLACK = 2

# Per-role counter
always assertion WriterCountersBounded:
    for w in writers:
        if w.writes > MAX_WRITES_PER_WRITER * BOUND_SLACK:
            return False
    return True

# Collection size
always assertion ObjectCountBounded:
    return len(objects) <= NUM_WRITERS * MAX_WRITES_PER_WRITER * BOUND_SLACK

# Monotonic ID / sequence number
always assertion ObjectIdsBounded:
    if len(objects) == 0:
        return True
    return max(objects.keys()) <= NUM_WRITERS * MAX_WRITES_PER_WRITER * BOUND_SLACK

# Pending-work queue / buffer
always assertion InboxBounded:
    for n in nodes:
        if len(n.inbox) > MAX_INFLIGHT * BOUND_SLACK:
            return False
    return True

# Map size (e.g., per-key history)
always assertion HistorySizeBounded:
    for k in history:
        if len(history[k]) > MAX_VERSIONS * BOUND_SLACK:
            return False
    return True
```

Then run simulation first (cheap, often finds violations in seconds):

```bash
./fizz -x --max_runs 1000 spec.fizz
```

**Reading the result:**

- **Assertion fires** → there's a *likely* unboundedness. Either the bound
  expectation is wrong (the structural max is higher than you thought —
  loosen the assertion), or the spec really is unbounded (find the loop in
  the failure trace and fix the spec).
- **Assertion doesn't fire** across many runs *and* model checking finishes
  at higher `max_actions` → evidence in favor of finiteness, but not a
  proof. Simulation can miss adversarial paths; consider whether you've
  bounded *all* the right quantities.

**Where to add bound assertions** — for every stateful variable (globals,
role fields, nested collections), ask:

- Is there a `require` that limits how often it's incremented / appended to?
- Can the action be re-enabled after the body runs, without that re-enable
  being itself bounded?
- Is the body's effect bounded by something *already* bounded (objects size,
  role count), or by something else that itself can grow?

If you can't trace it back to "bounded by X" where X is a constant or
another bounded quantity, encode a bound assertion.

---

## Summary: Verification Checklist

1. **Simulation**: `./fizz -x --max_runs 1 --seed N spec.fizz`
   - Try 3-5 seeds, check traces hit expected behaviors
2. **Guided trace**: `./fizz --trace "..." spec.fizz`
   - Write traces for key scenarios (happy path, edge cases, race conditions)
   - Check for "incomplete" warnings
3. **Config variants**: `./fizz --preinit-hook "..." spec.fizz`
   - Small focused configs for model checking (isolate each concern)
   - Large realistic configs for simulation (sanity check at scale)
4. **Minimal config**: Reduce parameters for visual inspection
   - Visualize with `dot -Tsvg graph.dot`
5. **Full model check**: `./fizz spec.fizz`
   - Verify PASSED with assertions
   - Compare state counts across configs for sanity
6. **Bound assertions** for any counter / collection you believe is bounded
   - `BOUND_SLACK = 2` multiplier; simulation often trips these in seconds
   - Assertion fires → likely unbounded (or your bound was wrong)
   - PASSED with bounds intact across higher `max_actions` → evidence of finiteness, not proof
