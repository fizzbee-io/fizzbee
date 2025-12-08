/**
 * Represents an argument passed to action methods.
 */
export interface Arg {
  name: string;
  value: any;
}

/**
 * Unique identifier for a role instance.
 */
export interface RoleId {
  roleName: string;
  index: number;
}

/**
 * Custom error for indicating that a feature is not implemented.
 */
export class NotImplementedError extends Error {
  constructor(message: string = 'Not implemented') {
    super(message);
    this.name = 'NotImplementedError';
  }
}

/**
 * Custom error for MBT-related failures.
 */
export class MbtError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'MbtError';
  }
}
