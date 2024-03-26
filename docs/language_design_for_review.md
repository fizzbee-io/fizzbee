# TLA+ to Fizz

## Next State and Primed Variables

In TLA+, next states are denoted using primed variables, e.g., `count' = count + 1`.
To enhance clarity for programmers unfamiliar with TLA+, Fizz introduces explicit names for
current and next states—`this` and `next` respectively. This results in a more readable
notation: `next.count = this.count + 1`.

While TLA+ treats the `=` operator as commutative, TLC (the TLA+ model checker) does not.
As a result, next states must always be on the left side of the equation. However, Fizz
simplifies this by eliminating the need for explicit `this` and `next` separation in common
cases. For example, `count = count + 1` now succinctly represents the same concept in Fizz.

```fizz
# TLA+ notation
count' = count + 1

# Fizz explicit notation
next.count = this.count + 1

# Simplified Fizz notation
count = count + 1

```

This syntax is familiar to almost every software engineer.

## State Variables

TLA+ requires explicit variable declaration followed by initialization using
an `Init` action. Fizz simplifies this process by introducing an implicit
declaration during initialization, following a straightforward convention.

State variables in Fizz are succinctly defined within a dedicated section:

```fizz
# Fizz state variables
state:
  count = 0
  list = []
```

## Non-atomic actions

In TLA+, every action is atomic. In PlusCal, we can make the statements
atomic or serial by assigning labels to block of statements.

This implies engineer's have to explicitly model the errors,
making it challenging to realistically model distributed systems.
Fizz addresses this by making failures the norm. And then introducing
explicit labels for the order of execution, making it more aligned with
common distributed systems scenarios.

These are my observations designing distributed systems:

* **Murphy's law is real.** Anything that can go wrong will go wrong.
* **Non-Atomic Operations**: Many operations in distributed systems are non-atomic
* **Sequential Steps**: Commonly, operations follow a sequential pattern, and any step can fail. For example
    * Write a message to a DB
    * Publish an event
* **Parallel Operations**:. They can be
    * Explicit like make multiple IO operations in parallel
    * Implicit like publish an event, and two separate services listen to the events
      and process them or update the DB.
* Obviously, some operations are atomic.

Consider an example where we need to model an object counter. In this scenario,
an object is written to a persistent object store, and then, a counter is updated
in a different key-value store.

TLA+:

```
Add(b) == 
  /\ blobs' = blobs \union {b}
  /\ count' = count + 1
```

With this model, count will actually always match the blobs count because in TLA+ every
action is atomic. However, in reality, the count update could fail. Without explicit consideration
by the programmer, this leads to a potentially buggy implementation.

To account for the failure case, the programmer must explicitly write:

```
Add(b) == 
  \/ /\ blobs' = blobs \union {b}
     /\ count' = count + 1
  \/ /\ blobs' = blobs \union {b}
     /\ UNCHANGED <<count>>
```

In this formulation, blobs are written first, and then the count is updated.
The count update might fail - so left UNCHANGED.

Recognizing that this is a common scenario, in Fizz, we default to a serial order.

### Serial

In Fizz:

```
add(b):
  # A block of statements can be labeled as serial,
  # then we will automatically test the cases where only some
  # steps succeeded.
  # serial is the default, so this can be ignored.
  serial:
    blobs.add(b)
    count = count + 1
    
add_alternative(b):
  # serial is the default
  blobs.add(a)
  count = count + 1
```

### Atomic

If the intention is to make the operation atomic, for instance, when utilizing
transactions on the same database, the block can be labeled as atomic in Fizz.
Here's how it would look:

```
add(b):
  atomic:
    blobs.add(b)
    count = count + 1
```

In this model, if the update to blobs is successful,
the count will also be updated atomically.

### Parallel

If the scenario requires parallel execution, where blobs and count can be
updated simultaneously, the block can be labeled as parallel in our new language.
Here's the equivalent Fizz representation:

```
add(b):
  parallel:
    blobs.add(b)
    count = count + 1
```

In this setup, there is a chance that the count is updated successfully, but updating blobs fails since these steps
occur in parallel.

For comparison, the TLA+ equivalent of this parallel scenario is:

```
Add(b) == 
  \/ /\ blobs' = blobs \union {b}
     /\ count' = count + 1
  \/ /\ blobs' = blobs \union {b}
     /\ UNCHANGED <<count>>
  \/ /\ count' = count + 1
     /\ UNCHANGED <<blobs>>
```

### Oneof:

In Fizz, the equivalent of TLA+ conjunct (`/\`), representing an atomic operation,
is denoted by the label atomic. Similarly, the equivalent of TLA+ disjunct (`\/`)
is labeled as **`oneof`**.

For instance, the parallel scenario in TLA+ can be expressed in Fizz using oneof:

```
add(b):
  # Equivalent of parallel with just oneoff and atomic.
  oneof:
    atomic:
      blobs.add(b)
      count = count + 1
    atomic:
      blobs.add(b)
    atomic:
      count = count + 1      
      
# since there is only a single statement, atomic can be ignored.
add_alternative(b):
  oneof:
    atomic:
      blobs.add(b)
      count = count + 1
    blobs.add(b)
    count = count + 1   
```

## Implicit UNCHANGED

Unlike TLA+, Fizz assumes that if there is no specified next state transition,
variables remain unchanged. Therefore, explicit declarations of `UNCHANGED`
are unnecessary in Fizz, simplifying the syntax.

## Universal(\A) Quantifier

In Fizz, the universal quantifier `\A` in TLA+ is succinctly represented using
the Python keyword `for`. For instance:

TLA+

```
\A n \in nodes:
  n' = [n EXCEPT !.status = 'done']
```

Fizz

```
for n in nodes:
  n.status = 'done'
```

This Pythonic syntax is intuitive even to non-Python programmers.

## Existential(\E) Quantifier

There is no equivalent for the existential qualifier `\E` in python.
In Fizz, we introduce the `any` keyword as an equivalent to the existential
quantifier \E in TLA+. The `any` keyword is syntactically similar to `for`.
The primary difference is it creates separate branches for each element
in a list or set. For example:

TLA+

```
\E r \in records:
  records' = records \ {r}
```

Fizz

```
any r in records:
  # alternately records.remove(r)
  records = records - {r}
```

This simplifies the representation of existential quantification in Fizz,
providing a clear and intuitive syntax.

## Implicit Spec and Next State Actions

In Fizz, all actions now start with the keyword `action`, akin to `def`
for Python functions. This design eliminates the need for a separate
"Next State" or "Spec" section.

> Should we remove the action keyword as well?

```
action Add:
  # ...
  
action Remove:
  # ...  
```

Each of these will be part of the next state actions.

## Fairness

> What would be a good keywords to specify to differentiate
> between strong and weak fairness.

For now, in each action, you can add a modifier. `weak` or `strong`

```
action FirstAction:
  # ... unfair action

weak<var1, var2> action SecondAction
  # action with weak fairness
  # WF_<<var1,var2>>(Next)
  # like fair process of PlusCal 
    
weak action ThirdAction:
  # action with weak fairness, but all 
  # declared state variables
  # like fair process of PlusCal
  
  
strong action ForthAction:
  # ... action with strong fairness
  # similar to fair+ of PlusCal
  
```

### Alternatives being considered for defining fairness

1. Using fair Keyword:
   ```
   fair<var1, var2> SecondAction:
   # ... (Action with weak fairness)
   
   fair<strong=true, var1, var2> ForthAction:
   # ... (Action with strong fairness)
   ```
2. Using fair keyword, but required weak/strong when variables specified.
   That is, the grammar would look like

   `[ 'fair' [ '<' 'weak'|'strong' [, var]*'>' ] ] 'action' `

   ```
   fair<weak, var1, var2> action SecondAction
     # action with weak fairness
     # WF_<<var1,var2>>(Next)
     # like fair process of PlusCal
    
   fair action ThirdAction:
     # action with weak fairness, but all
     # declared state variables
     # like fair process of PlusCal

   fair<strong> action ForthAction:
     # ... action with strong fairness
     # similar to fair+ of PlusCal
   ```
3. Using fairness Parameter within Action
   ```
   action<fairness=weak, var1, var2> SecondAction:
     # ... (Action with weak fairness)
   ```

## Python functions

In Fizz, most built-in Python functions are available for use,
providing familiarity and flexibility. Additional functions can
be easily added as needed.

However, for the initial implementation, Fizz utilizes the Starlark
language instead of standard Python. This decision offers advantages
in terms of security and hermeticity. While some modules may not be
importable in Starlark, it provides a secure and controlled environment.

For now, this limitation is not a major concern as essential Python functions
are accessible, and Fizz is being equipped with a repository of reusable libraries.
Future iterations may consider transitioning to standard Python if deemed necessary.

## Imports

In Fizz, import statements will follow the syntax style of the **Go** language.
Additionally, imports can include other **Fizz** files or other **Starlark** files.
All Starlark/Python functions will be treated as atomic by default

## Roles

Roles in Fizz provide a way to organize components within a large distributed system,
analogous to a class in object-oriented programming but at a higher system level.
Roles can represent microservices, databases, distributed caches, or subcomponents
of a monolith, among other things.

For example, in a two-phase commit, the coordinator (transaction manager)
and the participants (resource managers) are different roles.
In practice, each participant could act as a coordinator, and they can be
part of the same service or process.

As a convenience, every module is inherently a role or a specific role type.

Within the same module or .fizz file, you can define the role with
keyword `role`.

```
# Coordinator
role TransactionManager {
  state:
    # state variables
  
  # Action definitions
}

# Participant
role ResourceManager(rm) {
  state:
    # state variables
  
  # Action definitions
}
```

To instantiate roles within the same module:

```
constants:
  RM

state:
  resMgrs = {ResourceManager(rm) for rm in RM}
  transMgr = TransactionManager()

```

## Channel (Messaging channel)

A channel is the mechanism to connect two roles. This simplifies
modeling of message passing.

* Blocking/NonBlocking
* Delivery (atmost once, at least once, exactly once)
* Ordering (unordered, pairwise, ordered)

For now, we will only support blocking semantics, with non-blocking
simulated using two separate actions for request and response.

### Default

#### Intra-role call:

Since calls between roles are usually in-memory call, the default
will be reliable. `blocking exactlyonce ordered`

#### Inter-role call:

Inter role calls are usually some kind of message passing -
either blocking (RPC) operation or non-blocking (Message Queues)
so these are unreliable. We will default to rpc semantics.
`blocking atmostonce unordered`

# Alternative considered

## Separate Inputs, Guard clauses and post actions

TLA+ does not separate guard clauses (preconditions) or inputs
separately. Everything is just another state assertion.
Many other formal languages separate preconditions, state
transitions, and some even separate inputs (For example: Event-B).

Separating precondition vs state transition has a significant
advantage for readability. And it also makes the implementation
a lot simpler.

```
# Option: NOT PLANNED for now, unless users prefer this.

# TLA+ equivalent of
# Count == count < 10 /\ count' = count + 1

action Count:
  pre:
    count < 10
  next:
    count = count + 1

# Current proposal
action Count:
  if count < 10:
    count = count + 1

```

The drawback is, it reduces expressiveness for many cases. Especially,
those actions that use Universal (`\A`) and Existential (`\E`)
qualifiers.

For example: Notify all subscribers in pending state to done.
TLA+

```
NotifySubscribers ==
  \A s \in subscribers:
    /\ status[s] == "pending"
    /\ status' = [status EXCEPT ![s] = "done"]

```

Fizz

```
action NotifySubscribers:
  for s in subscribers:
    if status[s] == 'pending'
      status[s] = 'done'
```

In this case, action NotifySubscribers is not ENABLED if no subscribers are in pending state.
However, this would become a lot more verbose to separate as precondition.

```
action NotifySubscribers:
  pre:
    len({sub,status for sub,status in subscribers if status=="pending"})>0
  next:
    for s in subscribers:
      if status[s] == 'pending'
        status[s] = 'done'
```

Note: As the implementation of model checker is a lot simpler if we can separate
guard clauses.

Also, not separating preconditions is actually how programmers
typically program since no major programming language does it.
For cases where preconditions seem natural, they could simply handle
it with if block at the top.

# Inputs to actions

Separating inputs is actually very intuitive for programmers.
This is mostly specified in the RPC specification.

For example: To remove a document from stored documents.
TLA+

```
Remove == 
  \E d in documents:
    documents' = documents \ {d}
```

Fizz

```
# Fizz
action Remove:
  any d in documents:
    documents.remove(d)

```

Alternatives being considered:

```
# option1: Similar to event-b
action Remove:
  input:
    any d in documents:
  next:
    documents.remove(d)

# option2 
action Remove(any d in documents):
  documents.remove(d)
  
# option3 
# read as d such that d is in documents.
action Remove(d in documents):
  documents.remove(d)
```

An example combining all these actions:

```
# option 1:
action Add:
  input: 
    any d in ALL_DOCUMENTS # Model value; set of all documents
  pre:
    d not in documents
  next:
    documents.add(d)

# option 2: separate input no separate guard clause
# This makes the grammer a bit harder
action Add(any d in ALL_DOCUMENTS):
    if d not in documents:
      documents.add(d)
```
