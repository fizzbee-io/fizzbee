// scripts/gen_protos.rs
// Whenever the protobuf definitions in `shared/proto/mbt_plugin.proto` are updated, run it as
// cargo run --bin gen_protos
fn main() -> Result<(), Box<dyn std::error::Error>> {
    let crate_dir = std::env::current_dir()?;
    let proto_dir = crate_dir.join("../shared/proto");
    let proto_file = proto_dir.join("mbt_plugin.proto");

    println!("Generating Rust protobufs...");
    println!("Proto file: {}", proto_file.display());

    tonic_build::configure()
        .build_client(true)
        .build_server(true)
        .out_dir("src/pb")
        .compile_protos(&[proto_file], &[proto_dir])?;

    println!("✅ Protobufs generated successfully in src/pb/");
    Ok(())
}
