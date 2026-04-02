# DarkPool Market Maker Example

A reference implementation for third-party Market Makers to integrate with the DarkPool system.

## Overview

This project provides a complete Market Maker implementation example to help third-party MMs quickly understand and integrate with the DarkPool WebSocket communication protocol.

Key Features:
- WebSocket connection, automatic reconnection, and heartbeat management
- EIP-712 signed quote responses (chain-specific domain support)
- Depth data batch publishing
- Mock strategy implementation (replaceable with real quoting logic)

## Requirements

- Go 1.22+
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

| Section | Field | Description |
|---------|-------|-------------|
| `signer` | `privateKey` | MM signing private key (development) |
| `signer` | `privateKeyEnv` | Environment variable name for private key (production recommended) |
| `websocket` | `serverUrl` | SwapEngine WebSocket URL |
| `websocket` | `apiToken` | JWT Token obtained from SwapEngine admin panel (mm_id must match signer address) |
| `websocket` | `reconnectInterval` | Reconnection interval (e.g. `5s`) |
| `websocket` | `heartbeatInterval` | Heartbeat interval (e.g. `30s`) |
| `eip712Domains` | - | EIP-712 signing domains per chain, must match DarkPool RFQ Manager contract configuration |
| `quote` | `validDuration` | Quote validity period (e.g. `30s`) |
| `depth` | `enabled` | Whether to enable depth data push |
| `depth` | `pushInterval` | Depth push interval (e.g. `3s`) |

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
├── cmd/mm/                    # Application entry point
│   └── main.go
├── configs/                   # Configuration files
│   └── config.example.yaml
├── E2ETest/                   # E2E integration test
│   ├── main.go                # Test program (no MM mock)
│   └── config.yml             # Test configuration template
├── internal/
│   ├── config/                # Configuration parsing
│   │   └── config.go
│   ├── depth/                 # Depth data module
│   │   ├── provider.go        # DepthProvider interface
│   │   ├── mock_provider.go   # Mock implementation
│   │   └── pusher.go          # Depth batch pusher
│   ├── quote/                 # Quote module
│   │   ├── strategy.go        # QuoteStrategy interface
│   │   ├── mock_strategy.go   # Mock implementation
│   │   └── handler.go         # Quote request handler
│   ├── runner/                # Service orchestration
│   │   └── runner.go
│   ├── signer/                # EIP-712 signing
│   │   ├── signer.go          # Signing implementation
│   │   ├── types.go           # MMQuote struct and type hash
│   │   ├── domain.go          # Chain-specific domain manager
│   │   ├── signer_test.go     # Signer unit tests
│   │   └── domain_test.go     # Domain unit tests
│   └── ws/                    # WebSocket client
│       ├── client.go          # Client implementation
│       ├── reconnect.go       # Auto-reconnection logic
│       ├── heartbeat.go       # Heartbeat mechanism
│       └── client_test.go     # Client unit tests
├── mm/v1/                     # Protobuf generated code
│   └── mm.pb.go
├── proto/mm/v1/               # Proto source files
│   └── mm.proto
├── scripts/                   # Scripts
│   ├── run.sh                 # Run script
│   └── gen-proto.sh           # Proto code generation
├── docs/                      # Documentation
├── logs/                      # Log output (auto-created)
├── Makefile
└── README.md
```

## Custom Implementation

### Quote Strategy

Implement the `QuoteStrategy` interface:

```go
type QuoteStrategy interface {
    // CalculateQuote calculates a quote
    // Input: chain ID, token pair, input amount (native decimals)
    // Output: output amount, minimum output, price impact, etc.
    CalculateQuote(ctx context.Context, params *QuoteParams) (*QuoteResult, error)
}
```

Refer to `internal/quote/mock_strategy.go` for implementation details.

### Depth Data

Implement the `DepthProvider` interface:

```go
type DepthProvider interface {
    // GetDepth retrieves depth data for a specified pair
    // chainID: Chain ID
    // pairID: trading pair identifier
    // Returns OrderBook or error
    GetDepth(chainID uint64, pairID string) (*OrderBook, error)
}
```

Refer to `internal/depth/mock_provider.go` for implementation details.

## E2E Integration Test

The `E2ETest/` directory contains a standalone end-to-end test program that verifies your Market Maker service is working correctly with the DarkPool platform. It does **not** mock any MM logic — it tests against your live MM service.

### What It Tests

| Step | Description |
|------|-------------|
| **Step 1: Orderbook** | Connects to the QE WebSocket, subscribes to a trading pair, and verifies that depth data from your MM is received |
| **Step 2: firmQuote** | Picks the MM from the depth update, calls the B-side firmQuote API, and verifies a valid quote is returned |
| **Step 3: On-chain Settlement** | Decodes the RFQ quote, approves the input token, and calls `Settlement.settle()` on-chain to verify the quote settles successfully |

### Prerequisites

- Your MM service must be **running and pushing depth** for the configured trading pair
- The trader account must hold sufficient **input token balance** and **native token for gas**
- Go 1.22+

### Usage

1. Copy and fill in the configuration:

```bash
cp E2ETest/config.yml E2ETest/config.local.yml
# Edit config.local.yml with your real values
```

2. Run the test:

```bash
go run ./E2ETest/ E2ETest/config.local.yml
```

If no config path is provided, it defaults to `E2ETest/config.yml`.

3. All three steps should print `PASSED`:

```
[Step 1] PASSED: Orderbook depth received successfully
[Step 2] PASSED: firmQuote returned valid result
[Step 3] PASSED: On-chain settlement succeeded

========== B2B E2E Test PASSED ==========
```

### Configuration Reference

See [`E2ETest/config.yml`](E2ETest/config.yml) for all available options and descriptions.

## Documentation

- [WebSocket Protocol Details](docs/PROTOCOL.md)
- [EIP-712 Signature Guide](docs/SIGNATURE.md)

## Make Commands

```bash
make help     # Show help
make build    # Build binary
make run      # Build and run
make test     # Run tests
make proto    # Regenerate proto code
make clean    # Clean build artifacts and logs
make tidy     # Tidy go modules
make fmt      # Format code
make vet      # Vet code
make lint     # Run fmt + vet
```

## License

MIT License
