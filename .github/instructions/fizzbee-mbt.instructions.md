---
applyTo: "**"
---

# FizzBee Model-Based Testing (MBT)

FizzBee MBT generates exhaustive test sequences from a verified Fizz spec and runs them against the real system under test (SUT). Supported languages: **TypeScript**, **Go**, **Rust**, **Java**.

**Examples repository**: https://github.com/fizzbee-io/fizzbee-mbt-examples

## Installation

```bash
brew tap fizzbee-io/fizzbee && brew install fizzbee
brew tap fizzbee-io/fizzbee-mbt && brew install fizzbee-mbt
```

## Workflow

1. Write and verify a `.fizz` spec: `fizz spec.fizz` → PASSED
2. Scaffold adapter code: `fizz mbt-scaffold --lang <lang> --gen-adapter --out-dir <dir> spec.fizz`
3. Implement adapter methods in the generated `*_adapters` file
4. Start MBT server: `fizzbee-mbt-server --states_file <spec>/out/run_<timestamp>/`
5. Run tests

## Scaffold Commands

```bash
fizz mbt-scaffold --lang typescript --gen-adapter --out-dir src/fizztests/ spec.fizz
fizz mbt-scaffold --lang go --go-package myapp --gen-adapter --out-dir fizztests/ spec.fizz
fizz mbt-scaffold --lang rust --gen-adapter --out-dir src/fizztests/ spec.fizz
fizz mbt-scaffold --lang java --java-package org.example.myapp --gen-adapter --out-dir fizztests/ spec.fizz
```

Generated files per spec (do not edit interfaces or test files — only implement the adapters):

| File | Edit? |
|---|---|
| `*_interfaces.*` — role + model interfaces | No |
| `*_adapters.*` — stub methods to implement | **Yes** |
| `*_test.*` — test runner wiring | No |

---

## Language Feature Support

| Feature | TypeScript | Go | Rust | Java |
|---|---|---|---|---|
| Sequential testing | ✓ | ✓ | ✓ | ✓ |
| Concurrent testing | ✓ (cooperative) | ✓ | ✓ | ✓ |
| StateGetter | ✓ | ✓ | ✓ | ✓ |
| SnapshotStateGetter | ✓ | ✓ | ✓ | ✓ |
| Sentinel values | ✓ `IGNORE`/`ignored()` | ✓ | ✓ | ✓ |
| AfterActionHook | ✓ | — | — | — |
| OverridesProvider (fuzzing) | ✓ | — | — | — |
| Playwright utilities | ✓ | — | — | — |

---

## TypeScript / Playwright (Primary UI/E2E Target)

### Package

```bash
npm install @fizzbee/mbt
```

### Core Interfaces

```typescript
// Implement these on the model adapter as needed:

interface Model extends RoleMapper {
  init(): Promise<void>;       // before each test run
  cleanup(): Promise<void>;    // after each test run
  cleanupAll(): Promise<void>; // once at end of all tests (close browser etc.)
}

interface AfterActionHook {
  afterAction(): Promise<void>; // called after every action in sequential mode
}

interface OverridesProvider {
  provideOverrides(builder: OverridesBuilder, options: FuzzOptions): void | Promise<void>;
}

interface StateGetter {
  getState(): Promise<Record<string, any>>;
}

interface SnapshotStateGetter {
  snapshotState(): Promise<Record<string, any>>; // takes precedence over getState
}
```

### Playwright Best Practices

**Use `waitForDOMSettled`, never `waitForTimeout` or `slowMo`.**
The model checker generates thousands of action sequences; timing-based waits cause flakiness. `waitForDOMSettled` waits for DOM mutations to stop, which is precise and fast.

```typescript
import { waitForDOMSettled } from '@fizzbee/mbt/playwright';

// In AfterActionHook — stabilize after every action
async afterAction(): Promise<void> {
  await waitForDOMSettled(this.page, { debounceTimeout: 1 });
}
```

**Use semantic locators — `getByRole`, `getByTestId`.** These survive CSS/structure refactors.

```typescript
// ✓ Good
const input = page.getByRole('textbox', { name: 'What needs to be done?' });
const item = page.getByTestId('todo-item').nth(index);
const btn = item.getByRole('button', { name: 'Delete' });

// ✗ Brittle
const item = page.locator('.todo-item li').nth(index);
```

**Do not use `expect` in adapter action methods.** FizzBee validates state through `always assertion` blocks in the spec and `getState()` comparisons. Putting `expect` inside adapter actions fights the model and makes failures hard to trace.

```typescript
// ✓ Correct: just execute, let FizzBee assert
async actionUserAddsItem(args: Arg[]): Promise<any> {
  const title = args[0].toString();
  await page.getByRole('textbox', { name: 'What needs to be done?' }).fill(title);
  await page.getByRole('textbox', { name: 'What needs to be done?' }).press('Enter');
}

// ✗ Wrong: expect in action method
async actionUserAddsItem(args: Arg[]): Promise<any> {
  await page.fill('input', args[0].toString());
  await expect(page.locator('.todo-item')).toHaveCount(prevCount + 1); // Don't do this
}
```

**Disable parallel testing for UI.** Playwright browser contexts are not thread-safe across concurrent sequences.

```typescript
export function getTestOptions(): Record<string, any> {
  return {
    'max-seq-runs': 1000,
    'max-parallel-runs': 0,     // Always 0 for Playwright/UI tests
    'max-fuzz-seq-runs': 4,
    'max-actions': 10,
  };
}
```

**Read state from DOM or storage, not from UI appearance.** UI rendering can lag; localStorage/indexedDB reflects actual state immediately.

```typescript
async getState(): Promise<Record<string, any>> {
  const items = await this.page.evaluate((key) => {
    const raw = localStorage.getItem(key);
    return raw ? JSON.parse(raw) : [];
  }, 'react-todos');

  const todos = new Map<any, any>();
  for (const item of items) {
    todos.set(ignored(), {  // ignored() = unique placeholder key for unordered map
      id: IGNORE,           // IGNORE = field present but not compared (e.g. DB IDs)
      title: item.title,
      completed: item.completed,
    });
  }
  return { todos };
}
```

**Browser lifecycle — `init` / `cleanup` / `cleanupAll`:**

`PlaywrightEnv` is not part of the library — create your own helper (see the todomvc example in fizzbee-mbt-examples) or manage the browser directly:

```typescript
import { chromium, Browser, Page } from '@playwright/test';
import { waitForDOMSettled } from '@fizzbee/mbt/playwright';

export class MyModelAdapter implements MyModel, AfterActionHook {
  private browser?: Browser;
  private page?: Page;

  async init(): Promise<void> {
    // Called before EACH test run — create a fresh page
    if (!this.browser) {
      this.browser = await chromium.launch();
    }
    this.page = await this.browser.newPage();
    await this.page.goto('http://localhost:3000');
    // Initialize role adapters with this.page
  }

  async cleanup(): Promise<void> {
    // Called after EACH test run — close the page (lightweight)
    await this.page?.close();
  }

  async cleanupAll(): Promise<void> {
    // Called ONCE after all test runs complete — close the browser
    await this.browser?.close();
  }

  async afterAction(): Promise<void> {
    await waitForDOMSettled(this.page!, { debounceTimeout: 1 });
  }
}
```

### Sentinel Values

```typescript
import { IGNORE, ignored } from '@fizzbee/mbt';

// IGNORE: the field is present in state but its value is not compared.
// Use for: auto-generated IDs, timestamps, internal counters.
return { id: IGNORE, title: 'Buy milk', done: false };

// ignored(): creates a unique placeholder suitable as a map key.
// Use for: map/dict entries where you care about the values but not the exact key.
// Each call returns a distinct placeholder so the map has correct cardinality.
const todos = new Map();
todos.set(ignored(), { title: 'Buy milk', completed: false });
todos.set(ignored(), { title: 'Walk dog', completed: true });
```

### Fuzzing with OverridesProvider

Override Fizz spec constants with generated inputs per fuzz run:

```typescript
import fc from 'fast-check';
import { OverridesBuilder, FuzzOptions, Tuple } from '@fizzbee/mbt';

// Spec has: TODO_STRINGS = ("Buy milk", "Walk dog", "Read book")
// Override with randomly generated strings each fuzz run:
provideOverrides(builder: OverridesBuilder, options: FuzzOptions): void {
  const samples = fc.sample(
    fc.string({ unit: 'grapheme', minLength: 1, maxLength: 30 }),
    { seed: options.seed, numRuns: 3 }
  );
  // Use setTuple for Starlark tuple constants, setList for lists
  builder.setTuple('TODO_STRINGS', new Tuple(...samples));
}
```

OverridesBuilder API: `setString`, `setInt`, `setBool`, `setList`, `setDict`, `setTuple`, `setSet`, or generic `set` with auto-type detection.

### Running TypeScript Tests

```bash
# Terminal 1: start MBT server
fizzbee-mbt-server --states_file specs/myapp/out/run_<timestamp>/

# Terminal 2: run tests
npm run build
npm test

# Or with environment variable overrides
FIZZBEE_MBT_MAX_SEQ_RUNS=100 FIZZBEE_MBT_MAX_ACTIONS=5 npm test
```

Environment variables: `FIZZBEE_MBT_MAX_SEQ_RUNS`, `FIZZBEE_MBT_MAX_PARALLEL_RUNS`, `FIZZBEE_MBT_MAX_FUZZ_SEQ_RUNS`, `FIZZBEE_MBT_MAX_ACTIONS`, `FIZZBEE_MBT_SEQ_SEED`, `FIZZBEE_MBT_PARALLEL_SEED`, `FIZZBEE_MBT_FUZZ_SEQ_SEED`.

---

## Go

```go
import mbt "github.com/fizzbee-io/fizzbee/mbt/lib/go"

// go get github.com/fizzbee-io/fizzbee/mbt/lib/go

type MyRoleAdapter struct {
  sut *MySUT
}

func (a *MyRoleAdapter) ActionDoThing(args []mbt.Arg) (any, error) {
  return nil, a.sut.DoThing(context.Background())
}

// Optional: state assertions
func (a *MyRoleAdapter) GetState() (map[string]any, error) {
  v, err := a.sut.GetValue()
  return map[string]any{"value": v}, err
}

type MyModelAdapter struct {
  role *MyRoleAdapter
}

func (m *MyModelAdapter) Init() error {
  m.role = &MyRoleAdapter{sut: NewMySUT()}
  return nil
}
func (m *MyModelAdapter) Cleanup() error         { return m.role.sut.Close() }
func (m *MyModelAdapter) GetState() (map[string]any, error) {
  return nil, mbt.ErrNotImplemented
}
func (m *MyModelAdapter) GetRoles() (map[mbt.RoleId]mbt.Role, error) {
  return map[mbt.RoleId]mbt.Role{
    {RoleName: "MyRole", Index: 0}: m.role,
  }, nil
}
```

Running:
```bash
# Terminal 1
fizzbee-mbt-server --states_file specs/out/run_<timestamp>/

# Terminal 2
go test ./fizztests
go test ./fizztests -v
go test ./fizztests --seq-seed=12345   # reproduce a specific failure
```

---

## Rust

```rust
use fizzbee_mbt::{Model, Role, StateGetter, Arg, MbtError};

pub struct MyRoleAdapter { sut: MySUT }

#[async_trait]
impl MyRole for MyRoleAdapter {
  async fn action_do_thing(&self, args: &[Arg]) -> Result<Value, MbtError> {
    self.sut.do_thing().await?;
    Ok(Value::None)
  }
}

pub struct MyModelAdapter { role: Option<MyRoleAdapter> }

#[async_trait]
impl Model for MyModelAdapter {
  async fn init(&mut self) -> Result<(), MbtError> {
    self.role = Some(MyRoleAdapter { sut: MySUT::new().await? });
    Ok(())
  }
  async fn cleanup(&mut self) -> Result<(), MbtError> { Ok(()) }
}
```

---

## Java

```java
public class MyRoleAdapter implements MyRole {
  private final MySUT sut;

  @Override
  public Object actionDoThing(List<Arg> args) throws Exception {
    sut.doThing();
    return null;
  }

  // Optional state assertions
  @Override
  public Map<String, Object> getState() {
    return Map.of("value", sut.getValue());
  }
}

public class MyModelAdapter implements MyModel {
  private MyRoleAdapter role;

  @Override
  public void init() {
    this.role = new MyRoleAdapter(new MySUT());
  }

  @Override
  public Map<RoleId, Role> getRoles() {
    return Map.of(new RoleId("MyRole", 0), role);
  }
}
```

---

## Starlark → Host Language Types

| Starlark | TypeScript | Go | Rust |
|---|---|---|---|
| `int` | `number` | `int64` | `i64` |
| `float` | `number` | `float64` | `f64` |
| `bool` | `boolean` | `bool` | `bool` |
| `string` | `string` | `string` | `String` |
| `list`/`tuple`/`set` | `any[]` | `[]any` | `Vec<Value>` |
| `dict` | `Map<any,any>` | `map[any]any` | `HashMap` |
| `None` | `null` | `nil` | `None` |
| record/struct | `Record<string,any>` | `map[string]any` | `HashMap<String,Value>` |

---

## Implementation Tips (All Languages)

- Implement `init()` / `getRoles()` first, run tests, then implement actions one by one
- Unimplemented actions return `NotImplementedError` / `mbt.ErrNotImplemented` — the runner skips them without failing, enabling incremental development
- `cleanup()` runs between every test trace — keep it fast
- `cleanupAll()` (TypeScript) / final teardown runs once — put expensive teardown here
- For nondeterministic actions, `args[i]` contains the chosen value in the same order as `any` statements appear in the spec action
- State comparison uses the Fizz spec's state structure — field names must match exactly
- Do not add fields to `getState()` that aren't in the Fizz spec's role state — extra fields are ignored but can cause confusion
