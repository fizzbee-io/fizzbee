package io.fizzbee.mbt.types;

/**
 * Sentinel values for partial state matching in model-based testing.
 * <p>
 * Use these constants in your {@code getState()} implementation to indicate
 * fields that should be ignored or validated differently during state comparison.
 * <p>
 * Example:
 * <pre>{@code
 * public Map<String, Object> getState() {
 *     Map<String, Object> state = new HashMap<>();
 *     state.put("id", Sentinel.IGNORE);  // ID not visible in UI
 *     state.put("title", todo.getTitle());
 *     state.put("completed", todo.isCompleted());
 *     return state;
 * }
 * }</pre>
 */
public enum Sentinel {
    /**
     * IGNORE sentinel: Field should be completely ignored during state comparison.
     * <p>
     * Use this for:
     * <ul>
     *   <li>Non-deterministic values (UUIDs, timestamps, random IDs)</li>
     *   <li>Implementation details not visible in the UI</li>
     *   <li>Fields that exist in the model but can't be observed in the SUT</li>
     * </ul>
     */
    IGNORE;
}
