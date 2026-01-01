#!/bin/bash

# Protocol Buffer code generation script

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PROTO_DIR="${PROJECT_ROOT}/proto"

OUT_DIR="${PROJECT_ROOT}/pkg/protocol/pb"

mkdir -p "${OUT_DIR}"

protoc \
  --proto_path="${PROTO_DIR}" \
  --go_out="${OUT_DIR}" \
  --go_opt=paths=source_relative \
  "${PROTO_DIR}"/*.proto

echo "Protobuf code generated successfully in ${OUT_DIR}"
