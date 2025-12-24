package signer

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// ExtraDataParams contains MMQuote.extraData parameters
// Format: abi.encode(pool, zeroForOne, sqrtPriceLimitX96, callbackData)
type ExtraDataParams struct {
	Pool              common.Address // V3 Pool address
	ZeroForOne        bool           // Swap direction
	SqrtPriceLimitX96 *big.Int       // Price limit
	CallbackData      []byte         // Callback data (ABI encoded payToken address)
}

// EncodeExtraData encodes MMQuote.extraData
// Format: abi.encode(address pool, bool zeroForOne, uint160 sqrtPriceLimitX96, bytes callbackData)
func EncodeExtraData(params *ExtraDataParams) ([]byte, error) {
	addrTy, _ := abi.NewType("address", "", nil)
	boolTy, _ := abi.NewType("bool", "", nil)
	uint160Ty, _ := abi.NewType("uint160", "", nil)
	bytesTy, _ := abi.NewType("bytes", "", nil)

	args := abi.Arguments{
		{Type: addrTy},
		{Type: boolTy},
		{Type: uint160Ty},
		{Type: bytesTy},
	}

	sqrtPriceLimit := params.SqrtPriceLimitX96
	if sqrtPriceLimit == nil {
		sqrtPriceLimit = MinMaxSqrtPriceX96(params.ZeroForOne)
	}

	return args.Pack(params.Pool, params.ZeroForOne, sqrtPriceLimit, params.CallbackData)
}

// DetermineZeroForOne determines swap direction based on token addresses
// If sellerToken == token0, then zeroForOne = true (swap token0 for token1)
// If sellerToken == token1, then zeroForOne = false (swap token1 for token0)
func DetermineZeroForOne(sellerToken, token0, token1 common.Address) bool {
	return sellerToken == token0
}

// MinMaxSqrtPriceX96 returns the price limit for V3 swap
// When zeroForOne=true, returns minimum price limit
// When zeroForOne=false, returns maximum price limit
func MinMaxSqrtPriceX96(zeroForOne bool) *big.Int {
	if zeroForOne {
		// MIN_SQRT_RATIO + 1
		return new(big.Int).Add(big.NewInt(4295128739), big.NewInt(1))
	}
	// MAX_SQRT_RATIO - 1
	max, _ := new(big.Int).SetString("1461446703485210103287273052203988822378723970342", 10)
	return new(big.Int).Sub(max, big.NewInt(1))
}

// BuildCallbackData constructs callback data
// Callback data format: abi.encode(payToken)
// payToken is the token address to transfer from Vault
func BuildCallbackData(payToken common.Address) ([]byte, error) {
	addrTy, _ := abi.NewType("address", "", nil)
	args := abi.Arguments{{Type: addrTy}}
	return args.Pack(payToken)
}
