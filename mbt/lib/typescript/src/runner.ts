import * as grpc from '@grpc/grpc-js';
import { spawn, ChildProcess } from 'child_process';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import { FizzBeeMbtPluginService } from './plugin-service';
import type { Model, ActionFunc } from './interfaces';
import { FizzBeeMbtPluginServiceService } from '../proto-gen/mbt_plugin_grpc_pb';

/**
 * Options for configuring the test runner.
 */
export interface RunTestsOptions {
  'max-seq-runs'?: number;
  'max-parallel-runs'?: number;
  'max-actions'?: number;
  'seq-seed'?: number;
  'parallel-seed'?: number;
  [key: string]: any;
}

/**
 * Gets a numeric value from environment variable.
 */
function getEnvNumber(key: string): number | undefined {
  const value = process.env[key];
  if (value) {
    const num = parseInt(value, 10);
    return isNaN(num) ? undefined : num;
  }
  return undefined;
}

/**
 * Gets the path to the fizzbee-mbt-runner binary from environment.
 */
function getMbtBinPath(): string {
  return process.env.FIZZBEE_MBT_BIN || 'fizzbee-mbt-runner';
}

/**
 * Main function to run MBT tests.
 */
export async function runTests(
  model: Model,
  actionsRegistry: Map<string, Map<string, ActionFunc>>,
  options: RunTestsOptions = {}
): Promise<void> {
  // Create temporary directory for socket
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'fizzbee-mbt-'));
  const socketPath = path.join(tmpDir, 'plugin.sock');

  let server: grpc.Server | null = null;
  let childProcess: ChildProcess | null = null;

  try {
    // Create service implementation
    const serviceImpl = new FizzBeeMbtPluginService(model, actionsRegistry);

    // Create and start gRPC server
    server = new grpc.Server();
    server.addService(FizzBeeMbtPluginServiceService, serviceImpl);

    // Start server on Unix domain socket
    await new Promise<void>((resolve, reject) => {
      server!.bindAsync(
        `unix://${socketPath}`,
        grpc.ServerCredentials.createInsecure(),
        (error) => {
          if (error) {
            reject(error);
          } else {
            console.log(`gRPC server started on ${socketPath}`);
            resolve();
          }
        }
      );
    });

    // Build command arguments
    const runnerCmd = getMbtBinPath();
    const args = [`--plugin-addr=${socketPath}`];

    // Add options, with environment variable overrides
    for (const [optionName, optionValue] of Object.entries(options)) {
      let value = optionValue;

      // Override with environment variables if present
      if (optionName === 'max-actions') {
        value = getEnvNumber('FIZZBEE_MBT_MAX_ACTIONS') ?? value;
      } else if (optionName === 'max-seq-runs') {
        value = getEnvNumber('FIZZBEE_MBT_MAX_SEQ_RUNS') ?? value;
      } else if (optionName === 'max-parallel-runs') {
        value = getEnvNumber('FIZZBEE_MBT_MAX_PARALLEL_RUNS') ?? value;
      }

      args.push(`--${optionName}=${value}`);
    }

    // Add seed flags from environment if provided
    const seqSeed = getEnvNumber('FIZZBEE_MBT_SEQ_SEED');
    if (seqSeed !== undefined) {
      args.push(`--seq-seed=${seqSeed}`);
    }
    const parallelSeed = getEnvNumber('FIZZBEE_MBT_PARALLEL_SEED');
    if (parallelSeed !== undefined) {
      args.push(`--parallel-seed=${parallelSeed}`);
    }

    // Start child process
    console.log(`Starting runner: ${runnerCmd} ${args.join(' ')}`);
    childProcess = spawn(runnerCmd, args, {
      stdio: 'inherit'
    });

    // Handle process signals
    const signalHandler = () => {
      console.log('Interrupt received, stopping runner and test plugin...');
      if (childProcess) {
        childProcess.kill('SIGTERM');
      }
    };

    process.on('SIGINT', signalHandler);
    process.on('SIGTERM', signalHandler);

    // Wait for child process to complete
    await new Promise<void>((resolve, reject) => {
      childProcess!.on('exit', (code, signal) => {
        if (code === 0) {
          console.log('Runner exited successfully');
          resolve();
        } else {
          reject(new Error(`Runner exited with code ${code}, signal ${signal}`));
        }
      });

      childProcess!.on('error', (error) => {
        reject(new Error(`Failed to start runner: ${error.message}`));
      });
    });

  } finally {
    // Graceful shutdown
    if (server) {
      const serverRef = server; // Capture for use in closures
      await new Promise<void>((resolve) => {
        const shutdownTimeout = setTimeout(() => {
          console.log('Forcing server stop');
          serverRef.forceShutdown();
          resolve();
        }, 5000);

        serverRef.tryShutdown(() => {
          clearTimeout(shutdownTimeout);
          console.log('Test executor shut down gracefully');
          resolve();
        });
      });
    }

    // Clean up temporary directory
    try {
      if (fs.existsSync(socketPath)) {
        fs.unlinkSync(socketPath);
      }
      fs.rmdirSync(tmpDir);
    } catch (error) {
      // Ignore cleanup errors
    }
  }
}
