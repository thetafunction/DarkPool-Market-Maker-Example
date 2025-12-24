#!/bin/bash
# gen-proto.sh - Generate Go code from protobuf definitions
# Usage: ./scripts/gen-proto.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "Generating protobuf code..."

# Check if protoc is installed
if ! command -v protoc &> /dev/null; then
    echo "Error: protoc is not installed"
    echo "Install with: brew install protobuf (macOS) or apt install protobuf-compiler (Linux)"
    exit 1
fi

# Check if protoc-gen-go is installed
if ! command -v protoc-gen-go &> /dev/null; then
    echo "Installing protoc-gen-go..."
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi

# Create output directory
mkdir -p "$PROJECT_ROOT/mm/v1"

# Generate Go code
protoc \
    --proto_path="$PROJECT_ROOT/proto" \
    --go_out="$PROJECT_ROOT" \
    --go_opt=paths=source_relative \
    --go_opt=Mmm/v1/mm.proto=github.com/ThetaSpace/DarkPool-Market-Maker-Example/mm/v1 \
    "$PROJECT_ROOT/proto/mm/v1/mm.proto"

# Move generated file to correct location
if [ -f "$PROJECT_ROOT/proto/mm/v1/mm.pb.go" ]; then
    mv "$PROJECT_ROOT/proto/mm/v1/mm.pb.go" "$PROJECT_ROOT/mm/v1/mm.pb.go"
fi

echo "Protobuf code generated successfully!"
echo "Output: $PROJECT_ROOT/mm/v1/mm.pb.go"
