# Compositional Model Checking

This document outlines the compositional model checking techniques planned for FizzBee. It aims to give a high-level overview of each technique and its intended use.

**Feedback welcome on:**
- correctness of logic and tradeoffs
- proposed techniques
- syntax/grammar
- suggestions or missing techniques

---

# Introduction

FizzBee is an explicit-state model checker. As the number of roles and actions grows, the state space can explode, making it difficult to analyze large systems.

To address this, we explore *compositional model checking*: breaking a large system into smaller, loosely coupled parts that are individually verified and then combined. This mirrors how engineers design complex systems, while retaining formal rigor.

Other scaling techniques (not covered here) include:

- **Symbolic model checking**: Uses symbolic representations to reduce state space size. But requires restrictive syntax, which we avoid to keep FizzBee accessible.
- **Simulation**: Already supported via `--simulation`. Explores states randomly, useful for fast checks but not exhaustive.
- **Distributed model checking**: Will be implemented later. Explores the state space in parallel across machines.

We aim to make formal verification accessible without requiring users to be formal methods experts. So the composition techniques and the syntax chosen would prioritize usability over technical elegance.

---

# Compositional Model Checking Techniques
These are the 4 techniques I'm planning to implement in the listed order unless I get a different feedback on ease of use vs ease of implementation tradeoffs.

- State space composition
- Modular refinement
- Component refinement
- Component substitution

## State Space Composition

This is similar to an SQL inner join: combining component state spaces on shared variables.
For example: In specifying the Raft protocol, we can split the specs into leader election and log replication. Then, compose them together based on the shared variables like the leader, term, log etc.
It is easy for users to understand and for us to implement. Variants offer tradeoffs between precision and performance:

- **State Bucketing**:  
  Group states by shared variables (using a user-defined mapping). If `p` states from A and `q` states from B match, join them into `p × q` states.
    - ✅ Simple, fast, intuitive
    - ✅ *Complete* (no false negatives)
    - ❌ Not *sound* (can produce false positives)
    - ❌ No liveness checking; error traces are unclear

- **Transition Bucketing**:  
  Similar to state bucketing, but group transitions instead. Mapping is more complex, but may reduce false positives further.
    - ✅ Complete
    - ⚠️ Sound (with a caveat)
    - ⚠️ Liveness is supported but nuanced (no extra work for the users, though)
    - ⚠️ Slightly harder to implement and comprehend
    - ❌ Error traces will still be unclear

- **Interleaving Graph Traversal**:  
  Traverse component state graphs together, validating every step against both specs.
    - ✅ *Complete* and *sound*
    - ✅ Supports liveness checking and error traces
    - ❌ Slowest and most complex
    - ⚠️ Same time complexity as full spec, but with reduced memory and constant-factor speedup

Even if not faster in worst-case time, this technique improves modularity and understandability for users.

---

## Modular Refinement

[Refinement](https://en.wikipedia.org/wiki/Refinement_%28computing%29#Program_refinement) is the process of verifying that a lower-level, more detailed specification correctly implements a higher-level, abstract one.

Traditionally, this involves writing and verifying an abstract spec, then writing a detailed spec of the same system. But the detailed spec is often too complex to model check, and the abstract spec becomes less useful—so refinement is rarely used in practice.

In FizzBee, we'll implement *modular refinement*. The user first writes a high-level spec describing the system abstractly, verifying safety and liveness. For example, a Raft spec might specify that a leader is elected, clients send writes to the leader, and logs are replicated. Then, separate detailed specs (e.g., for leader election and log replication) can be written and verified to refine the corresponding parts of the high-level spec.

This reduces state space and mirrors how engineers design and understand systems.

This is related to **State Space Composition** in that both use separate component specs. However, instead of joining them, modular refinement verifies each one against the high-level spec. Conceptually, it proves that the composed system refines the abstract specification.

### Pros
- ✅ Mirrors how engineers naturally reason about systems.
- ✅ Smaller, more manageable state space for each part.
- ✅ High-level spec doubles as clear documentation.
- ✅ Detailed specs can focus only on local component logic.
- ✅ Sound and complete — no false positives or false negatives.
- ✅ Error traces are meaningful (with some learning curve).

### Cons
- ❌ Slightly more complex to implement in the tool.
- ❌ Refinement violation traces can be harder to interpret than assertion failures.

###  Notes
- ⚠️ Liveness checking is not automatically supported.
    - May be added later via interleaving graph traversal or bisimulation with little or no user effort.

## Component Refinement


This technique is similar to **modular refinement**, but instead of refining a horizontal feature (e.g. leader election vs. log replication), **component refinement focuses on refining a single component vertically**.

This is unique to FizzBee due to its built-in concept of **Roles**, which define the behavior of a single component in isolation. This refinement technique is conceptually similar to verifying the [Liskov Substitution Principle](https://en.wikipedia.org/wiki/Liskov_substitution_principle):

- Preconditions must not be strengthened
- Postconditions must not be weakened
- Invariants must not be weakened
- History constraints must be satisfied

In practice, this means:

We can write a high-level system specification where the role behavior is left intentionally abstract (e.g., the behavior of `Prepare` in a 2PC participant is unspecified — it can either prepare or abort arbitrarily). This high-level spec might include multiple instances of this role interacting with other roles.

Then, we can refine a single **Role** with more detailed behavior — for example, by defining conditions under which `Prepare` aborts, or by introducing internal timeouts or failure handling logic — and verify that this refined version **implements** the abstract one correctly. That is:

Given the same usage context — same externally visible function calls — does the refined role behave in a way that’s compatible with the abstract role? It may take more steps internally or do additional work, but it must not violate observable expectations.

### Example

In the [Two Phase Commit FizzBee example](https://fizzbee.io/design/examples/two_phase_commit_actors/#complete-code), the `Participant` role is defined abstractly with just two actions: `Prepare` and `Finalize`. The abstract version does not specify why `Prepare` might choose to abort or prepare.

We can then refine this role with a more specific implementation — say, one where the participant may time out before responding, or where it checks some internal state before deciding. As long as the refined behavior remains substitutable for the abstract one (i.e., satisfies its observable contract), the refinement is valid.

### Pros
- ✅ Simpler for users to understand and apply.
- ✅ Easy to implement within the FizzBee framework.
- ✅ Significantly reduces state space: only one role is model checked at a time, even if the abstract spec includes many instances.

### Cons
- ❌ Cannot add new global invariants based on the refined state variables
- ❌ Less flexible than full system refinement.
- ❌ Has nuanced edge cases that can be tricky to specify.

### Notes
- ⚠️ Handling of internal actions (e.g. `Timeout`) requires care:
    - For example, in the refined 2PC participant, if a `Timeout` occurs before `Prepare` is called, the state transitions before the abstract function is invoked.
    - In the abstract spec, such internal transitions are not modeled, so reconciling the two behaviors requires careful constraint modeling or design patterns.
- ⚠️ Global state access: FizzBee allows roles to access and modify global state, which simplifies modeling but introduces subtlety in ensuring behavioral equivalence during refinement


## Component Substitution

This is another technique unique to FizzBee, enabled by its built-in support for **Roles**. Unlike component refinement, which cannot assert global properties based on a substituted component, component substitution allows us to verify whether the system maintains new safety and liveness properties when certain roles are replaced with different implementations.

For example, consider the side panel of a two-player board game showing which player is active. Only one indicator is ON at a time. Suppose we refine this indicator into a countdown timer with pause/unpause functionality. Independently, we can verify properties of the timer (e.g., it reaches 0 only when unpaused).

With component substitution, we can validate that if the indicator is replaced with this timer role—where "active" corresponds to "unpaused"—the system still maintains properties like: only one timer reaches 0 at a time.

---

### Pros and Cons

✅ Extremely powerful and flexible  
❌ Too complex for most users to understand and use  
❌ Too complex to implement  
⚠️ The flexibility implies high specification complexity—for example, when multiple role instances are substituted with different implementations (e.g., one with version A, another with version B)


# Implementation Details

The details are not fully refined.

## State space composition

Let's take a simple but trivial example of a clock, and let us say we were supposed to track time in minutes and hours. So, we have two variables `minute` and `hour` (in 12 hour format). (This is a trivial example, could've been modeled with a single variable, but using this only to explain this approach). The trivial spec would have 720 states.
```
---
options:
    max_actions: 1000
---
MAX_MINUTES = 59
MAX_HOUR=12

action Init:
    minute = 0
    hour = MAX_HOUR

atomic fair action MinuteTick:
    if minute == MAX_MINUTES:
        minute = 0
        hour = (hour%MAX_HOUR) + 1
    elif minute < MAX_MINUTES:
        minute += 1
```
Instead, let us split them into two.
**minute.fizz**
```
MAX_MINUTES = 59
action Init:
    minute = 0

atomic fair action MinuteTick:
    if minute == MAX_MINUTES:
        minute = 0
    elif minute < MAX_MINUTES:
        minute += 1
```
and **hour.fizz**
```
MAX_HOUR=12
action Init:
    hour = MAX_HOUR

atomic action HourTick:
    hour = (hour%MAX_HOUR) + 1
```
Run these two in the [FizzBee Playground](https://fizzbee.io/play) You'll see a simple loop for both with 60 and 12 states respectively.

A naive cross-join in this case will produce the same 720 states, but it will imply a total cross-join. As in, the a state can transition from `2:35` -> `3:35` which is not a legal state transition in the combined spec.
To do that, let us split the hour states into two for each hour - where the clock system waits for an hour, and then ticks.

```
MAX_HOUR=12
action Init:
    hour = MAX_HOUR
    hour_elapsed = False

atomic action HourTick:
    require hour_elapsed
    hour = (hour%MAX_HOUR) + 1
    hour_elapsed = False

atomic action WaitAnHour:
    require not hour_elapsed
    hour_elapsed = True
```
When you run it in the playground, it will give 24 states, where, it alternates between (12, False) --WaitAnHour-->(12,True)--HourTick-->(1,False)--WaitAnHour-->(1,True)--...
You might have guessed, the `hour_elapsed=True` implies the minute reached `59` and if the minute `[0,59)` it is equivalent to `hour_elapsed=False`

Now, in the **minute.fizz** spec,
```

MAX_MINUTES = 59
action Init:
    minute = 0
    # This field is not really required as it is purely a derived field,
    # we will show how to remove this later
    hour_elapsed = False


atomic fair action MinuteTick:
    if minute == MAX_MINUTES:
        minute = 0
    elif minute < MAX_MINUTES:
        minute += 1

    if minute == MAX_MINUTES:
        hour_elapsed = True
    else:
        hour_elapsed = False
```
When you run this in the playground and see the state graph, you could see 60 states where the minute goes from 0..58 while `hour_elapsed=False`, then changes to `minute=59, hour_elapsed=True`, then it will switch to `minute=0, hour_elapsed=False`
Now, the `hour_elapsed` is the shared state. Imagine there are two SQL tables minutes(hour_elapsed, minute) and hours(hour_elapsed, hour), we could inner join these two tables to get

| hour_elapsed | hour | minute |
|--|--|--|
| False | 12 | 0 |
| False | 12 | 1 |
| False | 12 | 2 |
|   "   | " | ... |
| False | 12 | 58 |
|  **True** | **12** | **59** |
| False |  1 |  0 |
| False |  1 |  1 |
|   "   | " | ... |
| False | 1 | 58 |
|  **True** | **1** | **59** |
| False |  2 |  0 |

Here, you would see, the init state is (False, 12, 0), from there, the only next state available in both the specs are stutter at (False, 12), and (False, 1). When the minute is at (False, 58), the only next step is (True, 59) and for the hour spec, it also proceeds to (True, 12), so (True, 12, 59) is the only next step. Then, the next steps are (False, 0) for minute and (False, 1) for hour, so the next step is (False, 1, 0).

Now, we can make complete assertion on the combined fields not just the shared fields in this model. For example, this will pass the ClockProperty transition assertion

```
transition assertion ClockBehavior(before, after):
    if before.hour != after.hour:
        return before.minute == MAX_MINUTES and after.minute == 0
    else:
        return before.minute < MAX_MINUTES and after.minute == before.minute+1
```

### Proposed Syntax

```
# Import the dependent files with python like syntax
import path.to.minute.spec as minute_spec
import path.to.hour.spec as hour_spec

# Considering an SQL like syntax familiar to most engineers
join minute_spec,hour_spec 
	on minute_spec.hour_elapsed == hour_spec.hour_elapsed 

# We can remove the minute.hour_elapsed variable completely
join minute_spec,hour_spec 
	on (minute_spec.minute == 59) == hour_spec.hour_elapsed
	
# Each of these could be function calls as well
join minute_spec,hour_spec 
	on minute_spec.get_shared_state() == hour_spec.get_shared_state() 

# Can add safety assertions (both state and transition), for example
transition assertion ClockBehavior(before, after):
    if before.hour != after.hour:
        return before.minute == MAX_MINUTES and after.minute == 0
    else:
        return before.minute < MAX_MINUTES and after.minute == before.minute+1
```


Node, instead of `minute.hour_elapsed`, it could be even be a function call or any expression. The LHS and RHS will be evaluated independently for the State Bucket and Transition Bucket algorithms. (If we've to support other expression types, then only the Interleaved Graph Traversal algorithm would work)

Here, the syntax must exactly be of the form left side something `==` right side something. We might only support `==` , and no other operators like `<` or `!=` in the near future. (But the option exists to add them in the future. One big risk is, developers would always be tempted to try fancy expressions and complain it does't work :( )
The other issue is, it introduces two new keywords `join` and `on`. We could still make this a non-reserved keyword like the `any` keyword in FizzBee (But that was added by mistake because I am not python developer, and didn't realize `any` was a common builtin function).

One major drawback is, it is not python like.

#### Alternative Syntax options
Any suggestions welcome


### State Bucket Algorithm
The most performant and easiest way to implement is, to create a table(or most likely in-memory map) with key being the shared state (or a hash of it), and have two columns one for states from the spec 1 and the other for states from the spec 2. For a given key k1, if there are M states from spec 1 and N states from spec 2, it is a cross join, that is, M*N state total.
In the clock example, for the key `False`, there will be 12 hour states, and 59 minutes states, producing 12*59=708 states. For the key `True`, there will be 12 hour states and 1 minutes state, producing 12*1 = 12 states, totaling 720 states.

#### Completeness (No false negatives?)
This approach is actually complete. If there is any state that violates the invariant, the violation will be reported.

#### Soundness (No false positives?)
Since we are only looking at the shared state, but not the path to the states, it is possible we might report violations that are not actually possible in the spec.
For example:
Let's take these two graphs.

![Image](https://private-user-images.githubusercontent.com/152331581/442948512-e035f854-e184-44b0-a94f-1f02f64184fb.svg?jwt=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJnaXRodWIuY29tIiwiYXVkIjoicmF3LmdpdGh1YnVzZXJjb250ZW50LmNvbSIsImtleSI6ImtleTUiLCJleHAiOjE3NDcwODQyNDIsIm5iZiI6MTc0NzA4Mzk0MiwicGF0aCI6Ii8xNTIzMzE1ODEvNDQyOTQ4NTEyLWUwMzVmODU0LWUxODQtNDRiMC1hOTRmLTFmMDJmNjQxODRmYi5zdmc_WC1BbXotQWxnb3JpdGhtPUFXUzQtSE1BQy1TSEEyNTYmWC1BbXotQ3JlZGVudGlhbD1BS0lBVkNPRFlMU0E1M1BRSzRaQSUyRjIwMjUwNTEyJTJGdXMtZWFzdC0xJTJGczMlMkZhd3M0X3JlcXVlc3QmWC1BbXotRGF0ZT0yMDI1MDUxMlQyMTA1NDJaJlgtQW16LUV4cGlyZXM9MzAwJlgtQW16LVNpZ25hdHVyZT0yOTllMjNiZDI3MDA2M2IwMjIwNTdlY2ZjZjQwYmZhZDAwYzRiZDgwNDQ1NzYzZGNmYjEwNGUzODgzOGI0N2FhJlgtQW16LVNpZ25lZEhlYWRlcnM9aG9zdCJ9.1DAeHqkEB869C71Gzm6VHy4HhilBUZdosTNXC0GY_2E)
Following the State Bucketing Algorithm, with x being the shared variable, you can see, there is a 1:1 mapping between the two states.
However, the state when x=3, is technically not reachable because, it can be reached only if the traversed path is (x=0 -> x=1 -> x=3) in Spec A, but in Spec B, the trace has to be (x=0 -> x=2 -> x=3). So a proper graph traversal will never reach the state (x=3,y=3,z=3).
This implies, we cannot add safety property that a particular state would never be reached or a liveness property that certain states would eventually be reached.

While it has false positives, and I could come up with a graph, I couldn't come up with a meaningful formal model such case could happen. I am still looking for some examples and if it is not common, the speed advantage might outweigh the false positives - after all, if there are no errors reported, we known there were no bugs.

### Transition Bucket Algorithm
Instead of mapping based on the shared state, we will map the system based on the shared state pairs (and always include a self loop indicating a stutter step).

This avoids the false positive issue reported above. Even a small modification to the above graph. Here, if y=4,z=4 violates a safety assertion, this algorithm will notice this state as an error because there is an equivalent link in both the graphs, but, we would also notice that there is no link between `x=1 -> x=3` in graph 2 or `x=2 -> x=3` in graph 1. So, this error would be reported independent of this. So the false positive issue will be superseded by this transition violation.

![enter image description here](https://private-user-images.githubusercontent.com/152331581/443527869-a38b32e5-31be-47b2-95fe-e0095d075a93.png?jwt=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJnaXRodWIuY29tIiwiYXVkIjoicmF3LmdpdGh1YnVzZXJjb250ZW50LmNvbSIsImtleSI6ImtleTUiLCJleHAiOjE3NDcyMDU3NTksIm5iZiI6MTc0NzIwNTQ1OSwicGF0aCI6Ii8xNTIzMzE1ODEvNDQzNTI3ODY5LWEzOGIzMmU1LTMxYmUtNDdiMi05NWZlLWUwMDk1ZDA3NWE5My5wbmc_WC1BbXotQWxnb3JpdGhtPUFXUzQtSE1BQy1TSEEyNTYmWC1BbXotQ3JlZGVudGlhbD1BS0lBVkNPRFlMU0E1M1BRSzRaQSUyRjIwMjUwNTE0JTJGdXMtZWFzdC0xJTJGczMlMkZhd3M0X3JlcXVlc3QmWC1BbXotRGF0ZT0yMDI1MDUxNFQwNjUwNTlaJlgtQW16LUV4cGlyZXM9MzAwJlgtQW16LVNpZ25hdHVyZT1hNThkYmUyNGIzNzUxMjcwMTU1ZDVkNmMzMGVkYjdiMDg3OTc3NGFkMTZlN2VkZTBmYTBlZmI2ZjZkYjRjNzE5JlgtQW16LVNpZ25lZEhlYWRlcnM9aG9zdCJ9.-i4GeZWGTYf79sLsTKoru4eLIUzmLGrpoJje-LuwCTM)

Note: In this approach, we need to explicitly add stuttering transitions.

#### Completeness:
This solution is complete for safety properties. If the spec has bugs, the model checker would report it

#### Soundness:
Yes. If it reports an error, it indicates a problem. There won't be any false positives. (There is a tiny caveat as explained above)

#### Liveness:
Liveness on the independent specs are obvious. But liveness based on the composition is non-trivial but feasible. One viable approach is, find the states that match the liveness predicate aka witness nodes. These must be a subset of the witness nodes in the independent specs. This is a reasonable constraint but also required because we don't have to introduce new fairness specs. Once we find these states, mark these corresponding states in the independent specs, and check for liveness using the standard technique on the subgraph.

#### Error Trace
Incase of an invariant violation, showing a proper and shorter error trace still seems to be non-trivial. One quick workaround is, show two traces one for each spec, showing the shortest path to the violating state. Since every transition in the first spec must have an equivalent transition (or stuttering step) in the second spec, we might be able to coalesce into a single trace.

### Interleaving Graph Traversal

TODO: Add details

## Modular refinement

TODO: Add details

Use case:
We could define a higher level spec for Raft that doesn't involve any details on leader election or how the logs get replicated etc. Then, have two separate specs - one for the leader election and the other for log replication. We'll then want to assert if the leader election spec implements the leader election part of the abstract spec. Similarly, the log replication spec implements the log replication part of the parent spec.

Unlike a standard refinement, where the entire concrete spec has to be proven to match the parent spec, here we would only model and assert these modules separately.

For a simpler example:
Let us say we are modeling a 12-hour clock with am/pm.
*hour_clock.fizz*
```
action Init:
    hour = 12
    meridiem = "am"

atomic action HourTick:
    hour = (hour%12) + 1
    if hour == 12:
        meridiem = "pm" if meridiem == "am" else "am"

```
There're only 24 states 12 for am and 12 for pm. Let's say, we want to refine this hour clock with details on how the hour gets updated. Instead of hour, let's track the minutes in the concrete spec.

*minute_spec.fizz*
```
action Init:
	minute = 0
atomic action MinuteTick:
	minute = (minute + 1) % (12*60) # Cycles every 12 hours 
```
Now, we need to prove if the minute_spec implements the hour part of the hour_clock spec.

### Syntax
Here, we need a way to map the minute to an hour. Also, we need to indicate we don't care about the `meridiem` in the concrete spec. This means, we would need two way mapping.

*minute_spec.fizz*
```
import path.to.hour_clock.spec as hour_clock

refines hour_clock
	on hour_clock.hour == int(minute / 60) if minute >= 60 else 12

# Init and Tick and other actions and assertions

```
Note:

1. The refinement specification will only support equals `==` operator
2. The LHS and RHS can be any expression including function calls that return the state to be used for equivalence checking.
     ```
    refines hour_clock
   on hour_clock.get_state() == get_state()
   ```

## Component Refinement
TODO

## Component Substitution
TODO
