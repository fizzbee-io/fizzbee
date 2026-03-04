---
description: >
  Create model-based tests (MBT) connecting a FizzBee spec to a real system
  under test (SUT). Use when the user has a .fizz spec and wants to generate
  and run tests against a TypeScript/Playwright UI, Go service, Rust library,
  or Java application. Also use when working with fizzbee-mbt adapter code
  in any of these languages.
---

# FizzBee Model-Based Testing (MBT)

FizzBee MBT generates exhaustive test sequences from a verified Fizz spec and runs them against the real system under test (SUT).

**Examples**: https://github.com/fizzbee-io/fizzbee-mbt-examples

## Installation

```bash
# fizzbee (model checker)
brew tap fizzbee-io/fizzbee && brew install fizzbee

# fizzbee-mbt (test runner + server)
brew tap fizzbee-io/fizzbee-mbt && brew install fizzbee-mbt
```

---

## Language Support Overview

| Feature | TypeScript | Go | Rust | Java |
|---|---|---|---|---|
| Sequential testing | ✓ | ✓ | ✓ | ✓ |
| Concurrent testing | ✓ (cooperative) | ✓ | ✓ | ✓ |
| StateGetter | ✓ | ✓ | ✓ | ✓ |
| SnapshotStateGetter | ✓ | ✓ | ✓ | ✓ |
| Sentinel values (IGNORE) | ✓ | ✓ | ✓ | ✓ |
| AfterActionHook | ✓ | — | — | — |
| OverridesProvider (fuzzing) | ✓ | — | — | — |
| Playwright utilities | ✓ | — | — | — |

TypeScript and Go have the broadest feature support. TypeScript is the primary language for UI/E2E testing.

---

## Workflow

1. Write and verify the `.fizz` spec: `fizz spec.fizz` → PASSED
2. Scaffold: `fizz mbt-scaffold --lang <lang> --gen-adapter --out-dir <dir> spec.fizz`
3. Implement the adapter methods
4. Start the MBT server: `fizzbee-mbt-server --states_file out/run_<timestamp>/`
5. Run tests

---

## Scaffold Commands

```bash
# TypeScript
fizz mbt-scaffold --lang typescript --gen-adapter --out-dir src/fizztests/ spec.fizz

# Go
fizz mbt-scaffold --lang go --go-package myapp --gen-adapter --out-dir fizztests/ spec.fizz

# Rust
fizz mbt-scaffold --lang rust --gen-adapter --out-dir src/fizztests/ spec.fizz

# Java
fizz mbt-scaffold --lang java --java-package org.example.myapp --gen-adapter --out-dir fizztests/ spec.fizz
```

Three files are generated per spec:

| File | Description | Edit? |
|---|---|---|
| `*_interfaces.ts/go/rs/java` | Role + model interfaces from spec | No |
| `*_adapters.ts/go/rs/java` | Stub methods to implement | **Yes** |
| `*_test.ts/go/rs/java` | Test runner wiring | No |

---

## TypeScript / Playwright

### Setting Up

```bash
npm install @fizzbee/mbt
```

### Model Structure

```typescript
import { Model, Role, StateGetter, AfterActionHook, OverridesProvider,
         OverridesBuilder, FuzzOptions, Arg, IGNORE, ignored } from '@fizzbee/mbt';
import { waitForDOMSettled } from '@fizzbee/mbt/playwright';

export class MyModelAdapter implements MyModel, AfterActionHook, OverridesProvider {
  private page?: Page;

  async init(): Promise<void> {
    // Called before each test run — set up browser page, reset app state
    this.page = await browser.newPage();
    await this.page.goto('http://localhost:3000');
  }

  async cleanup(): Promise<void> {
    // Called after each test run — close the page
    await this.page?.close();
  }

  async cleanupAll(): Promise<void> {
    // Called once at the very end — close the browser
    await browser.close();
  }

  // Called after every action — wait for UI to settle
  async afterAction(): Promise<void> {
    await waitForDOMSettled(this.page!, { debounceTimeout: 1 });
  }

  async getRoles(): Promise<Map<string, Role>> {
    return new Map([['MyRole#0', this.myRole]]);
  }
}
```

### Playwright Best Practices

**Use `waitForDOMSettled`, never `waitForTimeout` or `slowMo`:**
```typescript
// ✓ Precise: waits for DOM mutations to stop
await waitForDOMSettled(page, { debounceTimeout: 1 });

// ✗ Fragile: arbitrary delay, flaky on slow machines
await page.waitForTimeout(500);
```

**Use role-based and test-ID locators, not CSS selectors:**
```typescript
// ✓ Semantic locators — survive UI refactors
const input = page.getByRole('textbox', { name: 'What needs to be done?' });
const item = page.getByTestId('todo-item').nth(index);
const btn = item.getByRole('button', { name: 'Delete' });

// ✗ Brittle — breaks on class/structure changes
const item = page.locator('.todo-item').nth(index);
```

**Do not use `expect` assertions in adapter actions.** FizzBee drives the assertions from the spec's `always assertion` blocks and state comparison — `expect` in the adapter fights against that model. Just execute the action:
```typescript
async actionUserAddsItem(args: Arg[]): Promise<any> {
  const title = args[0].toString();
  await page.getByRole('textbox', { name: 'What needs to be done?' }).fill(title);
  await page.getByRole('textbox', { name: 'What needs to be done?' }).press('Enter');
  // No expect() here — FizzBee handles validation
}
```

**Use `getState()` to expose DOM/storage state for assertion:**
```typescript
async getState(): Promise<Record<string, any>> {
  const items = await page.evaluate(() => {
    const stored = localStorage.getItem('todos');
    return stored ? JSON.parse(stored) : [];
  });

  const todos = new Map<any, any>();
  for (const item of items) {
    todos.set(ignored(), {    // ignored() = unique placeholder key
      id: IGNORE,             // IGNORE = field not compared
      title: item.title,
      completed: item.completed,
    });
  }
  return { todos };
}
```

**No parallel tests for UI:**
```typescript
export function getTestOptions(): Record<string, any> {
  return {
    'max-seq-runs': 1000,
    'max-parallel-runs': 0,     // Playwright is not thread-safe
    'max-fuzz-seq-runs': 4,
    'max-actions': 10,
  };
}
```

### Sentinel Values

Use when state contains non-deterministic or intentionally ignored fields:

```typescript
import { IGNORE, ignored } from '@fizzbee/mbt';

// IGNORE: field exists but value is not compared (e.g. DB-generated IDs)
return { id: IGNORE, title: 'Buy milk', done: false };

// ignored(): unique placeholder for map keys that aren't compared
// Use a fresh ignored() per entry so the map has the right size
const todos = new Map();
todos.set(ignored(), { title: 'Buy milk', completed: false });
todos.set(ignored(), { title: 'Walk dog', completed: true });
return { todos };
```

### Fuzzing with OverridesProvider

Override Fizz constants with generated test data:

```typescript
import fc from 'fast-check';
import { OverridesBuilder, FuzzOptions, Tuple } from '@fizzbee/mbt';

provideOverrides(builder: OverridesBuilder, options: FuzzOptions): void {
  // Generate diverse inputs using fast-check, reproducible via seed
  const samples = fc.sample(
    fc.string({ unit: 'grapheme', minLength: 1, maxLength: 30 }),
    { seed: options.seed, numRuns: 3 }
  );
  builder.setTuple('TODO_STRINGS', new Tuple(...samples));
}
```

The Fizz spec defines `TODO_STRINGS` as a constant; the override replaces it per fuzz run with generated values.

---

## Go

```go
type MyRoleAdapter struct { sut *MySUT }

func (a *MyRoleAdapter) ActionDoThing(args []mbt.Arg) (any, error) {
    return nil, a.sut.DoThing(context.Background())
}

// Optional: expose state for assertions
func (a *MyRoleAdapter) GetState() (map[string]any, error) {
    v, err := a.sut.GetValue()
    return map[string]any{"value": v}, err
}

type MyModelAdapter struct { role *MyRoleAdapter }

func (m *MyModelAdapter) Init() error {
    m.role = &MyRoleAdapter{sut: NewMySUT()}
    return nil
}
func (m *MyModelAdapter) Cleanup() error { return m.role.sut.Close() }
func (m *MyModelAdapter) GetState() (map[string]any, error) {
    return nil, mbt.ErrNotImplemented
}
func (m *MyModelAdapter) GetRoles() (map[mbt.RoleId]mbt.Role, error) {
    return map[mbt.RoleId]mbt.Role{
        {RoleName: "MyRole", Index: 0}: m.role,
    }, nil
}
```

Run tests:
```bash
# Terminal 1
fizzbee-mbt-server --states_file specs/out/run_<timestamp>/

# Terminal 2
go test ./fizztests
go get github.com/fizzbee-io/fizzbee/mbt/lib/go
```

---

## Starlark → Host Language Type Mapping

| Starlark | TypeScript | Go | Rust |
|---|---|---|---|
| `int` | `number` (int) | `int64` | `i64` |
| `float` | `number` | `float64` | `f64` |
| `bool` | `boolean` | `bool` | `bool` |
| `string` | `string` | `string` | `String` |
| `list`/`tuple`/`set` | `any[]` | `[]any` | `Vec<Value>` |
| `dict` | `Map<any,any>` | `map[any]any` | `HashMap<Value,Value>` |
| `None` | `null` | `nil` | `None` |
| record/struct | `Record<string,any>` | `map[string]any` | `HashMap<String,Value>` |

---

## Implementation Tips

- Implement `Init()` + `getRoles()` first, run tests, then implement actions one by one
- Return `NotImplementedError` / `mbt.ErrNotImplemented` for unimplemented actions — the runner skips them rather than failing
- Keep `cleanup()` lightweight — it runs between every test trace
- Keep `cleanupAll()` for expensive teardown (close browser, connection pool) — runs once at end
- For concurrent testing: if the SUT isn't thread-safe, set `max-parallel-runs: 0`
