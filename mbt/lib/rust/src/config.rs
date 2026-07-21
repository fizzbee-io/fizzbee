#[derive(Default)]
pub struct TestOptions {
    pub max_seq_runs: Option<u32>,
    pub max_parallel_runs: Option<u32>,
    /// Number of dynamic (simulation-guided / fuzz) sequential traces to run.
    /// Unlike max_seq_runs, these traces are generated at test time by the
    /// model checker rather than replayed from the pre-generated state graph.
    /// The plugin's Model::provide_overrides is invoked with the fuzz seed
    /// before each run.
    pub max_fuzz_seq_runs: Option<u32>,
    pub max_actions: Option<u32>,
}
