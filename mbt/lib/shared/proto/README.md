
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
