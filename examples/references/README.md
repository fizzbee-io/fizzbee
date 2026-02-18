# FizzBee Language Reference Examples

Comprehensive reference examples for the FizzBee specification language, organized by topic for easy learning and AI coding agent reference.

📖 **[FizzBee Language Reference Guide](LANGUAGE_REFERENCE.md)** - Complete language specification with syntax and semantics
⚡ **[Performance Guide](PERFORMANCE_GUIDE.md)** - Tips for reducing state space and runtime
⚠️ **[Gotchas and Common Issues](GOTCHAS.md)** - Known pitfalls and workarounds
🔍 **[Verification Guide](VERIFICATION_GUIDE.md)** - How to sanity-check specs with simulation and guided traces

## Organization

Examples use **hierarchical numbering** (e.g., `01-01`, `13-02-01`, `99-01`):
- **Section** (01-99): Major topic area
- **Item** (01-99): Example within section
- **Subsection** (01-07): Subcategory for large sections (Section 13: Patterns)
- **Section 99**: Miscellaneous (catch-all for features not fitting other categories)

### Sections

- **01**: Basics (5 examples) - Foundation concepts
- **02**: Flow Modifiers (4) - atomic, serial, parallel, oneof
- **03**: Conditionals (3) - if/else statements
- **04**: Loops (5) - for/while/break/continue
- **05**: Nondeterminism (3) - any statement variations
- **06**: Functions (5) - Function definitions and modifiers
- **07**: Guards (3) - require/pass statements
- **08**: Data Types (9) - Collections, enums, records
- **09**: Assertions (5) - Safety and liveness properties
- **10**: Fairness (3) - Unfair, weak fair, strong fair
- **11**: Roles (8) - Role basics and management
- **12**: Advanced (6) - Nesting, config, best practices
- **13**: Patterns (19) - Distributed systems patterns
  - **13-01**: Locking (4) - Mutex, RW lock, distributed lock
  - **13-02**: Transactions (3) - 2PC, saga, snapshot isolation
  - **13-03**: Consensus (3) - Consensus, leader election, CAS
  - **13-04**: Communication (3) - Producer-consumer, queues, cache
  - **13-05**: Consistency (2) - Eventual consistency, idempotency
  - **13-06**: Resilience (2) - Circuit breaker, rate limiting
  - **13-07**: Data/Events (2) - WAL, event sourcing
- **14**: Fault Injection (4) - Crash and message loss
- **15**: Configuration (2) - Action-level config, coordination
- **16**: Symmetry Reduction (13) - State space optimization (old API + symmetry module)
- **99**: Miscellaneous (1) - Checkpoints and other utilities

**Total: 100 examples across 17 sections**

## Examples Created

### 01-01-noop: No-op Action
- **State space**: 1 node, 1 unique state
- **Purpose**: Simplest possible FizzBee spec demonstrating basic action syntax
- **Key concepts**: `action Init`, `atomic action`, `pass` statement
- **Status**: ✅ PASSED

### 01-02-atomic-action: Atomic Action
- **State space**: 101 nodes (max_actions default is 100)
- **Purpose**: Single atomic action modifying state
- **Key concepts**: atomic modifier, state transitions
- **Status**: ✅ PASSED
- **⚠️ Important**: This is an INFINITE state space model! FizzBee caps at max_actions=100. Real models with unbounded counters will explode - always bound your state variables.

### 01-03-multiple-actions: Multiple Actions
- **State space**: 201 nodes (counter can go from 0 to 100 or 0 to -100)
- **Purpose**: Multiple independent actions operating on same state
- **Key concepts**: Multiple actions creating different transition paths
- **Status**: ✅ PASSED

### 01-04-init-action: Init Action Details
- **State space**: 101 nodes
- **Purpose**: Demonstrates how Init initializes multiple state variables
- **Key concepts**: State variables, lists as state, multiple fields
- **Status**: ✅ PASSED

### 01-05-init-with-constants: Constants and Init
- **State space**: 4 nodes (deadlocks at counter=3)
- **Purpose**: Global constants and guard clauses
- **Key concepts**: Top-level constants, guard clauses, deadlock detection
- **Status**: ⚠️  DEADLOCK (intentional - shows guard clause behavior)
- **Notes**: Includes commented fix options

### 02-01-atomic-block: Atomic Block
- **State space**: 2 nodes
- **Purpose**: Atomic blocks execute all statements indivisibly
- **Key concepts**: `atomic` block modifier, preventing race conditions
- **Status**: ⚠️  ASSERTION FAILURE (intentional - shows serial vs atomic difference)
- **Notes**: Demonstrates how atomic prevents observing intermediate states

### 02-02-serial-block: Serial Block
- **State space**: 9 nodes, 8 unique states
- **Purpose**: Serial blocks yield after each statement (default)
- **Key concepts**: `serial` block modifier, yielding, interleaving
- **Status**: ✅ PASSED

### 02-03-parallel-block: Parallel Block
- **State space**: 69 nodes, 56 unique states
- **Purpose**: Parallel blocks execute statements concurrently
- **Key concepts**: `parallel` block modifier, concurrent execution, interleaving
- **Status**: ✅ PASSED

### 02-04-oneof-block: Oneof Block
- **State space**: 5 nodes, 4 unique states
- **Purpose**: Oneof blocks create nondeterministic choice
- **Key concepts**: `oneof` block modifier, nondeterminism, branching
- **Status**: ✅ PASSED

### 03-01-if-else: If-Else Statement
- **State space**: 4 nodes, 4 unique states
- **Purpose**: Basic conditional branching
- **Key concepts**: `if/else` control flow
- **Status**: ✅ PASSED

### 03-02-if-elif-else: If-Elif-Else Statement
- **State space**: 11 nodes, 11 unique states
- **Purpose**: Multiple conditional branches
- **Key concepts**: `if/elif/else` control flow
- **Status**: ✅ PASSED

### 03-03-nested-if: Nested If Statements
- **State space**: 4 nodes, 4 unique states
- **Purpose**: Nested conditional logic
- **Key concepts**: Nested `if/else` structures
- **Status**: ✅ PASSED

### 04-01-for-loop-atomic: For Loop (Atomic)
- **State space**: 2 nodes, 2 unique states
- **Purpose**: Atomic for loop - all iterations as one step
- **Key concepts**: `atomic for`, no yielding between iterations
- **Status**: ✅ PASSED

### 04-02-for-loop-serial: For Loop (Serial)
- **State space**: 40 nodes, 39 unique states
- **Purpose**: Serial for loop - yields after each iteration
- **Key concepts**: `serial for`, interleaving between iterations
- **Status**: ✅ PASSED
- **Note**: Unbounded state space - see 0002

### 04-03-for-loop-parallel: For Loop (Parallel)
- **State space**: 111 nodes, 46 unique states
- **Purpose**: Parallel for loop - iterations execute concurrently
- **Key concepts**: `parallel for`, all interleavings explored
- **Status**: ✅ PASSED

### 04-04-while-loop: While Loop
- **State space**: 4 nodes, 4 unique states
- **Purpose**: While loop with bounded condition
- **Key concepts**: `while` loop, guard conditions
- **Status**: ✅ PASSED

### 04-05-break-continue: Break and Continue
- **State space**: 2 nodes, 2 unique states
- **Purpose**: Loop control statements
- **Key concepts**: `break`, `continue` in loops
- **Status**: ✅ PASSED

### 05-01-any-statement: Any Statement
- **State space**: 11 nodes, 7 unique states
- **Purpose**: Nondeterministic choice from a collection
- **Key concepts**: `any` statement, exploring all possibilities
- **Status**: ✅ PASSED

### 05-02-any-with-condition: Any with Condition
- **State space**: 32 nodes, 26 unique states
- **Purpose**: Any statement with filtering
- **Key concepts**: `any [x for x in list if condition]`, conditional nondeterminism
- **Status**: ✅ PASSED

### 05-03-any-fairness: Any with Fairness
- **State space**: 251 nodes, 147 unique states
- **Purpose**: Any statement with fairness modifiers
- **Key concepts**: `fair` modifier with `any`, liveness properties
- **Status**: ✅ PASSED

### 06-01-simple-function: Simple Function
- **State space**: 4 nodes, 4 unique states
- **Purpose**: Basic function definition and calls
- **Key concepts**: `func` keyword, function calls, return values
- **Status**: ✅ PASSED

### 06-02-function-with-params: Function with Parameters
- **State space**: 3 nodes, 3 unique states
- **Purpose**: Functions with multiple parameters
- **Key concepts**: Multiple parameters, parameter order
- **Status**: ✅ PASSED

### 06-03-function-return: Function Return Values
- **State space**: 21 nodes, 17 unique states
- **Purpose**: Conditional returns in functions
- **Key concepts**: Early return, conditional logic
- **Status**: ✅ PASSED

### 06-04-atomic-function: Atomic Function
- **State space**: 2 nodes, 2 unique states
- **Purpose**: Atomic functions execute without yielding
- **Key concepts**: `atomic func` modifier
- **Status**: ✅ PASSED

### 06-05-serial-function: Serial Function
- **State space**: 40 nodes, 39 unique states
- **Purpose**: Functions with serial behavior internally
- **Key concepts**: `serial` within atomic func, function call restrictions
- **Status**: ✅ PASSED
- **Note**: Functions must be called from atomic context or roles

### 07-01-require-statement: Require Statement
- **State space**: 2 nodes, 2 unique states
- **Purpose**: Explicit guard clauses with require
- **Key concepts**: `require` statement, enabling conditions
- **Status**: ✅ PASSED

### 07-02-pass-statement: Pass Statement
- **State space**: 6 nodes, 6 unique states
- **Purpose**: Unconditionally enabling actions
- **Key concepts**: `pass` statement, preventing deadlock
- **Status**: ✅ PASSED

### 07-03-guard-clauses: Guard Clauses Patterns
- **State space**: 4 nodes, 4 unique states
- **Purpose**: Comparing guard clause patterns
- **Key concepts**: if-return vs require, enabling behavior
- **Status**: ✅ PASSED

### 08-01-lists: Lists
- **State space**: 9 nodes, 9 unique states
- **Purpose**: Ordered collections with list operations
- **Key concepts**: append, insert, pop, remove
- **Status**: ✅ PASSED

### 08-02-sets: Sets
- **State space**: 2 nodes, 2 unique states
- **Purpose**: Unordered unique element collections
- **Key concepts**: add, discard, union, intersection
- **Status**: ✅ PASSED

### 08-03-dicts: Dictionaries
- **State space**: 3 nodes, 3 unique states
- **Purpose**: Key-value pairs (hashable keys)
- **Key concepts**: get, set, keys, values
- **Status**: ✅ PASSED

### 08-04-genericset: Generic Set
- **State space**: 3 nodes, 3 unique states
- **Purpose**: Sets supporting non-hashable elements
- **Key concepts**: genericset with dicts/lists as elements
- **Status**: ✅ PASSED

### 08-05-genericmap: Generic Map
- **State space**: 3 nodes, 3 unique states
- **Purpose**: Maps supporting non-hashable keys
- **Key concepts**: genericmap with list keys (local scope)
- **Status**: ✅ PASSED
- **Note**: Limited state persistence - use in local scope

### 08-06-bags: Bags (Multisets)
- **State space**: 25 nodes, 25 unique states
- **Purpose**: Collections allowing duplicates
- **Key concepts**: bag operations, duplicate counting
- **Status**: ✅ PASSED

### 08-07-enums: Enums
- **State space**: 3 nodes, 3 unique states
- **Purpose**: Named constants for readability
- **Key concepts**: enum definition, dir() for iteration
- **Status**: ✅ PASSED

### 08-08-records: Records
- **State space**: 3 nodes, 3 unique states
- **Purpose**: Mutable named structures
- **Key concepts**: record creation, field access/modification
- **Status**: ✅ PASSED

### 08-09-structs: Structs
- **State space**: 4 nodes, 4 unique states
- **Purpose**: Immutable named structures (Starlark)
- **Key concepts**: struct immutability, constant configs
- **Status**: ✅ PASSED

### 09-01-always-assertion: Always Assertion (Safety)
- **State space**: 6 nodes, 6 unique states
- **Purpose**: Safety properties that must hold in all states
- **Key concepts**: `always assertion`, invariants
- **Status**: ✅ PASSED

### 09-02-exists-assertion: Exists Assertion
- **State space**: 11 nodes, 11 unique states
- **Purpose**: Existential properties - at least one state satisfies
- **Key concepts**: `exists assertion`, reachability
- **Status**: ✅ PASSED

### 09-03-transition-assertion: Transition Assertion
- **State space**: 8 nodes, 8 unique states
- **Purpose**: Properties about state transitions
- **Key concepts**: `transition assertion`, before/after states, stutter-invariant
- **Status**: ✅ PASSED

### 09-04-eventually-always: Eventually Always (Liveness)
- **State space**: 9 nodes, 9 unique states
- **Purpose**: Eventually reaches stable state
- **Key concepts**: `eventually always`, liveness properties, requires fairness
- **Status**: ✅ PASSED

### 09-05-always-eventually: Always Eventually (Progress)
- **State space**: 4 nodes, 4 unique states
- **Purpose**: System always makes progress
- **Key concepts**: `always eventually`, strong fairness, progress
- **Status**: ✅ PASSED

### 10-01-unfair-action: Unfair Action (Default)
- **State space**: 108 nodes, 46 unique states
- **Purpose**: Unfair actions have no fairness guarantees
- **Key concepts**: Default unfairness, starvation possible
- **Status**: ✅ PASSED

### 10-02-weak-fair-action: Weak Fair Action
- **State space**: 11 nodes, 11 unique states
- **Purpose**: Weak fairness guarantees execution if continuously enabled
- **Key concepts**: `fair` or `fair<weak>`, eventual execution
- **Status**: ✅ PASSED

### 10-03-strong-fair-action: Strong Fair Action
- **State space**: 4 nodes, 4 unique states
- **Purpose**: Strong fairness guarantees execution if infinitely often enabled
- **Key concepts**: `fair<strong>`, intermittent enabling
- **Status**: ✅ PASSED

### 11-01-simple-role: Simple Role
- **State space**: 11 nodes, 11 unique states
- **Purpose**: Basic role definition with actions
- **Key concepts**: `role` keyword, role actions, role instantiation
- **Status**: ✅ PASSED

### 11-02-role-init: Role Init Action
- **State space**: 108 nodes, 108 unique states
- **Purpose**: Role initialization with parameters
- **Key concepts**: Role Init action, role parameters, state initialization
- **Status**: ✅ PASSED

### 11-03-role-actions: Role Actions
- **State space**: 6 nodes, 6 unique states
- **Purpose**: Actions within roles
- **Key concepts**: Role actions, implicit self
- **Status**: ✅ PASSED

### 11-04-role-functions: Role Functions
- **State space**: 6 nodes, 6 unique states
- **Purpose**: Functions within roles
- **Key concepts**: Role functions, implicit self parameter
- **Status**: ✅ PASSED

### 11-05-role-state: Role State
- **State space**: 125 nodes, 115 unique states
- **Purpose**: Role state management (self.* variables)
- **Key concepts**: State vs local variables, self.* persistence
- **Status**: ✅ PASSED

### 11-06-role-decorators: Role Decorators
- **State space**: 6 nodes, 5 unique states
- **Purpose**: Role state decorators for durability
- **Key concepts**: `@state(durable=[...])`, `@state(ephemeral=[...])`
- **Status**: ✅ PASSED

### 11-07-role-instances: Role Instances
- **State space**: 330 nodes, 330 unique states
- **Purpose**: Creating and managing multiple role instances
- **Key concepts**: Multiple instances, independent state
- **Status**: ✅ PASSED

### 11-08-dynamic-role-lifecycle: Dynamic Role Lifecycle
- **State space**: 113 nodes, 70 unique states
- **Purpose**: Creating and removing role instances at runtime
- **Key concepts**: Dynamic `Role()` creation, removal by `__id__`, lifecycle management (hire → work → fire)
- **Status**: ✅ PASSED
- **Note**: Non-symmetric version requires `MAX_HIRES` cap because each Worker#N is distinct. Compare with 16-13 for symmetric version.

### 12-01-nested-flow-modifiers: Nested Flow Modifiers
- **State space**: 5206 nodes, 5200 unique states
- **Purpose**: Nesting flow modifiers with different behaviors
- **Key concepts**: Nested atomic/serial blocks, yield behavior
- **Status**: ✅ PASSED

### 12-02-yaml-frontmatter: YAML Frontmatter Configuration
- **State space**: 21 nodes, 21 unique states
- **Purpose**: Configure model checker via YAML frontmatter
- **Key concepts**: `options.max_actions`, `deadlock_detection`
- **Status**: ✅ PASSED

### 12-03-advanced-any-patterns: Advanced Any Patterns
- **State space**: 112 nodes, 76 unique states
- **Purpose**: Complex nondeterministic choices
- **Key concepts**: `any` with multiple filters, excluding previous choices
- **Status**: ✅ PASSED

### 12-04-state-space-best-practices: State Space Best Practices
- **State space**: 660 nodes, 660 unique states
- **Purpose**: Techniques to prevent state space explosion
- **Key concepts**: Guard clauses, modulo arithmetic, bounded collections
- **Status**: ✅ PASSED

### 12-05-error-handling-pattern: Error Handling Pattern
- **State space**: 240 nodes, 213 unique states
- **Purpose**: Model errors and recovery explicitly
- **Key concepts**: `oneof` for success/failure paths, retry logic, error states
- **Status**: ✅ PASSED

### 12-06-global-variables: Global Variables
- **State space**: 6 nodes, 6 unique states
- **Purpose**: State variables and top-level constants
- **Key concepts**: Automatic global state, top-level constants, no `global` keyword needed
- **Status**: ✅ PASSED

### 13-01-01-mutex: Mutex Pattern
- **State space**: 199 nodes, 199 unique states
- **Purpose**: Simple mutual exclusion with lock/unlock
- **Key concepts**: Mutex pattern, critical section, lock holder
- **Status**: ✅ PASSED

### 13-04-01-producer-consumer: Producer-Consumer Pattern
- **State space**: 18 nodes, 18 unique states
- **Purpose**: Classic bounded buffer problem
- **Key concepts**: Queue operations, capacity limits, coordination
- **Status**: ✅ PASSED

### 13-02-01-two-phase-commit: Two-Phase Commit Pattern
- **State space**: 20 nodes, 14 unique states
- **Purpose**: Simplified 2PC protocol with coordinator and participants
- **Key concepts**: Distributed consensus, prepare/commit phases, voting
- **Status**: ✅ PASSED

### 13-03-02-leader-election: Leader Election Pattern
- **State space**: 12 nodes, 8 unique states
- **Purpose**: Simple leader election with proposer and voters
- **Key concepts**: Voting, quorum, leader safety
- **Status**: ✅ PASSED

### 13-04-03-cache-coherence: Cache Coherence Pattern
- **State space**: 9 nodes, 9 unique states
- **Purpose**: Simple write-through cache with invalidation
- **Key concepts**: Cache consistency, invalidation, write-through
- **Status**: ✅ PASSED

### 13-01-02-read-write-lock: Read-Write Lock Pattern
- **State space**: 7509 nodes, 7509 unique states
- **Purpose**: Multiple readers OR single writer
- **Key concepts**: Readers-writer lock, concurrent reads, exclusive writes
- **Status**: ✅ PASSED

### 13-04-02-message-queue: Message Queue Pattern
- **State space**: 9 nodes, 9 unique states
- **Purpose**: FIFO message queue with send/receive
- **Key concepts**: Queue ordering, FIFO semantics, message delivery
- **Status**: ✅ PASSED

### 13-02-03-snapshot-isolation: Snapshot Isolation Pattern
- **State space**: 21 nodes, 21 unique states
- **Purpose**: Transactions read from snapshot, commit if no conflicts
- **Key concepts**: Snapshot isolation, read/write sets, conflict detection
- **Status**: ✅ PASSED

### 13-05-02-idempotent-operations: Idempotent Operations Pattern
- **State space**: 7 nodes, 4 unique states
- **Purpose**: Operations that can be safely retried
- **Key concepts**: Idempotency, deduplication, request IDs
- **Status**: ✅ PASSED

### 13-05-01-eventual-consistency: Eventual Consistency Pattern
- **State space**: 14 nodes, 14 unique states
- **Purpose**: Multiple replicas eventually converge
- **Key concepts**: Eventual consistency, replica synchronization, fairness
- **Status**: ✅ PASSED

### 13-01-04-optimistic-locking: Optimistic Locking Pattern
- **State space**: 26 nodes, 26 unique states
- **Purpose**: Version-based concurrency control
- **Key concepts**: Optimistic locking, version numbers, CAS, retry
- **Status**: ✅ PASSED

### 13-06-02-rate-limiting: Rate Limiting Pattern
- **State space**: 582 nodes, 582 unique states
- **Purpose**: Token bucket rate limiter
- **Key concepts**: Rate limiting, token bucket, capacity management
- **Status**: ✅ PASSED

### 13-03-01-consensus: Consensus Pattern
- **State space**: 42 nodes, 25 unique states
- **Purpose**: Simple consensus with proposals and acceptance
- **Key concepts**: Consensus, proposals, acceptance quorum
- **Status**: ✅ PASSED

### 13-01-03-distributed-lock: Distributed Lock Pattern
- **State space**: 5051 nodes, 5051 unique states
- **Purpose**: Lock with lease/timeout for fault tolerance
- **Key concepts**: Distributed locking, lease expiration, fencing
- **Status**: ✅ PASSED

### 13-02-02-saga-pattern: Saga Pattern
- **State space**: 7 nodes, 6 unique states
- **Purpose**: Long-running transaction with compensation
- **Key concepts**: Saga pattern, compensating transactions, rollback
- **Status**: ✅ PASSED

### 13-06-01-circuit-breaker: Circuit Breaker Pattern
- **State space**: 42 nodes, 42 unique states
- **Purpose**: Prevents cascading failures with circuit states
- **Key concepts**: Circuit breaker, failure threshold, state transitions
- **Status**: ✅ PASSED

### 13-07-01-write-ahead-log: Write-Ahead Log Pattern
- **State space**: 6 nodes, 6 unique states
- **Purpose**: Durability through logging before applying changes
- **Key concepts**: WAL, durability, crash recovery
- **Status**: ✅ PASSED

### 13-07-02-event-sourcing: Event Sourcing Pattern
- **State space**: 15 nodes, 15 unique states
- **Purpose**: State derived from event stream
- **Key concepts**: Event sourcing, event replay, state reconstruction
- **Status**: ✅ PASSED

### 13-03-03-compare-and-swap: Compare-And-Swap Pattern
- **State space**: 15 nodes, 15 unique states
- **Purpose**: Atomic compare-and-swap for lock-free algorithms
- **Key concepts**: CAS operation, lock-free updates, atomic operations
- **Status**: ✅ PASSED

### 14-01-crash-on-yield: Crash on Yield (Implicit Fault Injection)
- **State space**: 14 nodes, 14 unique states
- **Purpose**: FizzBee's automatic crash fault injection at yield points
- **Key concepts**: `crash_on_yield`, implicit fault injection, yield points
- **Status**: ✅ PASSED
- **Note**: This is FizzBee's key differentiator from TLA+

### 14-02-disable-crash-on-yield: Disable Crash on Yield
- **State space**: 17 nodes, 17 unique states
- **Purpose**: Turning off implicit fault injection
- **Key concepts**: `crash_on_yield: false`, happy path testing, liveness with fairness
- **Status**: ✅ PASSED

### 14-03-role-crash-ephemeral: Role Crash with Ephemeral State
- **State space**: 3158 nodes, 2752 unique states
- **Purpose**: Simulating process crashes with state loss
- **Key concepts**: Role crashes, ephemeral vs durable state, recovery
- **Status**: ✅ PASSED

### 14-04-message-loss-rpc: Message Loss in Role Communication
- **State space**: 9 nodes, 9 unique states
- **Purpose**: Automatic message loss simulation in RPC calls
- **Key concepts**: Message loss, RPC failures, network unreliability
- **Status**: ✅ PASSED

### 15-01-action-level-config: Action-Level Configuration
- **State space**: 72 nodes, 72 unique states
- **Purpose**: Per-action limits and crash settings
- **Key concepts**: `action_options`, per-action `max_actions`, per-action `crash_on_yield`
- **Status**: ✅ PASSED

### 15-02-action-coordination: Action Coordination with Require
- **State space**: Varies
- **Purpose**: Using `require` mid-action for coordination between actions
- **Key concepts**: Yield points, action coordination, require in middle of action
- **Status**: ✅ PASSED

### 16-01-symmetric-values: Symmetric Values for IDs
- **State space**: Significantly reduced compared to regular values
- **Purpose**: Using `symmetric_values()` to reduce state space by treating permutations as equivalent
- **Key concepts**: `symmetric_values()`, state space optimization, interchangeable IDs
- **Status**: ✅ PASSED
- **Note**: Compare state space with/without symmetry to see the reduction

### 16-02-symmetric-roles: Symmetric Roles
- **State space**: N! reduction factor for N role instances
- **Purpose**: Using `symmetric role` keyword for indistinguishable role instances
- **Key concepts**: `symmetric role`, automatic symmetry reduction, `__id__` field
- **Status**: ✅ PASSED
- **Critical**: Must use `bag()` or `set()`, NOT `list()` with symmetric roles

### 16-03-list-vs-bag-pitfall: Common Pitfall - List vs Bag
- **State space**: Shows broken symmetry when using list
- **Purpose**: Demonstrates why you MUST use bag() or set() with symmetric roles
- **Key concepts**: Order-dependent collections breaking symmetry, common mistakes
- **Status**: ⚠️ INTENTIONAL ISSUE - Shows incorrect usage
- **Lesson**: Lists defeat symmetry reduction; always use bags or sets

### 16-04-symmetry-comparison: State Space Comparison
- **Files**: WithoutSymmetry.fizz, WithSymmetry.fizz, README.md
- **State space**: ~50% reduction with 2 processes, N! reduction with N processes
- **Purpose**: Side-by-side comparison showing impact of symmetry reduction
- **Key concepts**: Measuring state space reduction, scaling analysis
- **Status**: ✅ BOTH PASSED
- **How to use**: Run both files and compare state/node counts

### 16-05-nominal-symmetry: Nominal Symmetry (symmetry module)
- **State space**: 8 nodes, 4 unique states
- **Purpose**: Interchangeable identifiers using `symmetry.nominal()`
- **Key concepts**: `fresh()`, `values()`, `choices()`, `choose()`, unordered IDs
- **Status**: ✅ PASSED

### 16-06-ordinal-symmetry: Ordinal Symmetry (symmetry module)
- **State space**: 5 nodes, 5 unique states
- **Purpose**: Ordered values where only relative rank matters
- **Key concepts**: `symmetry.ordinal()`, `fresh()`, `min()`, `max()`, ordering comparisons
- **Status**: ✅ PASSED

### 16-07-ordinal-segments: Ordinal Segments (gap insertion)
- **State space**: 4 nodes, 1 unique state
- **Purpose**: Inserting values between existing ordinal values using `segments()`
- **Key concepts**: `segments()`, `segment.fresh()`, `after`/`before` filtering
- **Status**: ✅ PASSED

### 16-08-interval-symmetry: Interval Symmetry (symmetry module)
- **State space**: 11 nodes, 11 unique states
- **Purpose**: Ordered values where distance is meaningful, with arithmetic
- **Key concepts**: `symmetry.interval()`, `val + int`, `val - int`, `val - val`, zero-shifting
- **Status**: ✅ PASSED

### 16-09-interval-divergence: Interval Divergence
- **State space**: 5 nodes, 5 unique states
- **Purpose**: Bounding the spread of interval values with `divergence` parameter
- **Key concepts**: `divergence`, automatic spread enforcement, derivation rules
- **Status**: ✅ PASSED

### 16-10-rotational-symmetry: Rotational Symmetry (symmetry module)
- **State space**: 11 nodes, 6 unique states
- **Purpose**: Ring/modular arithmetic with wrapping values
- **Key concepts**: `symmetry.rotational()`, `val + int` (wrapping), `val - val` (mod), ring positions
- **Status**: ✅ PASSED

### 16-11-reflection: Reflection Symmetry
- **Files**: ReflectionInterval.fizz (6 states), ReflectionNoReflection.fizz (11 states), ReflectionRotational.fizz (7 states)
- **Purpose**: Mirror-state equivalence using `reflection=True`
- **Key concepts**: `reflection=True`, interval reflection (v-min vs max-v), rotational reflection (CW=CCW)
- **Status**: ✅ ALL PASSED
- **Note**: Compare ReflectionInterval.fizz (6 states) vs ReflectionNoReflection.fizz (11 states) for ~45% reduction

### 16-12-materialize: Materialized Domains
- **State space**: 23 nodes, 6 unique states
- **Purpose**: Pre-populating all domain values with `materialize=True`
- **Key concepts**: `materialize=True`, `values()` returns all upfront, `fresh()` disallowed
- **Status**: ✅ PASSED

### 16-13-dynamic-symmetric-roles: Dynamic Symmetric Role Lifecycle
- **State space**: 9 nodes, 6 unique states
- **Purpose**: Creating and removing symmetric role instances at runtime
- **Key concepts**: `symmetric role` + `bag()`, dynamic creation, removal by `__id__`, unlimited lifecycle with symmetry
- **Status**: ✅ PASSED
- **Note**: Compare with 11-08 (non-symmetric, 113 nodes / 70 states). Symmetric version needs no hire cap — removed nodes get replaced by fresh ones that map to the same canonical states.

### 99-01-checkpoints: Visualization Checkpoints
- **State space**: 12 nodes, 10 unique states
- **Purpose**: Using backtick labels to create visualization breakpoints
- **Key concepts**: `` `checkpoint` `` syntax, visualization aids, debugging workflow
- **Status**: ✅ PASSED
- **Note**: Checkpoints don't affect semantics, only visualization in explorer

## State Space Analysis

All examples have reasonable state spaces:
- **0001-0009**: Flow modifiers and basic actions (1-69 states)
- **0010-0012**: Conditionals (4-11 states, naturally bounded)
- **0013**: Atomic for loop (2 states - all iterations atomic)
- **0014**: Serial for loop (40 states - yields between iterations) - ⚠️ **INFINITE without cap**
- **0015**: Parallel for loop (111 states - explores interleavings)
- **0016-0017**: While/break/continue (2-4 states, naturally bounded)
- **0018-0020**: Any statements (11-251 states, fairness increases exploration)
- **0021-0025**: Functions (2-40 states, atomic vs serial behavior)
- **0026-0028**: Guard clauses and enabling (2-6 states)
- **0029-0037**: Data structures (2-25 states, various collection types)
- **0038-0042**: Assertions (4-11 states, safety and liveness properties)
- **0043-0045**: Fairness (4-108 states, fairness modifiers affect exploration)
- **0046-0052**: Roles (6-330 states, role instances and state management)
- **0053-0058**: Advanced patterns (6-5206 states, configuration and best practices)
- **0059-0070**: Common distributed patterns (4-7509 states, practical concurrency patterns)
- **0071-0077**: Advanced distributed patterns (6-5051 states, consensus, sagas, WAL, event sourcing)
- **0078-0082**: Fault injection (9-3158 states, crash_on_yield, message loss, ephemeral state)

### Critical Warning: State Space Explosion
Examples 0002-0004 demonstrate unbounded state spaces that would grow infinitely without the `max_actions=100` default cap. In production models:
- **Always bound your state variables** (e.g., `if counter < MAX_VALUE`)
- **Use modulo arithmetic** for cyclic behavior (e.g., `counter = (counter + 1) % 10`)
- **Add guard clauses** to prevent unbounded growth
- **Monitor state space growth** during development

Example 0005 shows the correct pattern: naturally bounded state space using guard clauses.

## Usage

Run any example:
```bash
fizz examples/references/01-01-noop/NoOp.fizz
```

Output will be in the corresponding `out/` directory within each example folder.
