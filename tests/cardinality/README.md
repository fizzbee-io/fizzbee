# cardinality.counter differential tests

Each case pairs `Counter.fizz` (written with `cardinality.counter`) with
`Int.fizz` (the same spec with a plain int, bounded by a constant `N` or by
the spec itself). `expected.txt` gives the expected Counter verdict of every
assertion, the Int verdict observed (`int=`), and actions the counterexample
loop must contain (`loop_includes=`). `imprecision` marks a Counter FAIL
where Int PASSES that the abstraction or the loop rule allows.

Run (needs a build of the checker; `FIZZ` defaults to `./fizz`):

    FIZZ=path/to/fizz python3 tests/cardinality/difftest.py [--sweep 2,3,4,5] [case ...]

FizzBee stops at the first failing assertion, so difftest checks every
assertion in its own run, on a temp copy with the other assertions removed.
It checks that every Int FAIL/ERROR is a Counter FAIL/ERROR, that Counter
matches `expected.txt`, and that a Counter FAIL with Int PASS is marked
`imprecision` (if such a case PASSES, it is reported as IMPROVED, not as a
failure). `--sweep` reruns Int.fizz with `N = 2..5`; Int counts as FAIL if
any N fails. `flags.txt` / `int_flags.txt` / `counter_flags.txt` add command
line flags. Exit status: 0 ok, 1 mismatch, 2 `cardinality` not in the build.

| Case | What it covers |
|---|---|
| 01-drain-after-close | Send until Close, fair Ack: the post-Close MANY self-loop is dropped (PASS). |
| 02-unbounded-sends | Sends never stop: Send/Ack loop at MANY is real (FAIL, loop shows Send). |
| 03-independent-counters | `a` decrement-only (dropped), `b` with real inc/dec cycles (FAIL). |
| 04-unrelated-increment | `a` drains while `b` is incremented, in another action or in the same step. |
| 05-selfloop-and-inc-cycle | Decrement self-loops and an incrementing cycle in one component: cycle kept. |
| 06-dict-idle-busy | `counts["idle"]` / `counts["busy"]` are separate keys. |
| 07-copy-local-and-arg | Value stored back through a local and through a function argument/return. |
| 08-alias-copy | `other = pending` is an unknown change at `other` only. |
| 09-symmetric-role | Counter field of a symmetric role, coarsened to `Server/pending` (one allowed imprecision). Int runs with `--no-symmetry-reduction`. |
| 10-list-elements | Counters in a list, coarsened to `queues[*]` (one allowed imprecision). |
| 11-serial-steps | Serial action with inc and dec in separate steps: never PASS where Int FAILS. |
| 12-net-change-in-one-step | dec,inc,dec in one step is a decrement; inc,dec and dec,inc record nothing. |
| 13-two-dec-one-inc | Two decrement steps and one increment step per lap: kept (allowed imprecision). |
| 14-chained-refinement | a -> b -> c chain: several rounds of dropping, then a weak-fairness refinement. |
| 15-safety | Guarded decrements, value API (`bool`, `str`, `==`, `inc`, `dec`), a failing invariant. |
| 16-dec-zero-error | Unguarded `dec()` on ZERO is an error. |
| 17-unsupported-ops | `+`, `-`, `<`, `int()` on a counter are errors. |
