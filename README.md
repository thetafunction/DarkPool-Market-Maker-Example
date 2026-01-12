# DarkPool Market Maker Example

A reference implementation for third-party Market Makers to integrate with the DarkPool system.

## Overview

This project provides a complete Market Maker implementation example to help third-party MMs quickly understand and integrate with the DarkPool WebSocket communication protocol.

Key Features:
- WebSocket connection and heartbeat management
- EIP-712 signed quote responses
- Depth data publishing
- Mock strategy implementation (replaceable with real quoting logic)

## Requirements

- Go 1.21+
- protoc (optional, for regenerating proto code)

## Quick Start

### 1. Clone the Project

```bash
git clone https://github.com/ThetaSpace/DarkPool-Market-Maker-Example.git
cd DarkPool-Market-Maker-Example
```

### 2. Configuration

```bash
# Copy the configuration file
cp configs/config.example.yaml configs/config.yaml

# Edit the configuration
vim configs/config.yaml
```

Key configuration items:
- `signer.privateKey`: MM signing private key
- `websocket.serverUrl`: DarkPool system WebSocket URL
- `websocket.apiToken`: JWT Token obtained from DarkPool administrator (mm_id must match signer)
- `eip712Domains`: EIP-712 verifying contract domains for each chain

### 3. Build and Run

```bash
# Build
make build

# Run
make run
```

Or use the script:

```bash
./scripts/run.sh
```

## Project Structure

```
.
├── cmd/mm/                 # Application entry point
├── configs/                # Configuration files
├── internal/
│   ├── config/             # Configuration parsing
│   ├── depth/              # Depth data module
│   │   ├── provider.go     # DepthProvider interface
│   │   ├── mock_provider.go # Mock implementation
│   │   └── pusher.go       # Depth pusher
│   ├── quote/              # Quote module
│   │   ├── strategy.go     # QuoteStrategy interface
│   │   ├── mock_strategy.go # Mock implementation
│   │   └── handler.go      # Quote handler
│   ├── runner/             # Service orchestration
│   ├── signer/             # EIP-712 signing
│   └── ws/                 # WebSocket client
├── mm/v1/                  # Protobuf generated code
├── proto/                  # Proto source files
├── scripts/                # Scripts
├── Makefile
└── README.md
```

## Custom Implementation

### Quote Strategy

Implement the `QuoteStrategy` interface:

```go
type QuoteStrategy interface {
    CalculateQuote(ctx context.Context, params *QuoteParams) (*QuoteResult, error)
}
```

Refer to `internal/quote/mock_strategy.go` for implementation details.

### Depth Data

Implement the `DepthProvider` interface:

```go
type DepthProvider interface {
  GetDepth(chainID uint64, pairID string) (*OrderBook, error)
}
```

Refer to `internal/depth/mock_provider.go` for implementation details.

## Documentation

- [WebSocket Protocol Details](docs/PROTOCOL.md)
- [EIP-712 Signature Guide](docs/SIGNATURE.md)

## Make Commands

```bash
make help     # Show help
make build    # Build
make run      # Build and run
make test     # Run tests
make proto    # Regenerate proto code
make clean    # Clean build artifacts
make tidy     # Tidy go modules
make fmt      # Format code
make vet      # Vet code
make lint     # Run fmt + vet
```

## License

MIT License
