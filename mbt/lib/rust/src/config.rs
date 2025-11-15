#[derive(Default)]
pub struct TestOptions {
    pub max_seq_runs: Option<u32>,
    pub max_parallel_runs: Option<u32>,
    pub max_actions: Option<u32>,
}
