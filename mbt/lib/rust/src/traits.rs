use crate::error::MbtError;
use crate::value::Value;
use crate::types::{RoleId, Arg};
use async_trait::async_trait;

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