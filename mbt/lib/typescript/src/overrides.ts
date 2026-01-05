/**
 * Builder class for providing variable overrides.
 * Supports type-safe methods for setting various Starlark-compatible types.
 */
export class OverridesBuilder {
  private overrides: Map<string, any> = new Map();

  /**
   * Sets a variable override with automatic type inference.
   * @param key - The variable name
   * @param value - The value (string, number, boolean, array, or object)
   * @returns this for method chaining
   */
  set(key: string, value: string | number | boolean | any[] | Record<string, any> | Map<any, any>): this {
    this.overrides.set(key, value);
    return this;
  }

  /**
   * Sets a string variable override.
   * @param key - The variable name
   * @param value - String value
   * @returns this for method chaining
   */
  setString(key: string, value: string): this {
    return this.set(key, value);
  }

  /**
   * Sets an integer variable override.
   * @param key - The variable name
   * @param value - Numeric value (will be floored to integer)
   * @returns this for method chaining
   */
  setInt(key: string, value: number): this {
    return this.set(key, Math.floor(value));
  }

  /**
   * Sets a boolean variable override.
   * @param key - The variable name
   * @param value - Boolean value
   * @returns this for method chaining
   */
  setBool(key: string, value: boolean): this {
    return this.set(key, value);
  }

  /**
   * Sets a list variable override.
   * @param key - The variable name
   * @param value - Array value
   * @returns this for method chaining
   */
  setList(key: string, value: any[]): this {
    return this.set(key, value);
  }

  /**
   * Sets a dict (map/object) variable override.
   * @param key - The variable name
   * @param value - Object or Map value
   * @returns this for method chaining
   */
  setDict(key: string, value: Record<string, any> | Map<any, any>): this {
    return this.set(key, value);
  }

  /**
   * Gets all variable overrides.
   * @returns A copy of the overrides map
   * @internal
   */
  getOverrides(): Map<string, any> {
    return new Map(this.overrides);
  }

  /**
   * Checks if a variable has been set.
   * @param key - The variable name to check
   * @returns true if the variable has been set
   */
  has(key: string): boolean {
    return this.overrides.has(key);
  }

  /**
   * Gets a variable override by key.
   * @param key - The variable name
   * @returns The value, or undefined if not set
   */
  get(key: string): any {
    return this.overrides.get(key);
  }

  /**
   * Removes a variable override.
   * @param key - The variable name to remove
   * @returns true if the key was removed, false if it didn't exist
   */
  delete(key: string): boolean {
    return this.overrides.delete(key);
  }

  /**
   * Clears all variable overrides.
   */
  clear(): void {
    this.overrides.clear();
  }

  /**
   * Gets the number of variable overrides.
   * @returns The count of overrides
   */
  size(): number {
    return this.overrides.size;
  }
}
