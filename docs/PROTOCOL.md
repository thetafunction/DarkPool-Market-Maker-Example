# WebSocket Protocol

This example follows the current MMHub WebSocket protocol used by the RFQ stack. All messages are protobuf-encoded and wrapped in a top-level `Message`.

## Authentication

Connect to the MMHub WebSocket endpoint with:

```text
Authorization: Bearer <market_maker_api_key>
```

Use the `market_maker` API key issued for the Market Maker identity. Do not use a `business` API key here.

## Connection Flow

```text
MM Client                               MMHub
  |                                       |
  | WebSocket connect + Bearer token      |
  |-------------------------------------->|
  |                                       |
  | CONNECTION_ACK                        |
  |<--------------------------------------|
  |                                       |
  | DEPTH_SNAPSHOT_BATCH (periodic)       |
  |-------------------------------------->|
  |                                       |
  | QUOTE_REQUEST                         |
  |<--------------------------------------|
  |                                       |
  | QUOTE_RESPONSE or QUOTE_REJECT        |
  |-------------------------------------->|
  |                                       |
  | HEARTBEAT ping/pong                   |
  |<------------------------------------->|
```

After `CONNECTION_ACK.success=true`, the client can start publishing depth and answering quote requests.

## Wrapper

```protobuf
message Message {
  MessageType type = 1;
  int64 timestamp = 2;

  oneof payload {
    DepthSnapshot depth_snapshot = 3;
    QuoteRequest quote_request = 4;
    QuoteResponse quote_response = 5;
    QuoteReject quote_reject = 6;
    Heartbeat heartbeat = 7;
    Error error = 8;
    ConnectionAck connection_ack = 9;
    DepthSnapshotBatch depth_snapshot_batch = 10;
  }
}
```

Relevant message types in this example:

- `MESSAGE_TYPE_CONNECTION_ACK`
- `MESSAGE_TYPE_DEPTH_SNAPSHOT_BATCH`
- `MESSAGE_TYPE_QUOTE_REQUEST`
- `MESSAGE_TYPE_QUOTE_RESPONSE`
- `MESSAGE_TYPE_QUOTE_REJECT`
- `MESSAGE_TYPE_HEARTBEAT`
- `MESSAGE_TYPE_ERROR`

## CONNECTION_ACK

```protobuf
message ConnectionAck {
  bool success = 1;
  string session_id = 2;
  int64 server_time = 3;
  string mm_id = 4;
  ConnectionConfig config = 5;
  string error_message = 6;
  repeated string supported_versions = 7;
}
```

- `supported_versions` declares the RFQ protocol versions the server accepts.
- This example currently responds only to `v1`.

## Depth Publishing

The example publishes all configured pairs in one batch:

```protobuf
message DepthSnapshotBatch {
  repeated DepthSnapshot snapshots = 1;
}

message DepthSnapshot {
  uint64 chain_id = 1;
  string pair_id = 2;
  string mm_id = 3;
  string token_a = 4;
  string token_b = 5;
  repeated PriceLevel bids = 6;
  repeated PriceLevel asks = 7;
  string min_order_size = 8;
}
```

`PriceLevel.price` is the `token_b_wei / token_a_wei` ratio. `PriceLevel.amount` uses `token_a` native decimals. `min_order_size` is the minimum fill size for `token_a` (wei string); `"0"`, empty, or omitted means no minimum.

## Quote Request Envelope

`QuoteRequest` is a stable envelope. The body is versioned and encoded in `quote_request_data`.

```protobuf
message QuoteRequest {
  string quote_id = 1;
  uint64 chain_id = 2;
  string mm_id = 3;
  string protocol_version = 4;
  bytes quote_request_data = 5;
}
```

Constraints used by this example:

- `quote_id` must be a 32-byte hex string
- `protocol_version` must be `v1`
- `quote_request_data` must decode as `QuoteRequestV1`

## QuoteRequestV1

```protobuf
message QuoteRequestV1 {
  string token_in = 1;
  string token_out = 2;
  string amount_in = 3;
  string executor = 4;
  int64 deadline = 5;
  string nonce = 6;
  string from = 7;
  string recipient = 8;
  ConfidenceExtractedValue confidence_extracted_value = 9;
}
```

Notes:

- `amount_in`, `nonce`, and the confidence-extracted-value fields are decimal strings.
- `deadline` is a Unix second timestamp.
- `token_in` or `token_out` may be the zero address to represent the native token.
- This example uses wrapped-native addresses only for local pair lookup. The outgoing quote and signature keep the original request token addresses unchanged.

## Quote Response Envelope

```protobuf
message QuoteResponse {
  string quote_id = 1;
  uint64 chain_id = 2;
  string mm_id = 3;
  QuoteStatus status = 4;
  string protocol_version = 5;
  bytes mm_quote_data = 6;
}
```

When `status=QUOTE_STATUS_SUCCESS`, `mm_quote_data` contains `MMQuoteV1`.

## MMQuoteV1

```protobuf
message MMQuoteV1 {
  string maker = 1;
  string vault = 2;
  string executor = 3;
  string token_in = 4;
  string token_out = 5;
  string amount_in = 6;
  string amount_out = 7;
  string deadline = 8;
  string nonce = 9;
  ConfidenceExtractedValue confidence_extracted_value = 10;
  bytes extra_data = 11;
  bytes mm_signature = 12;
  string quote_id = 13;
}
```

- `maker` is the signer address used by the MM.
- `vault` is the RFQ vault configured for that chain.
- `amount_out` is the quoted minimum output amount.
- `mm_signature` is the EIP-712 signature of the on-chain `MMQuote`.

## Quote Reject

```protobuf
enum RejectReason {
  REJECT_REASON_UNSPECIFIED = 0;
  REJECT_REASON_INSUFFICIENT_LIQUIDITY = 1;
  REJECT_REASON_PRICE_MOVED = 2;
  REJECT_REASON_PAIR_NOT_SUPPORTED = 3;
  REJECT_REASON_AMOUNT_TOO_SMALL = 4;
  REJECT_REASON_AMOUNT_TOO_LARGE = 5;
  REJECT_REASON_RATE_LIMITED = 6;
  REJECT_REASON_INTERNAL_ERROR = 7;
  REJECT_REASON_VERSION_NOT_SUPPORTED = 8;
  REJECT_REASON_INVALID_PARAMS = 9;
}
```

This example rejects requests when:

- the protocol version is unsupported
- the pair is not configured locally
- the request cannot be parsed or validated
- the quote strategy returns no liquidity
- signing fails

## Heartbeat

```protobuf
message Heartbeat {
  bool ping = 1;
  bool pong = 2;
}
```

The client should reply with `pong=true` when the server sends `ping=true`.

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
