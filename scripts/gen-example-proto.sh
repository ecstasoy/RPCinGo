#!/bin/bash

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PROTO_DIR="${PROJECT_ROOT}/examples/proto"

OUT_DIR="${PROJECT_ROOT}/examples/proto"

for service_dir in "${PROTO_DIR}"/*/; do
    if [ -d "$service_dir" ]; then
        service_name=$(basename "$service_dir")
        echo "Generating code for ${service_name}..."
        
        protoc \
            --proto_path="${PROTO_DIR}" \
            --go_out="${OUT_DIR}/${service_name}" \
            --go_opt=paths=source_relative \
            "${service_dir}"/*.proto
        
        echo "✅ Generated code for ${service_name}"
    fi
done

echo "All protobuf code generated successfully!"




