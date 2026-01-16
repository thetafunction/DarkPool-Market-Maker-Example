# WebSocket Protocol Details

This document describes the WebSocket communication protocol between Market Maker and the DarkPool system.

## Connection Establishment

### Endpoint

```
ws://<host>:<port>/ws
```

### Authentication

Authentication uses JWT Token, carried in the HTTP Header:

```
Authorization: Bearer <jwt_token>
```

The token is obtained from the DarkPool admin console. Ensure that:
- The `mm_id` in the token matches the signer address
- The token has not expired

### Connection Flow

```
Client                                Server
  |                                     |
  |  WebSocket Connect (JWT Header)     |
  |------------------------------------>|
  |                                     |
  |       CONNECTION_ACK (success)      |
  |<------------------------------------|
  |                                     |
  |     DEPTH_SNAPSHOT (periodic)       |
  |------------------------------------>|
  |                                     |
  |       QUOTE_REQUEST                 |
  |<------------------------------------|
  |                                     |
  |       QUOTE_RESPONSE                |
  |------------------------------------>|
  |                                     |
```

## Message Format

All messages are serialized using Protobuf binary format.

### Message Types

```protobuf
enum MessageType {
  MESSAGE_TYPE_UNSPECIFIED = 0;
  MESSAGE_TYPE_REGISTER = 1;
  MESSAGE_TYPE_REGISTER_ACK = 2;
  MESSAGE_TYPE_DEPTH_SNAPSHOT = 3;
  MESSAGE_TYPE_QUOTE_REQUEST = 4;
  MESSAGE_TYPE_QUOTE_RESPONSE = 5;
  MESSAGE_TYPE_QUOTE_REJECT = 6;
  MESSAGE_TYPE_HEARTBEAT = 7;
  MESSAGE_TYPE_ERROR = 8;
  MESSAGE_TYPE_CONNECTION_ACK = 9;
}
```

### Message Wrapper

```protobuf
message Message {
  MessageType type = 1;
  int64 timestamp = 2;  // Unix millisecond timestamp

  oneof payload {
    DepthSnapshot depth_snapshot = 3;
    QuoteRequest quote_request = 4;
    QuoteResponse quote_response = 5;
    QuoteReject quote_reject = 6;
    Heartbeat heartbeat = 7;
    Error error = 8;
    ConnectionAck connection_ack = 9;
  }
}
```

## Message Type Details

### CONNECTION_ACK

Sent by the server after successful token verification.

```protobuf
message ConnectionAck {
  bool success = 1;
  string session_id = 2;
  int64 server_time = 3;    // Unix milliseconds
  string mm_id = 4;
  ConnectionConfig config = 5;
  string error_message = 6;
}

message ConnectionConfig {
  uint32 depth_push_interval_ms = 1;
  uint32 quote_timeout_ms = 2;
  uint32 heartbeat_interval_ms = 3;
}
```

After receiving `success=true`, the client enters Ready state and can start pushing depth data.
`config` provides server-suggested intervals; the client may keep using its local configuration.

### DEPTH_SNAPSHOT

Depth snapshot actively pushed by the Market Maker.

```protobuf
message DepthSnapshot {
  uint64 chain_id = 1;
  string pair_id = 2;
  string mm_id = 3;
  string token_a = 4;
  string token_b = 5;
  repeated PriceLevel bids = 6;
  repeated PriceLevel asks = 7;
}

message PriceLevel {
  string price = 1;   // wei/wei format (tokenBWei / tokenAWei)
  string amount = 2;  // tokenA native decimals
}
```

**Notes**:
- `price` uses wei/wei format (tokenBWei / tokenAWei, no decimals adjustment)
  - Example: tokenA=WETH(18d), tokenB=USDC(6d), 1 WETH=3400 USDC, price = 3400×10^6 / 10^18 ≈ 0.0000000034
- `amount` uses tokenA (baseToken) native decimals
  - Example: 3.28 WETH is represented as "3280000000000000000"
- `asks` sorted by price in ascending order
- `bids` sorted by price in descending order

### QUOTE_REQUEST

Quote request sent by the server.

```protobuf
message QuoteRequest {
  string quote_id = 1;
  uint64 chain_id = 2;
  string mm_id = 3;
  string token_in = 4;
  string token_out = 5;
  string amount_in = 6;  // native decimals
  string recipient = 7;
  string nonce = 8;
  int64 deadline = 9;
  uint32 slippage_bps = 10;
}
```

**Notes**:
- `amount_in` uses the token's native decimals (e.g., USDC is 6 decimals, WETH is 18 decimals)
- `token_in` as `0x0000...0000` represents the native token
- If `token_in`/`token_out` is zero address, the client replaces it with the chain's wrapped token when building the response/signature
- `deadline` is a Unix second timestamp

### QUOTE_RESPONSE

Successful quote response.

```protobuf
message QuoteResponse {
  string quote_id = 1;
  uint64 chain_id = 2;
  string mm_id = 3;
  QuoteStatus status = 4;
  QuoteInfo quote = 5;
  SignedOrder order = 6;
  int64 valid_until = 7;  // millisecond timestamp
}

message QuoteInfo {
  string token_in = 1;
  string token_out = 2;
  string amount_in = 3;          // native decimals
  string amount_out = 4;         // native decimals
  string amount_out_minimum = 5; // native decimals
  string price = 6;
  string price_impact = 7;
}

message SignedOrder {
  string signer = 1;
  string rfq_manager = 2;       // verifying contract address
  string nonce = 3;
  string amount_in = 4;   // native decimals
  string amount_out = 5;  // native decimals (matches signature)
  int64 deadline = 6;
  bytes extra_data = 7;   // optional opaque bytes (demo uses empty bytes)
  bytes signature = 8;    // EIP-712 signature
}
```

### QUOTE_REJECT

Quote rejection.

```protobuf
message QuoteReject {
  string quote_id = 1;
  uint64 chain_id = 2;
  string mm_id = 3;
  RejectReason reason = 4;
  string message = 5;
}

enum RejectReason {
  REJECT_REASON_UNSPECIFIED = 0;
  REJECT_REASON_INSUFFICIENT_LIQUIDITY = 1;
  REJECT_REASON_PRICE_MOVED = 2;
  REJECT_REASON_PAIR_NOT_SUPPORTED = 3;
  REJECT_REASON_AMOUNT_TOO_SMALL = 4;
  REJECT_REASON_AMOUNT_TOO_LARGE = 5;
  REJECT_REASON_RATE_LIMITED = 6;
  REJECT_REASON_INTERNAL_ERROR = 7;
}
```

### HEARTBEAT

Heartbeat message.

```protobuf
message Heartbeat {
  bool ping = 1;
  bool pong = 2;
}
```

Client behavior:
- Send `ping=true` every 30 seconds
- Reply with `pong=true` when receiving `ping=true` from server

### ERROR

Error message.

```protobuf
message Error {
  ErrorCode code = 1;
  string message = 2;
  string related_quote_id = 3;
}

enum ErrorCode {
  ERROR_CODE_UNSPECIFIED = 0;
  ERROR_CODE_INVALID_MESSAGE = 1;
  ERROR_CODE_INVALID_SIGNATURE = 2;
  ERROR_CODE_TIMEOUT = 3;
  ERROR_CODE_INTERNAL = 4;
  ERROR_CODE_NOT_REGISTERED = 5;
  ERROR_CODE_DUPLICATE_REGISTER = 6;
  ERROR_CODE_UNAUTHORIZED = 7;
  ERROR_CODE_PAIR_NOT_WHITELISTED = 8;
}
```

## Heartbeat Mechanism

- Heartbeat interval: 30 seconds
- Read timeout: 90 seconds
- Reconnection triggered if no message received within timeout

## Reconnection Mechanism

- Initial interval: 5 seconds
- Maximum interval: base interval × 32 (e.g., 160 seconds when base is 5 seconds)
- Uses exponential backoff strategy (multiplier 2.0)
- Unlimited reconnection attempts by default

## Precision Handling

The DarkPool system uses **native decimals** throughout, without 18 decimals standardization:

1. **Receiving `amount_in`**: Use native decimals directly
2. **Quote calculation**: Use native decimals
3. **Signing**: Use native decimals
4. **Returning `amount_out`**: Use native decimals

### Depth Data Format

| Field | Format | Example |
|-------|--------|---------|
| Price | wei/wei ratio (tokenBWei/tokenAWei) | "0.0000000034" |
| Amount | tokenA native decimals | "3280000000000000000" |

**Example**: tokenA=WETH(18d), tokenB=USDC(6d), 1 WETH = 3400 USDC
- Price = 3400 × 10^6 / 10^18 = 3.4×10^-9 ≈ "0.0000000034"
- Amount = 3.28 WETH = "3280000000000000000"
