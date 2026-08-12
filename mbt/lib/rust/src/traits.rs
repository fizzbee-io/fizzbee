use crate::error::MbtError;
use crate::value::Value;
use crate::types::{RoleId, Arg};
use async_trait::async_trait;
use std::collections::HashMap;

// --- Fuzzing Support ---

/// Options passed to Model::provide_overrides during dynamic
/// (simulation-guided / fuzz) test runs.
#[derive(Debug, Clone, Copy)]
pub struct FuzzOptions {
    /// Random seed for reproducible fuzzing. Use it to derive any random
    /// values so a failing run can be replayed with the same seed.
    pub seed: i64,
}

// --- State Traits (Typically synchronous) ---

pub trait StateGetter {
    fn get_state(&self, key: &str) -> Result<Value, MbtError>;
}

pub trait SnapshotStateGetter {
    fn snapshot(&self) -> Result<Vec<(String, Value)>, MbtError>;
}

// --- Role and Concurrency Bounds ---

/// The base marker trait for all components representing a role in the system.
pub trait Role {}

/// A trait alias for Roles that are safe to share and execute concurrently.
/// This enforces the necessary `Send + Sync + 'static` bounds.
pub trait AsyncRole: Role + Send + Sync + 'static {}
impl<T: Role + Send + Sync + 'static> AsyncRole for T {}

// --- Lifecycle Traits (Must be async) ---

#[async_trait]
pub trait Model: Send + Sync + 'static {
    /// Initializes the model state before a test run.
    async fn init(&mut self) -> Result<(), MbtError>;
    /// Cleans up the model state after a test run.
    async fn cleanup(&mut self) -> Result<(), MbtError>;

    /// Provides spec variable overrides before model initialization during
    /// dynamic (fuzz) test runs. Called before init() only when the runner
    /// requests a fuzzing run (InitRequest.is_fuzzing); the returned map is
    /// sent back to the model checker to override spec constants for that
    /// run. The default implementation returns no overrides, so existing
    /// models are unaffected. (Rust equivalent of the TypeScript
    /// OverridesProvider interface — a default method instead of a separate
    /// trait because trait coherence prevents an overridable blanket impl.)
    ///
    /// Example:
    /// ```ignore
    /// async fn provide_overrides(&mut self, options: &FuzzOptions)
    ///     -> Result<HashMap<String, Value>, MbtError> {
    ///     let mut overrides = HashMap::new();
    ///     overrides.insert("MAX_RETRIES".to_string(), Value::Int(5));
    ///     // derive randomized values from options.seed for reproducibility
    ///     Ok(overrides)
    /// }
    /// ```
    async fn provide_overrides(
        &mut self,
        _options: &FuzzOptions,
    ) -> Result<HashMap<String, Value>, MbtError> {
        Ok(HashMap::new())
    }
}

// --- Execution Traits (Must be async) ---

#[async_trait]
pub trait DispatchModel: Send + Sync + 'static {
    /// Executes a named function on a specific role instance.
    async fn execute(
        &self,
        role_id: &RoleId,
        function_name: &str,
        _args: &[Arg],
    ) -> Result<Value, MbtError>;

    /// Discovers and returns all available role instances managed by the model.
    fn get_roles(&self) -> Result<Vec<RoleId>, MbtError>;
}