#!/usr/bin/env node

const { execSync } = require('child_process');
const path = require('path');
const fs = require('fs');

// Paths
const PROTO_DIR = path.resolve(__dirname, '../../shared/proto');
const PROTO_FILE = path.join(PROTO_DIR, 'mbt_plugin.proto');
const OUT_DIR = path.resolve(__dirname, '../proto-gen');

// Ensure output directory exists
if (!fs.existsSync(OUT_DIR)) {
  fs.mkdirSync(OUT_DIR, { recursive: true });
}

// Find protoc-gen-ts plugin
const PLUGIN_PATH = path.resolve(__dirname, '../node_modules/.bin/protoc-gen-ts');

console.log('Generating TypeScript code from proto files...');
console.log('Proto directory:', PROTO_DIR);
console.log('Proto file:', PROTO_FILE);
console.log('Output directory:', OUT_DIR);

try {
  // Generate JavaScript code with grpc_tools_node_protoc
  const grpcToolsPath = path.resolve(__dirname, '../node_modules/.bin/grpc_tools_node_protoc');

  const command = `${grpcToolsPath} \
    --proto_path=${PROTO_DIR} \
    --js_out=import_style=commonjs,binary:${OUT_DIR} \
    --grpc_out=grpc_js:${OUT_DIR} \
    --plugin=protoc-gen-grpc=${path.resolve(__dirname, '../node_modules/.bin/grpc_tools_node_protoc_plugin')} \
    ${PROTO_FILE}`;

  console.log('Running:', command);
  execSync(command, { stdio: 'inherit' });

  // Generate TypeScript definitions
  const tsCommand = `${grpcToolsPath} \
    --proto_path=${PROTO_DIR} \
    --plugin=protoc-gen-ts=${PLUGIN_PATH} \
    --ts_out=grpc_js:${OUT_DIR} \
    ${PROTO_FILE}`;

  console.log('Running:', tsCommand);
  execSync(tsCommand, { stdio: 'inherit' });

  console.log('Proto generation completed successfully!');
} catch (error) {
  console.error('Error generating proto files:', error.message);
  process.exit(1);
}
