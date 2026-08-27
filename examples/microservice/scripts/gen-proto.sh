#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PROTO_DIR="${PROJECT_ROOT}/api/user"
OUT_DIR="${PROTO_DIR}"

protoc \
  --proto_path="${PROTO_DIR}" \
  --go_out="${OUT_DIR}" \
  --go_opt=paths=source_relative \
  "${PROTO_DIR}"/user.proto

echo "Proto code generated successfully in ${OUT_DIR}"

