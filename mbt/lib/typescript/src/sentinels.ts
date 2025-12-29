/**
 * Sentinel values for partial state matching in model-based testing.
 *
 * Use these in your getState() implementation to indicate
 * fields that should be ignored during state comparison.
 *
 * @example
 * ```typescript
 * import { IGNORE, ignored } from '@fizzbee/mbt';
 *
 * class TodoRole implements Role, StateGetter {
 *   async getState(): Promise<Record<string, any>> {
 *     return {
 *       todos: todos.map(t => ({
 *         id: IGNORE,           // ID not visible in DOM
 *         title: t.title,
 *         completed: t.completed
 *       }))
 *     };
 *   }
 * }
 *
 * // For map keys that should be ignored (e.g., non-deterministic UUIDs):
 * class ModelRole implements Role, StateGetter {
 *   async getState(): Promise<Record<string, any>> {
 *     const todos: Record<any, any> = {};
 *     for (const todo of todosArray) {
 *       todos[ignored()] = { // Each ignored() creates a unique instance
 *         id: ignored(),
 *         title: todo.title,
 *         completed: todo.completed
 *       };
 *     }
 *     return { todos };
 *   }
 * }
 * ```
 */

/**
 * Ignored class: Represents a value that should be ignored during comparison.
 * Each instance is unique, allowing multiple ignored keys in maps.
 */
export class Ignored {
  // Each instance is unique by reference (identity-based equality)
}

/**
 * Factory function to create a new Ignored instance.
 * Use this when you need unique ignored values (e.g., for map keys).
 */
export function ignored(): Ignored {
  return new Ignored();
}

/**
 * IGNORE constant: Field should be completely ignored during state comparison.
 * Use this for values that should be ignored.
 *
 * Use this for:
 * - Non-deterministic values (UUIDs, timestamps, random IDs) as values
 * - Implementation details not visible in the UI
 * - Fields that exist in the model but can't be observed in the SUT
 *
 * For map keys that need to be ignored, use ignored() function instead.
 */
export const IGNORE = Symbol.for('fizzbee.mbt.IGNORE');

/**
 * Type representing a sentinel value.
 */
export type Sentinel = typeof IGNORE | Ignored;
