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
  AfterActionHook,
  OverridesProvider,
  FuzzOptions,
  Model,
  ActionFunc
} from './interfaces';

// Overrides
export {
  OverridesBuilder
} from './overrides';

// Types
export {
  Arg,
  RoleId,
  NotImplementedError,
  MbtError,
  Tuple
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
  Ignored,
  ignored,
  Sentinel
} from './sentinels';
