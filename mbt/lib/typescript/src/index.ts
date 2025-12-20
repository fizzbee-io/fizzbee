/**
 * FizzBee Model Based Testing (MBT) TypeScript/JavaScript Library
 *
 * This library provides the core interfaces and utilities for implementing
 * model-based testing with FizzBee in TypeScript/JavaScript.
 */

// Core interfaces
export {
  Role,
  StateGetter,
  SnapshotStateGetter,
  RoleMapper,
  Model,
  ActionFunc
} from './interfaces';

// Types
export {
  Arg,
  RoleId,
  NotImplementedError,
  MbtError
} from './types';

// Runner
export {
  runTests,
  RunTestsOptions
} from './runner';

// Value utilities
export {
  fromProtoValue,
  toProtoValue,
  fromProtoArg,
  fromProtoArgs
} from './value';

// Sentinel values
export {
  IGNORE,
  Sentinel
} from './sentinels';
