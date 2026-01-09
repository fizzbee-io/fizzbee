import type { OverridesBuilder } from './overrides';

/**
 * Base interface that all role interfaces should implement.
 * Roles represent the components/actors in your system under test.
 */
export interface Role {
  // Marker interface - no required methods
}

/**
 * Interface for roles that can return their current state.
 * When implemented, tests can use this to assert the state of the role.
 * If both GetState and SnapshotState are implemented, SnapshotState takes precedence.
 */
export interface StateGetter {
  /**
   * Returns the current state without guaranteeing thread safety.
   * @returns A promise that resolves to a map of state key-value pairs
   */
  getState(): Promise<Record<string, any>>;
}

/**
 * Interface for roles that can return a consistent snapshot of their state.
 * When implemented, this method would be used concurrently with the role's actions
 * to test intermediate states.
 * If both GetState and SnapshotState are implemented, SnapshotState takes precedence.
 */
export interface SnapshotStateGetter {
  /**
   * Returns a consistent, concurrency-safe snapshot of the state.
   * @returns A promise that resolves to a map of state key-value pairs
   */
  snapshotState(): Promise<Record<string, any>>;
}

/**
 * Interface for mapping role names/IDs to role instances.
 */
export interface RoleMapper {
  /**
   * Returns all role instances managed by this model.
   * @returns A promise that resolves to a map of RoleId to Role instances
   */
  getRoles(): Promise<Map<string, Role>>;
}

/**
 * Optional interface for models that need post-action stabilization.
 * Implement this when your system has asynchronous effects that need to
 * complete before the next action or state verification.
 *
 * This hook is ONLY called in sequential testing mode, not in concurrent mode.
 *
 * Examples of when to implement:
 * - UI frameworks with async rendering (React, Vue, etc.)
 * - Systems with event loops or message queues
 * - Eventually consistent systems that stabilize quickly
 *
 * When NOT to implement:
 * - Pure synchronous systems
 * - Systems with long-running async operations
 * - Highly concurrent systems (Deal with them in the actions themselves)
 */
export interface AfterActionHook {
  /**
   * Called after each action in sequential testing mode.
   * Use this to wait for async operations, UI updates, or eventual
   * consistency before the next action or state verification.
   *
   * @returns A promise that resolves when the system has stabilized
   */
  afterAction(): Promise<void>;
}

/**
 * Options passed to override providers during fuzzing/property-based testing.
 * Contains runtime information like seeds for reproducibility.
 */
export interface FuzzOptions {
  /**
   * Random seed for reproducible fuzzing.
   * Use this seed with property-based testing libraries like fast-check
   * to generate deterministic random values.
   */
  seed: number;
}

/**
 * Optional interface for models that need to provide variable overrides.
 * Implement this when you want to override global variables/constants before
 * model initialization. This is similar to the pre-init-hook in the FizzBee
 * model checker.
 *
 * Common use cases:
 * - Overriding constants for different test scenarios
 * - Setting environment-specific values
 * - Parameterizing model behavior for different test runs
 *
 * The override values support standard Starlark types:
 * - Primitives: string, int, bool
 * - Collections: list (array), dict (object/Map), tuple, set
 *
 * Example:
 * ```typescript
 * class MyModel implements Model, OverridesProvider {
 *   provideOverrides(builder: OverridesBuilder, options: FuzzOptions): void {
 *     // Use the seed for reproducible random generation
 *     const randomValue = generateWithSeed(options.seed);
 *     builder.setInt('MAX_RETRIES', 5)
 *            .setString('API_ENDPOINT', 'http://localhost:8080')
 *            .setList('ALLOWED_METHODS', ['GET', 'POST', 'PUT']);
 *   }
 *   // ... rest of implementation
 * }
 * ```
 */
export interface OverridesProvider {
  /**
   * Provides variable overrides before model initialization.
   * This method is called before init() to set up global variables/constants.
   *
   * @param builder - Builder for setting variable overrides with type safety
   * @param options - Fuzzing options including seed for reproducibility
   * @returns A promise or void
   */
  provideOverrides(builder: OverridesBuilder, options: FuzzOptions): void | Promise<void>;
}

/**
 * Main interface for the system model.
 * Your model should implement this interface to integrate with the MBT framework.
 */
export interface Model extends RoleMapper {
  /**
   * Initializes the model before each test run.
   * @returns A promise that resolves when initialization is complete
   */
  init(): Promise<void>;

  /**
   * Cleans up the model after each test run.
   * @returns A promise that resolves when cleanup is complete
   */
  cleanup(): Promise<void>;

  cleanupAll(): Promise<void>;
}

/**
 * Function signature for action methods.
 * @param instance The role instance or model on which to execute the action
 * @param args Array of arguments passed to the action
 * @returns A promise that resolves to the action's return value
 */
export type ActionFunc = (instance: any, args: any[]) => Promise<any>;
