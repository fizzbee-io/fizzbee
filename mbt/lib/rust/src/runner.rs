use crate::plugin_service::FizzBeeServiceImpl;
use crate::pb::fizz_bee_mbt_plugin_service_server::FizzBeeMbtPluginServiceServer;
use crate::error::MbtError;
use crate::traits::{Model, DispatchModel};
use crate::config::TestOptions;
use tokio::runtime::Builder;
use tonic::transport::Server;

// Imports for UDS and Child Process
use tokio::net::UnixListener;
use tokio_stream::wrappers::UnixListenerStream;
use std::path::Path;
use std::fs;
use tokio::process::Command;
use std::env;
use tokio::signal::unix::{signal, SignalKind};

// Helper for Binary Path
fn get_mbt_bin_path() -> String {
    env::var("FIZZBEE_MBT_BIN").unwrap_or_else(|_| "fizzbee-mbt-runner".to_string())
}

/// Checks environment variable first, then TestOptions, otherwise returns None.
fn get_arg_value(env_var: &str, option_value: Option<u32>) -> Option<String> {
    // Precedence 1: Environment variable
    if let Ok(val) = env::var(env_var) {
        Some(val)
    }
    // Precedence 2: TestOptions
    else if let Some(val) = option_value {
        Some(val.to_string())
    }
    // Fallback: None
    else {
        None
    }
}

// UDS Socket Path Constant
const UDS_PATH: &str = "/tmp/fizzbee_mbt.sock";

// --- Private Helper to start the Server Future ---
/// Binds to the UDS socket and returns the Server future.
fn serve_uds_socket<D>(
    dispatcher: D,
) -> Result<impl std::future::Future<Output = Result<(), tonic::transport::Error>>, MbtError>
where
    D: Model + DispatchModel + Send + Sync + 'static,
{
    let path = Path::new(UDS_PATH);

    // Clean up the old socket file if it exists.
    if path.exists() {
        fs::remove_file(path).map_err(MbtError::from_err)?;
    }

    let uds = UnixListener::bind(path).map_err(MbtError::from_err)?;
    let incoming = UnixListenerStream::new(uds);

    let service = FizzBeeServiceImpl::new(dispatcher);

    Ok(
        Server::builder()
            .add_service(FizzBeeMbtPluginServiceServer::new(service))
            .serve_with_incoming(incoming)
    )
}

// --- 3. Public Entry Point for Running Tests (MBT Orchestrator) ---
/// Starts the gRPC server, launches the child process, and waits for both to complete.
/// Returns the exit status of the child process.
pub fn run_mbt_test<D>(
    dispatcher: D,
    options: TestOptions,
) -> Result<(), MbtError>
where
    D: Model + DispatchModel + Send + Sync + 'static,
{
    let rt = Builder::new_multi_thread()
        .enable_all()
        .build()
        .map_err(MbtError::from_err)?;

    rt.block_on(async {
        // 1. Get the server future
        let server_future = serve_uds_socket(dispatcher)?;

        // 2. Launch the child process after the server has successfully bound.
        let bin_path = get_mbt_bin_path();
        let mut args: Vec<String> = Vec::new();

        // --- 2a. Mandatory Plugin Address ---
        // NOTE: Uses 'unix://' prefix, common for MBT runners to identify UDS
        args.push(format!("--plugin-addr={}", UDS_PATH));

        // --- 2b. Optional Config Options (Env > Options) ---
        if let Some(val) = get_arg_value("MAX_ACTIONS", options.max_actions) {
            args.push(format!("--max-actions={}", val));
        }

        if let Some(val) = get_arg_value("MAX_SEQ_RUNS", options.max_seq_runs) {
            args.push(format!("--max-seq-runs={}", val));
        }

        if let Some(val) = get_arg_value("MAX_PARALLEL_RUNS", options.max_parallel_runs) {
            args.push(format!("--max-parallel-runs={}", val));
        }

        // --- 2c. Optional Seed Options (Env only) ---
        if let Ok(val) = env::var("SEQ_SEED") {
            args.push(format!("--seq-seed={}", val));
        }

        if let Ok(val) = env::var("PARALLEL_SEED") {
            args.push(format!("--parallel-seed={}", val));
        }

        let mut child = Command::new(&bin_path)
            .args(&args)
            .spawn()
            .map_err(MbtError::from_err)?;

        let child_future = child.wait();

        // 3. Setup signal handlers for graceful exit (SIGINT/Ctrl+C and SIGTERM)
        let mut sigint = signal(SignalKind::interrupt()).map_err(MbtError::from_err)?;
        let mut sigterm = signal(SignalKind::terminate()).map_err(MbtError::from_err)?;

        // 4. Concurrently wait for the server, child process, or an interrupt signal.
        tokio::select! {
            // Wait for the gRPC server to terminate
            res = server_future => {
                let _ = child.kill().await;
                match res {
                    Ok(_) => {
                        Err(MbtError::other("gRPC server terminated unexpectedly before child process."))
                    },
                    Err(e) => {
                        Err(MbtError::other(format!("gRPC server failed: {}", e)))
                    }
                }
            }
            // Wait for the child process to terminate
            res = child_future => {
                match res {
                    Ok(status) => {
                        if status.success() {
                            Ok(())
                        } else {
                            Err(MbtError::other(format!("Child process failed with status: {}", status)))
                        }
                    }
                    Err(e) => {
                        Err(MbtError::from_err(e))
                    }
                }
            }
            // Handle interruption signals
            _ = sigint.recv() => {
                // Kill the child process
                let _ = child.kill().await;
                child.wait().await.map_err(MbtError::from_err)?; // Wait for cleanup
                Err(MbtError::other(format!("Test interrupted by signal {:?}", SignalKind::interrupt())))
            }
            _ = sigterm.recv() => {
                // Kill the child process
                let _ = child.kill().await;
                child.wait().await.map_err(MbtError::from_err)?; // Wait for cleanup
                Err(MbtError::other(format!("Test interrupted by signal {:?}", SignalKind::terminate())))
            }
        }
    })
}
