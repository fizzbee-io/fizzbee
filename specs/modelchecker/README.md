# Specs of the FizzBee model checker

FizzBee specs of the model checker itself, written before redesigning it for
scale. Each spec models what the Go code does today, so a new design can be
specified next to it and checked against the same properties.

## ExecutionLoop.fizz

The high-level exploration loop in `modelchecker/processor.go`, plus the
post-run deadlock walk in `markovchain.go`:

| Spec | Go |
| --- | --- |
| `OldLoop` | `Processor.Start` (default: `p.queue` holds unprocessed action-starts) |
| `ProcessedStart`, `ProcessedLoop` | `startProcessedQueue` (`--experimental_processed_queue`, optionally `--experimental_no_graph`) |
| `expand_to_yield` | `expandToYield` |
| `process_node` | `processNode` (dedup, collapse, invariant check, fork scheduling) |
| `publish_yield_point`, `yield_node`, `enqueue_scheduled` | the Go functions with the same names |
| `drain_and_flush` | `drainAndFlush` |
| `find_deadlock` | `traverseBFS` deadlock detection |
| `pop_queue`, `pop_inter` | the `bfs` / `dfs` / `random` collections chosen in `NewProcessor` |

The spec being checked is abstracted to a small graph of yield points,
mid-action forks and disabled forks, where a node id stands for the state's
canonical hash. `Init` picks one of six graphs and one combination of
strategy (bfs, dfs, random), mode (graph, processed, processed no-graph) and
`--experimental_collapse_steps`. Each checker loop iteration is one atomic
action, so a counterexample reads as one queue pop per step.

The checker run is compared with a ground truth computed straight from the
graph by a BFS on action depth:

- `NoMissedViolation`, `NoMissedDeadlock`: a PASS means no reachable bad
  state or deadlock within `max_actions`.
- `ReportedFailureIsReal`, `ReportedDeadlockIsReal`: no false alarms.
- `FullCoverage`: on a PASS, exactly the reachable yield points are stored.
- `UniqueYieldCount`, `ProcessedQueueHasNoDuplicates`,
  `CollapsedStepsNotStored`: bookkeeping invariants of the optimized modes.
- `Terminates`: the loop always finishes.

Run it with `./fizz specs/modelchecker/ExecutionLoop.fizz`. With the two
known issues excluded it passes (78 configurations, 879 unique states, about
a second).

Not modeled yet: crash on yield, thread continuations, channel messages,
`max_concurrent_actions` and per-action limits, symmetry, guided traces,
refinement, `ContinueOnInvariantFailures`, incremental liveness, the
liveness checks themselves, simulation mode, and the `Enabled` flag.

## Known issues found by the spec

Set `INCLUDE_KNOWN_ISSUES = True` to get the counterexamples. Both are
reproduced against the real checker by the specs in `repro/`.

1. **DFS and random strategies can miss states within `max_actions`**
   (`depth_shortcut`, `repro/DfsDepthBoundMiss.fizz`). The visited set
   ignores the depth a state was reached at. If DFS first stores a state at
   depth 2 via a longer path, it is not expanded at the bound, and the later
   depth-1 path to it is a dedup hit. The repro fails an assertion with bfs
   and passes with `--exploration_strategy dfs`. The `--experimental_processed_queue`
   flag help already warns about this for DFS/random; the default queue has
   the same problem.

2. **A deadlock is missed when the stuck yield point's only action reaches
   a mid-action state that was already visited from another yield point**
   (`shared_dead_end`, `repro/SharedDeadEndDeadlock.fizz`). All three modes
   report PASSED. The early check in `startProcessedQueue` counts the dedup
   hit as a live successor, and `traverseBFS` visits each node once, so the
   shared node is only ever attributed to the first yield point that reached
   it. Changing the action so the mid-action states differ makes every mode
   report the deadlock.
