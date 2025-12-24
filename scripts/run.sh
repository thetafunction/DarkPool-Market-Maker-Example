#!/bin/bash
# run.sh - Run the Market Maker Example
# Usage: ./scripts/run.sh [config_path]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

CONFIG_PATH="${1:-$PROJECT_ROOT/configs/config.yaml}"

echo "Starting Market Maker Example..."
echo "Config: $CONFIG_PATH"

# Check if config file exists
if [ ! -f "$CONFIG_PATH" ]; then
    echo "Error: Config file not found: $CONFIG_PATH"
    echo "Please copy configs/config.example.yaml to configs/config.yaml and update settings"
    exit 1
fi

# Build if binary doesn't exist
if [ ! -f "$PROJECT_ROOT/bin/mm" ]; then
    echo "Building..."
    cd "$PROJECT_ROOT"
    go build -o bin/mm ./cmd/mm
fi

# Run
cd "$PROJECT_ROOT"
./bin/mm -config "$CONFIG_PATH"
