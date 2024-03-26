# FizzBee Quick Start for TLA+ Users

## Introduction
The meat of the code will be in Starlark language, a subset of Python. So expressions can be tried
with a typical Python REPL. But there are some additions suitable for model checking. 

## High level structure
Directory:
fizz.yaml file. It is a config file, for model checking


###  directory
- .fizz file: The main model specification file
- fizz.yaml (Optional): The model checking config file. Yaml representation for the protobuf defined in proto/statespace_options.proto
- performance config files (Optional): The performance config files. Yaml representation for the protobuf defined in proto/performance_model.proto

### fizz.yaml file
Example:
```yaml
options:
  maxActions: 10
  maxConcurrentActions: 2
  
actionOptions:
  YourActionName:
    maxActions: 1
```

### .fizz file
The main file that contains the specification. It is a text file with the extension .fizz.

The generic structure of the file is:

```
# init action

# invariants

# action1

# action2

# additional_fuctions 
```
## Actions
Actions are the main building blocks of the model. They are the steps that the model takes to reach a state.

### Action definition
```
[atomic|serial] action YourActionName
  # Almost python code
  a += 1
  b += 1
```
Note: Each python statement is executed independently, they are the basic building
blocks. The python statements themselves are not parsed or interpreted directly by fizzbee.


### Atomic actions:
In TLA+ actions are atomic. In Fizz we explicitly state the atomicity of the action.
Atomic actions, mean there is no yield points between the statements.

In the above example, a and b are incremented atomically. If they both started at the same value,
they will end up at the same value.

### Serial action
Here, after each statement, there will be a yield point.
In the above example, a and b are incremented serially. 
So, if a=0 and b=0 at the beginning, the possible next steps are:

- a=1, b=0
- a=1, b=1

## Block modifiers
Every block can have a block modifier.
The block modifiers are: `atomic`, `serial`, `parallel`, `oneof`
`atomic` and `serial` are already explained.

### Oneof `oneof`
`oneof` is equivalent to \/ in TLA+.
For example,
```
action IncrementAny:
  oneof:
    a += 1
    b += 1
```
Here, either a or b will be incremented. Not both.
So, if a=0 and b=0 at the beginning, the possible next steps are:

- a=1, b=0
- a=0, b=1

### Parallel `parallel`
`parallel` implies the statements can be executed concurrently. 
So, they can be executed in any order. And there can be yield points between them,
so other actions could be interleaved between them.

```
action IncrementAny:
  oneof:
    a += 1
    b += 1
```
So, if a=0 and b=0 at the beginning, the possible next steps are:

- a=1, b=1
- a=0, b=1
- a=1, b=0

Note: These can be nested. But `atomic` can only contain `atomic` or `oneof` blocks.

## init action

`Init` is just another action called once at the beginning of the model checking. 
It is used to initialize the state of the model.

```
atomic action Init:
  a = 0
  b = 0
```

Some examples use the older way of defining the init action. 
It is still supported, but it will be removed soon. The new ways are more expressive and flexible.
as it can support non-determinism in the Init itself. 
Init can lead to multiple Init states. But the old way cannot express that.

```
# Old way
init:
  a = 0
  b = 0
```
Example with non-determinism in Init:
```
action Init:
  # More common usecases will use `any` statements
  oneof:
    atomic:
        a = 0
        b = 10
    atomic:
        a = 10
        b = 0
```


## Functions
Note: Functions is not fully implemented yet. Specifically, no parameters yet :(
It will be implemented soon.

Functions are defined with `func` keyword. It is syntactically similar to actions.

```
func TossACoin:
  oneof:
    return 0
    return 1
```

## Control Flow

### If-else
Same as python: if-elif-else
```
if a > b:
  b += 1
else:
  a += 1
```

### While
Same as python: while. (Note: Python's else clause on while is not supported)
```
while a < 5:
  a += 1
```
If a is 10 at the beginning, a will be 15 at the end.

### For
Same as python: for. (Note: Python's else clause on for is not supported)
Similar to `\A` in TLA+
```
for i in range(5):
  a += 1
```
If a is 10 at the beginning, a will be 15 at the end.

### Any statement
`any` is a non-deterministic statement. It is similar to `oneof` but for loop.
Similar to `\E` in TLA+
```
any i in range(5):
  a += i
```
If a is 10 at the beginning, there are 5 possible next states.
with a being 10, 11, 12, 13, 14 at the end.

## Invariants/Assertions
Invariants are the properties that should hold true at every state of the model.

There are two ways to define invariants. For most practical purposes, you'll need the first way.

Note: `assert` is a keyword in Python. So, we use `assertion`. 

```
always assertion FirstInvariant:
  return a == b
  
always assertion SecondInvariant2:
  # it can have loops, if-else, etc.
  return some_boolean_expression
  
```
Another way for most simple cases.
```
invariant:
  # Here each statement is a separate invariant.
  always a == 10
  always a < 10
  always b < 10
```

### always
This is equivalent to `[]` in TLA+. For safety properties.

### always eventually
This is equivalent to `[]<>` in TLA+. For liveness properties.

### eventually always
This is equivalent to `<>[]` in TLA+. For liveness properties.

Note: at this time, we don't have a way to nest these temporal operators.


## Guard clauses / Enabling conditions

Guard clauses are required to check deadlocks.
> Note: Deadlocks are not checked yet. But it will be implemented soon.

Guard clauses or enabling conditions are the predicates that tell whether
an action/transition is allowed or not. In some model checkers like Event-B, PRISM,
etc, the guard clauses are explicit. In TLA+, the guard clauses are implicit.
We follow the same approach as TLA+. The major benefit of this approach is, it is
typically how programmers write code. So, it is more natural to write and read.

An action is enabled if there is any valid transition (including self-loop).

For example:
```
# This action is always enabled
atomic action StartAlwaysEnabled:
  running = True

# This action is enabled only if running is False  
atomic action StartIfNotRunning:
  if not running:
    running = True
```
This approach works even in the case of nesting, method calls, etc.

```
# This action is always enabled
atomic action StartAlwaysEnabled:
  any node in nodes:
      running[node] = True

# This action is enabled only if running is False  
atomic action StartIfNotRunning:
  any node in nodes:
    if not running[node]:
        running[node] = True
```
In some extremely rare case, you might explicitly want to enable an action even if the predicate failed.
In that case, use pass statement, like in python.

```
atomic action ReportNodeFailure:
  if running:
    pass  # This action is enabled even if running is True
  else:
    msgs.append({type: "node_failure", node: node})

```

----
## Running the model checker

```
./fizz path_to_spec.fizz  

# Note: This will automatically build the binaries, the first time.
# But you might need to rebuild the binary after each `git pull`.
# with `bazel build //...`
```

### Example:
1. Create a example1/TwoCounter.fizz file with the following content:
```
atomic action Init:
    # Constants/Params are not supported yet
    C = ['a', 'b']
    counters = { key: 0 for key in C }

atomic action Next:
    any key in C:
        counters[key] = counters[key] + 1

```
2. Run the model checker.
Note: This is technically infinite model, but fizz sets a default limit of 100 actions depth.
`./fizz example1/TwoCounters.fizz`

```
./fizz example1/TwoCounter.fizz
Model checking example1/TwoCounters.json
dirPath: example1
configFileName: example1/fizz.yaml
fizz.yaml not found. Using default options
Nodes: 10201, elapsed: 225.399583ms
Time taken: 225.416167ms
Skipping dotfile generation. Too many nodes: 10201
PASSED: Model checker completed successfully
Max Depth 200
Writen 2 node files and 1 link files to dir example1/out/run_2024-03-05_12-53-45
```
You'll see the nodes and edges in the example1/out/run_2024-03-05_12-53-45 directory.
And the number of nodes are 10201. Since there are no assertions, obviously, the model checker succeeded.
Another point to note is: `Max Depth 200`, even though the default is 100. This is, every point at which
there is non-determinism will create a new node in the graph. It helps with debugging, but it is not counted
against the actions depth. 
In this example: Each any statement will create a new node, with one link for each alternative chosen.

3. Add a limit in the example1/fizz.yaml file.
```
options:
  max_actions: 4

```
4. Run the model checker again.
```
./fizz example1/TwoCounters.fizz
Model checking example1/TwoCounters.json
dirPath: example1
configFileName: example1/fizz.yaml
Nodes: 25, elapsed: 579.792µs
Time taken: 584.375µs
Writen graph dotfile: example1/out/run_2024-03-05_12-59-56/graph.dot
To generate png, run: 
dot -Tpng example1/out/run_2024-03-05_12-59-56/graph.dot -o graph.png && open graph.png
PASSED: Model checker completed successfully
Max Depth 8
Writen 1 node files and 1 link files to dir example1/out/run_2024-03-05_12-59-56
```
Notice the number of nodes is 25. And the max depth is 8.
When the number of nodes is small, it will generate a Graphviz graph.dot file. You can generate a png file from it.

On mac, you can open it with graphviz. Install using `brew install graphviz`
And copy and paste the command it printed. For example:
```
dot -Tpng example1/out/run_2024-03-05_12-59-56/graph.dot -o graph.png && open graph.png

```

5. Add an assertion to the TwoCounters.fizz file.
```
always assertion AlwaysBelowLimit:
    return all([counters[key] <= 2 for key in C])
```
Note: this assertion will fail.

6. Run the model checker again.
```
./fizz example1/TwoCounters.fizz                                                       
Model checking example1/TwoCounters.json
dirPath: example1
configFileName: example1/fizz.yaml
Nodes: 13, elapsed: 396.167µs
Time taken: 401.333µs
Writen graph dotfile: example1/out/run_2024-03-05_13-09-25/graph.dot
To generate png, run: 
dot -Tpng example1/out/run_2024-03-05_13-09-25/graph.dot -o graph.png && open graph.png
FAILED: Model checker failed
------
yield
--
state: {"C":"[\"a\", \"b\"]","counters":"{\"a\": 0, \"b\": 0}"}
------
Next
--
state: {"C":"[\"a\", \"b\"]","counters":"{\"a\": 0, \"b\": 0}"}
------
yield
--
state: {"C":"[\"a\", \"b\"]","counters":"{\"a\": 1, \"b\": 0}"}
------
Next
--
state: {"C":"[\"a\", \"b\"]","counters":"{\"a\": 1, \"b\": 0}"}
------
yield
--
state: {"C":"[\"a\", \"b\"]","counters":"{\"a\": 2, \"b\": 0}"}
------
Next
--
state: {"C":"[\"a\", \"b\"]","counters":"{\"a\": 2, \"b\": 0}"}
------
Any:"a"
--
state: {"C":"[\"a\", \"b\"]","counters":"{\"a\": 3, \"b\": 0}"}
Writen graph json: example1/out/run_2024-03-05_13-09-25/error-graph.json
Writen graph dotfile: example1/out/run_2024-03-05_13-09-25/error-graph.dot
To generate png, run: 
dot -Tpng example1/out/run_2024-03-05_13-09-25/error-graph.dot -o graph.png && open graph.png
```

The error trace is printed. You can also generate the graphviz graph.
And, the trace is also stored as a json file example1/out/run_2024-03-05_13-09-25/error-graph.json.
Eventually, we will be making a pretty printed HTML for viewing on the web, but not there yet.

7. Fix the assertion, and rerun model checker.
Quick fix is to change the assertion to 4.
```
always assertion AlwaysBelowLimit:
    return all([counters[key] <= 4 for key in C])
```

## Liveness check
Note: This is very early, and will be slow. But it is a work in progress.

FizzBee supports TLA+ style strict liveness check and also probabilistic evaluation.

To enable strict liveness check, add the following to the fizz.yaml file.

fizz.yaml
```
liveness: strict
```
The other possible value is `probabilistic`. To trigger it, see [Liveness with probabilistic evaluation](https://github.com/jayaprabhakar/fizzbee/blob/main/docs/fizzbee-quick-start-for-tlaplus-users.md#liveness-with-probabilistic-evaluation).
At present, it is not triggered automatically.
```
atomic action Init:
  n = 0
  any i in range(-2, 2)
    n = i

always eventually assertion StayPositive:
  return n == 0

atomic action Add:
  if n >= 3:
    n = 0
  else:
    n = n + 1

```
This model specifies, a will be initialized to be one of (-2,-1,0,1).
And, the numbers will be incremented at each step. But, it will be reset to 0, if it reaches 3.

The liveness property is that, n will eventually be 0. It can change to a non-zero number, but it should eventually be 0.

This spec will pass.

1. Make it fail by changing the add method to reset to 1 instead of 0, and rerun. You'll see the error trace.
2. Change the `always eventually` to `eventually always` and change the assertion to `n > 1`, and rerun.
```

always eventually assertion StayPositive:
  return n > 0
  
atomic action Add:
  if n >= 3:
    n = 1
  else:
    n = n + 1
```
3. Make it fail again, by resetting n to 0 in the Add method, and rerun.

## Probabilisitic Evaluation
Probabilistic evaluation is not implemented fully yet. This is a work in progress
and a bit spammy logs. Probabilistic evaluation is critical for performance evaluation
and for some liveness properties that cannot be evaluated using TLA+.
For example see: [What to do when strong fairness isn't strong enough?](https://groups.google.com/g/tlaplus/c/YTV6_o7hqHs/m/EmENa_6tBQAJ)

Get the out directory from the previous successful run.
```
bazel-bin/performance/performance_bin -s example1/out/run_2024-03-05_13-13-15/

// Too many logs. Will be improved soon.
// Finally, you will these lines:
   8: 0.06250000 state: {'C': '["a", "b"]', 'counters': '{"a": 4, "b": 0}'} / returns: {}
   9: 0.25000000 state: {'C': '["a", "b"]', 'counters': '{"a": 3, "b": 1}'} / returns: {}
  12: 0.37500000 state: {'C': '["a", "b"]', 'counters': '{"a": 2, "b": 2}'} / returns: {}
  17: 0.25000000 state: {'C': '["a", "b"]', 'counters': '{"a": 1, "b": 3}'} / returns: {}
  24: 0.06250000 state: {'C': '["a", "b"]', 'counters': '{"a": 0, "b": 4}'} / returns: {}
```
It shows the steady state probabilities of the states. In this example, the terminal states are obvious.
But, here the probabilities are calculated for each state.
You can see it follows a nice triangular distribution, with highest likelihood at `{"a": 2, "b": 2}`.

### Adding more complexity
Change the next state to either increment by 1 or 2.
```
atomic action Next:
    any key in C:
        oneof:
            counters[key] = counters[key] + 1
            counters[key] = counters[key] + 2
```
- Update the assertion to 8.
- Run the model checker again.
- Open the graph to see the new states.

Now, run the performance model again.
```
bazel-bin/performance/performance_bin -s example1/out/run_2024-03-05_13-18-26/

  21: 0.02691650 state: {'C': '["a", "b"]', 'counters': '{"a": 7, "b": 0}'} / returns: {}
  22: 0.01104736 state: {'C': '["a", "b"]', 'counters': '{"a": 8, "b": 0}'} / returns: {}
  24: 0.03570557 state: {'C': '["a", "b"]', 'counters': '{"a": 6, "b": 1}'} / returns: {}
  25: 0.04980469 state: {'C': '["a", "b"]', 'counters': '{"a": 6, "b": 2}'} / returns: {}
  27: 0.06787109 state: {'C': '["a", "b"]', 'counters': '{"a": 5, "b": 1}'} / returns: {}
  28: 0.09613037 state: {'C': '["a", "b"]', 'counters': '{"a": 5, "b": 2}'} / returns: {}
  38: 0.10491943 state: {'C': '["a", "b"]', 'counters': '{"a": 4, "b": 3}'} / returns: {}
  39: 0.07751465 state: {'C': '["a", "b"]', 'counters': '{"a": 4, "b": 4}'} / returns: {}
  49: 0.13769531 state: {'C': '["a", "b"]', 'counters': '{"a": 3, "b": 3}'} / returns: {}
  50: 0.10491943 state: {'C': '["a", "b"]', 'counters': '{"a": 3, "b": 4}'} / returns: {}
  68: 0.09613037 state: {'C': '["a", "b"]', 'counters': '{"a": 2, "b": 5}'} / returns: {}
  69: 0.04980469 state: {'C': '["a", "b"]', 'counters': '{"a": 2, "b": 6}'} / returns: {}
  87: 0.06787109 state: {'C': '["a", "b"]', 'counters': '{"a": 1, "b": 5}'} / returns: {}
  88: 0.03570557 state: {'C': '["a", "b"]', 'counters': '{"a": 1, "b": 6}'} / returns: {}
 114: 0.02691650 state: {'C': '["a", "b"]', 'counters': '{"a": 0, "b": 7}'} / returns: {}
 115: 0.01104736 state: {'C': '["a", "b"]', 'counters': '{"a": 0, "b": 8}'} / returns: {}
```

> **Note: By default, each non-determinism is assumed to be equally likely.**

We can assign custom probabilities to each oneof statement by adding labels.

### Adding custom probabilities
#### Add Labels to the fizz file
Labels for each statement is specified by prefixing with a label within backquote (`` ` ``) characters.
```
atomic action Next:
    any key in C:
        oneof:
            `add1` counters[key] = counters[key] + 1
            `add2` counters[key] = counters[key] + 2
```

This will not change the model checker. But, in the graph, the labels will be added for the arrows.
Lets add the performance model.

#### Create a performance model file
Create an yaml file, something like example1/perf_model1.yaml.
```
configs:
  Next.add1:
    probability: 0.75
  Next.add2:
    probability: 0.25
```
The labels are scoped to the Action/Function name. So, the label `add1` in the Next action will be `Next.add1`.

#### Run the performance model
Pass the performance model file to the performance model binary with `--perf` flag.

```
bazel-bin/performance/performance_bin -s example1/out/run_2024-03-05_13-36-24/ --perf example1/perf_model1.yaml

  21: 0.01777411 state: {'C': '["a", "b"]', 'counters': '{"a": 7, "b": 0}'} / returns: {}
  22: 0.00378466 state: {'C': '["a", "b"]', 'counters': '{"a": 8, "b": 0}'} / returns: {}
  24: 0.02807379 state: {'C': '["a", "b"]', 'counters': '{"a": 6, "b": 1}'} / returns: {}
  25: 0.02548981 state: {'C': '["a", "b"]', 'counters': '{"a": 6, "b": 2}'} / returns: {}
  27: 0.09249115 state: {'C': '["a", "b"]', 'counters': '{"a": 5, "b": 1}'} / returns: {}
  28: 0.09838343 state: {'C': '["a", "b"]', 'counters': '{"a": 5, "b": 2}'} / returns: {}
  38: 0.10868311 state: {'C': '["a", "b"]', 'counters': '{"a": 4, "b": 3}'} / returns: {}
  39: 0.04341030 state: {'C': '["a", "b"]', 'counters': '{"a": 4, "b": 4}'} / returns: {}
  49: 0.20722961 state: {'C': '["a", "b"]', 'counters': '{"a": 3, "b": 3}'} / returns: {}
  50: 0.10868311 state: {'C': '["a", "b"]', 'counters': '{"a": 3, "b": 4}'} / returns: {}
  68: 0.09838343 state: {'C': '["a", "b"]', 'counters': '{"a": 2, "b": 5}'} / returns: {}
  69: 0.02548981 state: {'C': '["a", "b"]', 'counters': '{"a": 2, "b": 6}'} / returns: {}
  87: 0.09249115 state: {'C': '["a", "b"]', 'counters': '{"a": 1, "b": 5}'} / returns: {}
  88: 0.02807379 state: {'C': '["a", "b"]', 'counters': '{"a": 1, "b": 6}'} / returns: {}
 114: 0.01777411 state: {'C': '["a", "b"]', 'counters': '{"a": 0, "b": 7}'} / returns: {}
 115: 0.00378466 state: {'C': '["a", "b"]', 'counters': '{"a": 0, "b": 8}'} / returns: {}
```
Notice the probability of the tail states are different now. The probabilities are calculated based on the custom probabilities.

Play with the probabilities and see how the probabilities change. The obvious,
examples are {0, 1}, {0.5,0.5}, {1, 0}, etc.

### Counters
In addition to finding the steady state probabilities, it can also find the cost to reach the states.
Although a lot more can be calculated, the script at present only exposes a very limited
set of features. One metric it emits now, is the cost to reach the steady state.

In this example, the cost to reach the steady state is the same as the cost to reach these terminal states.

In text books and papers and in PRISM, these are referred to as rewards in most cases and some times as costs.
To avoid having positive and negative connotations, we use the term counter.

In the performance model, you can define the counters.
In the perf_model1.yaml file, add the following:

```
configs:
  Next.add1:
    probability: 0.75
    counters:
      increments:
        numeric: 1
      latency:
        numeric: 1
  Next.add2:
    probability: 0.25
    counters:
      increments:
        numeric: 1
      latency:
        numeric: 10
```
Here it defines two counters. 
- The first counter `increments` counts the number of times, either of these labelled statements are executed.
- counter called `latency` with a numeric value of 1 for the `add1` label, but 10 for the `add2` label.


Run the performance model again.
```
bazel-bin/performance/performance_bin -s example1/out/run_2024-03-05_13-36-24/ --perf example1/perf_model1.yaml

# This will open two charts. Move the window to see the second chart.
# Just before the probabiltiies, it will print the counters.
# Including the mean, and also the CDFs
```

### Liveness with probabilistic evaluation
Currently, unlike TLA+, Fizzbee checks liveness using reachability analysis in markov chain.

For example, consider the following graph:

```
IDLE <---> PREPARED ---> DONE
```
This graph will always reach the DONE state. (As long as the transition
from PREPARED ---> DONE has a non-zero probability)

In TLA+, by default, this graph would fail, as the cycle IDLE <---> PREPARED,
can go on forever. But with strong fairness, this can be made to pass.
However, if you have two such nodes, and they can reach DONE only when both are in PREPARED state,
then strong fairness of TLA+ is not expressive enough. 
This is where probabilistic evaluation comes in.

```
0: ALICE:IDLE BOB:IDLE
1: ALICE:PREPARED BOB:IDLE
2: ALICE:IDLE BOB:PREPARED
3: ALICE:PREPARED BOB:PREPARED
4: ALICE:DONE BOB:DONE

0 <--> 1 <--> 3 ---> 4
^             ^     
|             |     
|----> 2 <----|
```
See this thread for more info: [What to do when strong fairness isn't strong enough?](https://groups.google.com/g/tlaplus/c/YTV6_o7hqHs/m/EmENa_6tBQAJ)

TLA+ can do that to. At present, Fizzbee does not implement TLA+ style liveness checks though.

#### Revert back to the basic TwoCounters.fizz file
```
atomic action Next:
    any key in C:
        counters[key] = counters[key] + 1
```
#### Add a liveness property
This will fail and show up in the probabilistic evaluation phase, as your see in the graph
```
eventually always assertion AlwaysReachLimit:
    # This would actually fail
    return any([counters[key] >= 4 for key in C])
```

#### Run the model checker
```
./fizz example1/TwoCounters.fizz                                                                            
Model checking example1/TwoCounters.json
dirPath: example1
configFileName: example1/fizz.yaml
Nodes: 25, elapsed: 1.518542ms
Time taken: 1.523958ms
Writen graph dotfile: example1/out/run_2024-03-05_21-50-30/graph.dot
To generate png, run: 
dot -Tpng example1/out/run_2024-03-05_21-50-30/graph.dot -o graph.png && open graph.png
PASSED: Model checker completed successfully
Max Depth 8
Writen 1 node files and 1 link files to dir example1/out/run_2024-03-05_21-50-30
```
Open the dot file and see the graph. You'll see some green nodes. These are the terminal states,
where the liveness property is satisfied. 

#### Run the performance model
Pass in the --source flag to the performance model binary.
Note: The .json file is generated in the first step as well. It is the Abstract Syntax Tree of the model.
```
bazel-bin/performance/performance_bin -s example1/out/run_2024-03-05_21-50-30/  --source example1/TwoCounters.json
### Ignore the spammy logs

AlwaysReachLimit eventually always
LIVE 1 8 0.06249999999992053 {'current': 0, 'failedInvariants': None, 'name': 'yield', 'returns': '{}', 'state': {'C': '["a", "b"]', 'counters': '{"a": 4, "b": 0}'}, 'stats': {'totalActions': 4, 'counts': {'Next': 4}}, 'threads': [], 'witness': [[False, True]]}
DEAD 1 9 0.24999999999968991 {'current': 0, 'failedInvariants': None, 'name': 'yield', 'returns': '{}', 'state': {'C': '["a", "b"]', 'counters': '{"a": 3, "b": 1}'}, 'stats': {'totalActions': 4, 'counts': {'Next': 4}}, 'threads': [], 'witness': [[False, False]]}
DEAD 1 12 0.37499999999953876 {'current': 0, 'failedInvariants': None, 'name': 'yield', 'returns': '{}', 'state': {'C': '["a", "b"]', 'counters': '{"a": 2, "b": 2}'}, 'stats': {'totalActions': 4, 'counts': {'Next': 4}}, 'threads': [], 'witness': [[False, False]]}
DEAD 1 17 0.24999999999968991 {'current': 0, 'failedInvariants': None, 'name': 'yield', 'returns': '{}', 'state': {'C': '["a", "b"]', 'counters': '{"a": 1, "b": 3}'}, 'stats': {'totalActions': 4, 'counts': {'Next': 4}}, 'threads': [], 'witness': [[False, False]]}
LIVE 1 24 0.06249999999992053 {'current': 0, 'failedInvariants': None, 'name': 'yield', 'returns': '{}', 'state': {'C': '["a", "b"]', 'counters': '{"a": 0, "b": 4}'}, 'stats': {'totalActions': 4, 'counts': {'Next': 4}}, 'threads': [], 'witness': [[False, True]]}
### More logs
```
If every state here is LIVE, then the liveness property is satisfied.

In this example, `eventually always` (`[]<>`) and `always eventually` (`<>[]`) are the same. But, in general, they are not.

`always eventually` is implemented as, in the steady state, every node should satisfy the liveness predicate.
`eventually always` is implemented as, make every node that satisfies the liveness predicate, into a terminal state. That is,
remove outbound edges, and make it a stuttering loop. Now, start from every node in the graph with equal probability,
and find the steady state. In the new steady state, if every node satisfies the liveness predicate, 
then the model's liveness property is satisfied.
