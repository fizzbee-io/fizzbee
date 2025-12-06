
### Java
To generate the proto files for java, run the following command:
```bash
protoc \
  --proto_path=mbt/lib/shared/proto/ \
  --java_out=mbt/lib/java/src/main \
  --grpc-java_out=mbt/lib/java/src/main \
  mbt/lib/shared/proto/mbt_plugin.proto
```

### Go
To generate the proto files for go, run the following command:
```bash
protoc \
  --proto_path=mbt/lib/shared/proto \
  --go_out=mbt/lib/go/internalpb --go_opt=paths=source_relative \
  --go-grpc_out=mbt/lib/go/internalpb --go-grpc_opt=paths=source_relative \
  mbt/lib/shared/proto/mbt_plugin.proto
```

### TypeScript
To generate the proto files for TypeScript, run the following command from the typescript directory:
```bash
cd mbt/lib/typescript
npm run generate-proto
```

Or manually with grpc_tools_node_protoc:
```bash
grpc_tools_node_protoc \
  --proto_path=mbt/lib/shared/proto \
  --js_out=import_style=commonjs,binary:mbt/lib/typescript/proto-gen \
  --grpc_out=grpc_js:mbt/lib/typescript/proto-gen \
  --plugin=protoc-gen-grpc=$(which grpc_tools_node_protoc_plugin) \
  mbt/lib/shared/proto/mbt_plugin.proto

grpc_tools_node_protoc \
  --proto_path=mbt/lib/shared/proto \
  --plugin=protoc-gen-ts=$(which protoc-gen-ts) \
  --ts_out=grpc_js:mbt/lib/typescript/proto-gen \
  mbt/lib/shared/proto/mbt_plugin.proto
```
