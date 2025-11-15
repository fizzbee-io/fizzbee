pub mod config;
pub mod value;
pub mod error;
pub mod types;
pub mod traits;

// Generated protobuf & gRPC types
pub(crate) mod pb {
    include!("pb/fizzbee.mbt.rs");
}
pub use crate::config::TestOptions;

// Internal modules (private)
mod runner;
mod plugin_service;

pub use runner::run_mbt_test;