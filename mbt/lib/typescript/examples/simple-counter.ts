/**
 * Simple counter example demonstrating FizzBee MBT usage in TypeScript
 */

// When running locally, import from the built dist directory
// When published to npm, users would do: import { ... } from '@fizzbee/mbt'
import { Model, StateGetter, Role, ActionFunc, runTests } from '../dist/index';

/**
 * Counter role - represents a simple counter
 */
class Counter implements Role, StateGetter {
  private value: number = 0;

  increment(): void {
    this.value++;
  }

  decrement(): void {
    this.value--;
  }

  getValue(): number {
    return this.value;
  }

  async getState(): Promise<Record<string, any>> {
    return { "value": this.value };
  }
}

/**
 * CounterModel - manages counter instances
 */
class CounterModel implements Model {
  private counters: Map<string, Counter> = new Map();

  async init(): Promise<void> {
    console.log('Initializing CounterModel...');
    // Create two counter instances
    this.counters.set('Counter#0', new Counter());
    // this.counters.set('Counter#1', new Counter());
  }

  async cleanup(): Promise<void> {
    console.log('Cleaning up CounterModel...');
    this.counters.clear();
  }

  async getRoles(): Promise<Map<string, Role>> {
    return this.counters as Map<string, Role>;
  }

  // async getState(): Promise<Record<string, any>> {
  //   const state: Record<string, any> = {};
  //   for (const [key, counter] of this.counters) {
  //     state[key] = await counter.getState();
  //   }
  //   return state;
  // }
}

// Define actions
async function incrementAction(instance: Counter, _args: any[]): Promise<any> {
  console.log("Calling incrementAction");
  instance.increment();
  // return instance.getValue();
}

async function decrementAction(instance: Counter, _args: any[]): Promise<any> {
  console.log("Calling decrementAction");
  instance.decrement();
  return instance.getValue();
}

async function getValueAction(instance: Counter, _args: any[]): Promise<any> {
  console.log("Calling getValueAction");
  return instance.getValue();
}

// Create action registry
const actionsRegistry = new Map<string, Map<string, ActionFunc>>();
const counterActions = new Map<string, ActionFunc>();
counterActions.set('Inc', incrementAction);
counterActions.set('Dec', decrementAction);
counterActions.set('Get', getValueAction);
actionsRegistry.set('Counter', counterActions);

// Main function
async function main() {
  // Create model
  const model = new CounterModel();

  // Configure test options
  // These can be overridden by environment variables:
  // FIZZBEE_MBT_MAX_SEQ_RUNS, FIZZBEE_MBT_MAX_PARALLEL_RUNS, FIZZBEE_MBT_MAX_ACTIONS
  const options = {
    'max-seq-runs': 10,
    'max-parallel-runs': 5,
    'max-actions': 100
  };

  // Run tests
  try {
    console.log("Starting tests...");
    await runTests(model, actionsRegistry, options);
    console.log('Tests completed successfully');
  } catch (error) {
    console.error('Tests failed:', error);
    process.exit(1);
  }
}

// Run if this is the main module
if (require.main === module) {
  main();
}
