# HR parental leave: Bedrock Automated Reasoning parity

This example reproduces the HR leave policy that AWS uses throughout the
[Amazon Bedrock Automated Reasoning checks](https://docs.aws.amazon.com/bedrock/latest/userguide/automated-reasoning-checks-concepts.html)
docs, and checks chatbot answers against it with FizzBee instead of an SMT
solver. Every Bedrock verdict (VALID, SATISFIABLE, INVALID, IMPOSSIBLE,
NO_TRANSLATIONS) has a direct FizzBee equivalent, and all 11 test claims
come back with the expected verdict.

The point is parity, not advantage. This policy is a set of static
eligibility rules, which is exactly what SMT is built for. It shows that
FizzBee covers the same ground before moving to stateful policies
(book, modify, cancel) where single-claim checks are not enough.

## Files

| File | What it is |
|---|---|
| `HrLeavePolicy.fizz` | The policy: variables plus rules R1 and R2 |
| `claims.json` | Question, LLM answer, premises, claim and expected verdict for each test |
| `check_claims.py` | Runs FizzBee on each claim and prints the verdict |

## Run it

```bash
python3 check_claims.py --fizz /path/to/fizz -v
```

Tested with FizzBee v0.5.4. The whole run is 23 model-checker calls of
about 0.3 seconds each.

```
T1   VALID            expected VALID            ok    Yes, you are eligible for parental leave.
T2   VALID            expected VALID            ok    Yes, you are eligible for parental leave.
T3   VALID            expected VALID            ok    No, only full-time employees are eligible for leave of absence.
T4   INVALID          expected INVALID          ok    No, you are not eligible for parental leave yet.
T5   INVALID          expected INVALID          ok    Yes, part-time employees can take a leave of absence.
T6   SATISFIABLE      expected SATISFIABLE      ok    Yes, full-time employees are eligible for parental leave.
T7   SATISFIABLE      expected SATISFIABLE      ok    No, part-time employees are not eligible for parental leave.
T8   SATISFIABLE      expected SATISFIABLE      ok    Yes, you are eligible for parental leave.
T9   IMPOSSIBLE       expected IMPOSSIBLE       ok    Yes, you are eligible for parental leave.
T10  IMPOSSIBLE       expected IMPOSSIBLE       ok    Yes, you can take parental leave too.
T11  NO_TRANSLATIONS  expected NO_TRANSLATIONS  ok    Today's special is lasagna.

11/11 claims matched the expected verdict
```

With `-v`, each verdict also prints its witness worlds, the same thing
Bedrock returns as `claimsTrueScenario` and `claimsFalseScenario`. For T6:

```
claimsTrueScenario:  {"eligibleForParentalLeave": true,  "isFullTime": true, "tenureMonths": 0, ...}
claimsFalseScenario: {"eligibleForParentalLeave": false, "isFullTime": true, "tenureMonths": 0, ...}
```

The false scenario is the explanation: the answer forgot the tenure
condition, and a full-time employee with 0 months is not covered by R1.

## The policy

AWS publishes one SMT-LIB rule for this example:

```lisp
;; If the employee is full-time AND has worked for more than 12 months,
;; then they are eligible for parental leave.
(=> (and isFullTime (> tenureMonths 12)) eligibleForParentalLeave)
```

and one test case that implies a second rule: "No, only full-time employees
are eligible for leave of absence." In FizzBee:

```python
atomic action Scenario:
    require not known
    isFullTime = oneof [True, False]
    tenureMonths = oneof range(0, MAX_TENURE_MONTHS + 1)
    eligibleForParentalLeave = oneof [True, False]
    eligibleForLeaveOfAbsence = oneof [True, False]

    # R1
    require (not (isFullTime and tenureMonths > 12)) or eligibleForParentalLeave
    # R2
    require (not eligibleForLeaveOfAbsence) or isFullTime

    known = True
```

An SMT policy is a list of variables and assertions; every assignment that
satisfies all assertions is a world the policy allows. `oneof` branches over
every assignment and `require` prunes the ones that break a rule, so the
states reachable after `Scenario` are exactly the allowed worlds (510 of
them here).

## Verdict mapping

A claim has premises P (the facts in the question) and a claim C (what the
answer asserts). FizzBee answers two reachability questions about the
policy:

* is a world reachable where `P and C`?
* is a world reachable where `P and not C`?

| Bedrock verdict | Meaning | `P and C` | `P and not C` | FizzBee assertion that passes |
|---|---|---|---|---|
| VALID | Claim follows from the rules | reachable | unreachable | `always assertion: return not P or C`, and `exists assertion: return P` |
| SATISFIABLE | Consistent, but true only in some worlds | reachable | reachable | `exists assertion: return P and C` and `exists assertion: return P and not C` |
| INVALID | Claim contradicts the rules | unreachable | reachable | `always assertion: return not P or not C`, and `exists assertion: return P` |
| IMPOSSIBLE | Premises contradict the rules or each other | unreachable | unreachable | `always assertion: return not P` |
| NO_TRANSLATIONS | Claim uses concepts the policy has no variables for | n/a | n/a | the expression names an undefined variable |

In the spec each P is guarded by `known`, so the initial state before
`Scenario` is not counted as a world.

In dynamic logic terms, VALID is `[Scenario](P → C)` and SATISFIABLE is
`<Scenario>(P ∧ C) ∧ <Scenario>(P ∧ ¬C)`.

`check_claims.py` checks each probe as `always assertion: return not (known
and X)`. That is the same question as `exists assertion: return known and X`,
but when X is reachable the failing `always` assertion prints a trace, and
its last state is the witness world.

One more check runs first: if no world is reachable at all, the rules
contradict each other. Bedrock reports that case as IMPOSSIBLE for every
claim; the script reports it once for the policy.

## The test claims

AWS publishes the rule and three question-and-answer pairs (T1, T2, T3) for
this policy, plus the contradictory-input example used in T9. The others
were written for this example to cover each verdict.

| ID | Answer being checked | Verdict | Why |
|---|---|---|---|
| T1 | Full-time, 18 months: "Yes, eligible for parental leave" | VALID | R1 applies (AWS walkthrough) |
| T2 | Full-time, 2 years: "Yes, eligible" | VALID | R1 applies; tests 2 years = 24 months (AWS QnA example) |
| T3 | Part-time: "No, only full-time can take leave of absence" | VALID | R2 (AWS CLI example) |
| T4 | Full-time, 18 months: "No, not eligible yet" | INVALID | A hallucinated answer to T1 |
| T5 | Part-time: "Yes, you can take leave of absence" | INVALID | Contradicts R2 |
| T6 | Full-time: "Yes, eligible" | SATISFIABLE | Incomplete: tenure was not checked |
| T7 | Part-time, 3 years: "No, not eligible" | SATISFIABLE | Policy gap: R1 says who is eligible, never who is not |
| T8 | Full-time, exactly 12 months: "Yes, eligible" | SATISFIABLE | Boundary: AWS's prose says "12+ months", its SMT rule says `> 12` |
| T9 | "I'm full-time and also part-time" | IMPOSSIBLE | Contradictory premises (AWS example) |
| T10 | Part-time but approved for leave of absence | IMPOSSIBLE | Premises contradict R2 |
| T11 | "Today's special is lasagna" | NO_TRANSLATIONS | No policy variable for the cafeteria |

T7 and T8 are worth a second look. They are not wrong answers, they are
policy gaps, and the checker finds them the same way Bedrock does. In T8 the
answer matches the AWS prose ("12+ months") but not the AWS rule (`> 12`),
so whichever one is the real policy, one of them needs fixing.

## Differences from Bedrock

* **Translation is out of scope.** Bedrock uses LLMs to turn the question
  and answer into premises and claims. Here they are written by hand in
  `claims.json`. NO_TRANSLATIONS is approximated by checking the expression
  for names that are not policy variables, and TRANSLATION_AMBIGUOUS (two
  LLM translations that disagree) has no equivalent yet.
* **Integers are bounded.** SMT reasons about all integers; FizzBee
  enumerates, so `tenureMonths` is limited to 0..120. A claim about 15
  years of tenure would need a larger bound.
* **Model checking scales differently.** Enumeration is cheap for a policy
  this size and grows with the product of variable domains. SMT handles
  large arithmetic domains symbolically.

## What FizzBee adds

Nothing yet for this policy, which is the point of a parity example. The
difference shows up when a policy has history: a rule that depends on what
happened earlier (booked within 24 hours, cabin changed, nothing flown yet)
is a property of a sequence of actions, not a single world. A single-claim
SMT check validates one answer at a time; FizzBee can also search every
sequence of actions for a path the policy did not intend.

## Known issues

In FizzBee v0.5.4, a `require` in `Init` that prunes any `oneof` branch
discards every Init state, and the run then panics in
`CheckSimpleExistsWitness`. That is why the rules live in the `Scenario`
action rather than in `Init`.
