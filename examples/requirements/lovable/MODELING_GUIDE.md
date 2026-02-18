# Modeling Lovable Apps in FizzBee

Guide for writing formal specifications of apps built with Lovable
(React frontend + Supabase/Postgres server).

---

## Architecture Mapping

| Lovable Component | FizzBee Model |
|:---|:---|
| Supabase database | `role Server` (single instance, ground truth) |
| User's browser | `role User` or `symmetric role Customer` (view + actions) |
| React component state | `self.view_*` fields on user roles |
| Supabase table row | Record in a list on the Server role |
| Row-level security (RLS) | `require` guards in Server functions |
| Supabase realtime | Not modeled — user refreshes manually (stale views) |

### Why Model Stale Views?

In Lovable apps, the browser fetches data and caches it locally. Between
fetches, other users (or the clock) can change the database. The spec
models this explicitly:

1. **RefreshView**: Reads current database state into `self.view_*`
2. **User action**: Reads from `self.view_*` (potentially stale)
3. **Server function**: Validates against current database state with `require`

This pattern catches race conditions where two users see the same slot
as available but only one can book it.

---

## Spec Structure

```python
---
deadlock_detection: false
options:
    max_concurrent_actions: 1
---

# Constants and symmetry domains
# ...

role Server:
    action Init:
        # Initialize database tables as lists/sets/dicts
    # Atomic functions for database operations (INSERT, UPDATE, DELETE)
    # Each function validates with require guards (like RLS policies)

symmetric role User:
    action Init:
        # Initialize view state
    atomic func RefreshView():
        # Read from db.* into self.view_*
    atomic action SomeUserAction:
        # Pick from self.view_*, call db.SomeOperation(), refresh view

# Clock (if time matters)
atomic fair action AdvanceClock:
    # Advance time, clean up past data

action Init:
    db = Server()
    users = bag()
    for i in range(NUM_USERS):
        users.add(User())

always assertion SomeInvariant:
    # Check database consistency
```

### Key Settings

- **`deadlock_detection: false`**: Users can always stop interacting
- **`max_concurrent_actions: 1`**: Single-user interaction at a time
  (even with multiple user roles, only one acts per step)

---

## Common Patterns

### Database Tables as Lists

Use lists when insertion order matters (e.g., `ORDER BY created_at`):

```python
self.items = []
self.items = self.items + [record(id=item_id, name=name, created_by=user_id)]
```

Use sets for unordered unique values:

```python
self.schedule = set()
self.schedule.add(schedule_code)
self.schedule.discard(schedule_code)
```

### Stale View → Server Validation

The core race condition pattern:

```python
symmetric role Customer:
    atomic func RefreshView():
        self.view_slots = [s for s in db.available_slots if is_valid(s)]

    atomic action BookSlot:
        chosen = any self.view_slots          # Pick from stale view
        db.BookSlot(chosen, self.__id__)      # Server validates
        self.RefreshView()                     # Refresh after action

role Server:
    atomic func BookSlot(slot, customer_id):
        require slot in self.available_slots   # Might fail if stale
        self.available_slots.remove(slot)
        self.bookings.append(record(slot=slot, customer=customer_id))
```

### Dynamic Actors (Hire/Fire Employees)

Use `symmetric role` + `bag()` for actors that can be added and removed:

```python
symmetric role Employee:
    action Init:
        self.status = "active"

role Owner:
    atomic action HireEmployee:
        emp = Employee()
        employees.add(emp)
        db.ActivateEmployee(emp.__id__)

    atomic action FireEmployee:
        emp = any employees
        db.DeactivateEmployee(emp.__id__)
        employees = bag([e for e in employees if e.__id__ != emp.__id__])
```

No hire cap needed — symmetry means removed-then-recreated actors map
to the same canonical states.

### Time Modeling

When appointments, deadlines, or expiration matter:

```python
DAYS = symmetry.interval(name="day", limit=BOOKING_WINDOW + 1)

# +1 slack because AdvanceClock temporarily creates a 2nd day value
# (clock_day + 1) before cleanup removes old references.

atomic fair action AdvanceClock:
    if clock_phase < SLOTS_PER_DAY:
        clock_phase = clock_phase + 1
    else:
        clock_day = clock_day + 1
        clock_dow = (clock_dow + 1) % DAYS_IN_WEEK
        clock_phase = 0
        db.CleanupPast()  # Free interval slots for garbage-collected data
```

**Why `limit = BOOKING_WINDOW + 1`?** When `AdvanceClock` increments
`clock_day`, it temporarily creates a new day value. Old references
(appointments, cached views) still point to the old day. With `limit=N`,
at most N distinct interval values can coexist. The +1 accommodates
this transient state before cleanup runs.

### Unique IDs

Use nominal symmetry for database primary keys:

```python
APPT_IDS = symmetry.nominal(name="appt", limit=3)
appt_id = APPT_IDS.fresh()
```

The limit should be the max number of live records, not total ever created.
Garbage-collect old records to free slots.

### Cross-Role Function Returns

`self.x = otherRole.fn()` crashes. Use a local variable:

```python
# CRASHES
self.cached = db.ReadDoc(doc_id)

# WORKS
content = db.ReadDoc(doc_id)
self.cached = content
```

---

## Performance Tips for Lovable Specs

1. **List comprehensions over loops**: Each statement creates nodes.
   Collapse nested loops and if-chains into single comprehensions.

2. **`any` blocks on empty**: No need for `require len(xs) > 0` before `any xs`.

3. **`require all([...])`** instead of for-loop with require.

4. **Tight symmetry limits**: `limit` = max coexisting values, not total ever.

5. **Merge equivalent phases**: If PRE and POST have identical behavior,
   combine into single BETWEEN phase (~25% state reduction).

6. **Garbage-collect**: Remove expired records to free symmetry slots.

See [Performance Guide](../../references/PERFORMANCE_GUIDE.md) for details.

---

## Verification Workflow

Don't just check "PASSED" — verify the spec models the right thing.

### 1. Simulate and Read Traces

```bash
./fizz -x --max_runs 1 --seed 1 spec.fizz
grep -o 'label="[^"]*"' path/to/out/*/graph.dot | sed 's/.*label="//;s/"//'
```

Check: Are bookings, cancels, schedule changes happening? Is time advancing?

### 2. Write Guided Traces for Key Scenarios

```bash
./fizz --trace "Employee#0.ToggleScheduleSlot
Any:schedule_code=0
Customer#0.Refresh
Customer#0.BookSlot
Any:chosen=record(cal_day = day0, dow_idx = 0, slot = 0)
AdvanceClock
AdvanceClock
AdvanceClock
Customer#0.Refresh" spec.fizz
```

If a step is blocked, you get **"Trace execution incomplete"** — investigate why.

### 3. Reduce Config for Debugging

```python
DAYS_IN_WEEK = 1
SLOTS_PER_DAY = 1
BOOKING_WINDOW = 1
NUM_CUSTOMERS = 1  # or remove symmetric role, use single role
```

Small configs generate visual graphs (< 250 nodes).

### 4. Visualize

```bash
dot -Tsvg path/to/out/*/graph.dot -o graph.svg && open graph.svg
```

See [Verification Guide](../../references/VERIFICATION_GUIDE.md) for the full workflow.

---

## Examples

| # | App | Key Features | States |
|:--|:----|:-------------|:-------|
| 01 | Todo | CRUD, reordering | 52 |
| 02 | Poll | Multi-option voting, toggle | 72 |
| 03 | Booking | Time slots, double-booking prevention | 94 |
| 04 | Kanban | Drag-drop columns, WIP limits | 26 |
| 05 | Store | Cart, inventory, checkout | 308 |
| 06 | Doc Permissions | Owner/viewer/editor, access control | 28 |
| 07 | Approval | Multi-stage workflow, roles | 56 |
| 08 | Shopping List | Collaborative editing, real-time sync | 202 |
| 09 | Salon Booking | Dynamic employees, symmetric roles | 1,012 |
| 10 | Salon Schedule | Recurring schedules, time passage, interval symmetry | 14,496 |
