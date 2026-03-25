# EIP-712 Signature

This example signs the on-chain `MMQuote` used by `rfq_contracts_v2`. The signature domain is the RFQ vault, not the old RFQ manager contract.

## Domain

Each chain uses an EIP-712 domain with:

```go
type EIP712Domain struct {
	Name              string  // "RFQ MMVault"
	Version           string  // "1"
	ChainID           uint256
	VerifyingContract address // rfqVault
}
```

Configured values come from:

```yaml
eip712Domains:
  - chainId: 56
    rfqVault: "0x87e46572565Efd04121FB81CeB758c47168B4487"
```

`rfqVault` must match the deployed vault contract used for settlement on that chain.

## Signed Struct

The example signs this logical payload:

```go
type MMQuote struct {
	QuoteId                   [32]byte
	Maker                     address
	Vault                     address
	Executor                  address
	InputToken                address
	OutputToken               address
	AmountIn                  uint256
	AmountOut                 uint256
	Deadline                  uint256
	Nonce                     uint256
	ConfidenceExtractedValueT uint256
	ConfidenceExtractedValueN uint256
	ConfidenceExtractedValueM uint256
	ConfidenceExtractedValueE uint256
	ExtraData                 bytes
}
```

The type string is:

```text
MMQuote(bytes32 quoteId,address maker,address vault,address executor,address inputToken,address outputToken,uint256 amountIn,uint256 amountOut,uint256 deadline,uint256 nonce,uint256 confidenceExtractedValueT,uint256 confidenceExtractedValueN,uint256 confidenceExtractedValueM,uint256 confidenceExtractedValueE,bytes extraData)
```

## Field Mapping

- `quoteId`: parsed from the envelope `quote_id` and kept as raw `bytes32`
- `maker`: MM signer address
- `vault`: configured `rfqVault` for the request chain
- `executor`: copied from `QuoteRequestV1.executor`
- `inputToken` and `outputToken`: copied from the request as-is
- `amountIn`: copied from the request
- `amountOut`: result of the quoting strategy
- `deadline`: copied from the request
- `nonce`: copied from the request
- `confidenceExtractedValue*`: copied from the request, defaulting to `0` when omitted
- `extraData`: optional opaque bytes, empty in this example

## Native Token Handling

If the RFQ request uses the zero address for the native token:

- local pair matching uses the wrapped native token for lookup
- the signed `MMQuote` still keeps the original request token address

That behavior matches the current RFQ flow, where the request payload defines what gets signed.

## Signing Steps

1. Build the `MMQuote`.
2. Hash the struct using the `MMQuote` type hash.
3. Fetch the RFQ vault domain separator for `chainId`.
4. Compute the EIP-712 digest `keccak256("\x19\x01" || domainSeparator || structHash)`.
5. Sign with secp256k1 and normalize `v` to `27` or `28`.

## Example

```go
func (s *signer) SignMMQuote(chainID uint64, quote *MMQuote) ([]byte, error) {
	domainSeparator, ok := s.domainManager.GetVaultDomainSeparator(chainID)
	if !ok {
		return nil, fmt.Errorf("domain not configured for chainId %d", chainID)
	}

	structHash, err := hashMMQuote(quote)
	if err != nil {
		return nil, err
	}

	digest := crypto.Keccak256Hash(
		[]byte{0x19, 0x01},
		domainSeparator,
		structHash,
	)

	sig, err := crypto.Sign(digest.Bytes(), s.privateKey)
	if err != nil {
		return nil, err
	}

	if sig[64] < 27 {
		sig[64] += 27
	}

	return sig, nil
}
```

See `internal/signer/signer.go` for the implementation.

## Common Failure Modes

- `rfqVault` does not match the deployed contract
- `quote_id` is not a 32-byte hex string
- `amount_in`, `amount_out`, `nonce`, or CEV fields are not valid decimal integers
- request `deadline` is already expired
