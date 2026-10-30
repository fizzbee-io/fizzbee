# Model checker loop, step by step

Each file is one step of the tutorial process applied to the model checker
itself. The write-up with runs and observations is the "Model Checker Spec
Walkthrough" doc in the project.

Main line, the loop at the high level (a state is an opaque id, the
Executor is an oracle):

- `step1.fizz`: roles and state from the whiteboard.
- `step2.fizz`: RPCs and the one `Step` action.
- `step3.fizz`: review the run with `exists` assertions.
- `step7.fizz`: the loop with exploration order, action depth bound,
  deadlock, safety check, and the end-of-run liveness call. Passes with bfs;
  set `ORDERS = ["dfs"]` to reproduce the known depth-bound issue.
- `step8.fizz`: the current high-level loop. BFS only, no crash recovery,
  edges labelled with fairness and crash for the liveness oracle, and the
  deadlock-or-stutter switch (`deadlock_detection`).

Robustness line, the same loop under checker crashes and with two workers.
This refines the main line and will move to its own spec:

- `step4a.fizz`, `step4b.fizz`, `step4c.fizz`: crashes, claims, and the seen
  set moving into the frontier.
- `step5.fizz`: properties against ground truth.
- `step6.fizz`: two explorers.

Run any of them with `./fizz specs/modelchecker/walkthrough/stepN.fizz`.
