package signer

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// MMQuote represents the Quote structure used for MM signing in the contract
// Corresponds to contract MMQUOTE_SIGNATURE_HASH
// Important: From and To come from SwapQuote.info, they are user addresses (msg.sender), not MM signer address
type MMQuote struct {
	Pool        common.Address // DarkPool Pool address (EIP-712 Domain VerifyingContract)
	From        common.Address // Source address (SwapQuote.info.from, usually user wallet address)
	To          common.Address // Target address (SwapQuote.info.to, usually same as From)
	InputToken  common.Address // Input token address
	OutputToken common.Address // Output token address
	AmountIn    *big.Int       // Input amount (native decimals)
	AmountOut   *big.Int       // Output amount (minimum guaranteed output, native decimals)
	Deadline    *big.Int       // Expiration timestamp (Unix seconds)
	Nonce       *big.Int       // Anti-replay nonce
	ExtraData   []byte         // Extra data (used to calculate extraDataHash)
}

// MMQuoteTypeHash is the keccak256 hash of MMQuote type
// Corresponds to contract MMQUOTE_SIGNATURE_HASH
var MMQuoteTypeHash = crypto.Keccak256Hash([]byte(
	"MMQuote(address pool,address from,address to,address inputToken,address outputToken," +
		"uint256 amountIn,uint256 amountOut,uint256 deadline,uint256 nonce,bytes32 extraDataHash)"))

// WrappedNativeTokens maps chain IDs to their Wrapped Native Token addresses
// chainId -> wrapped token address
var WrappedNativeTokens = map[uint64]common.Address{
	56:   common.HexToAddress("0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c"), // BSC: WBNB
	8453: common.HexToAddress("0x4200000000000000000000000000000000000006"), // Base: WETH
	1:    common.HexToAddress("0xc02aaa39b223fe8d0a0e5c4f27ead9083c756cc2"), // Ethereum: WETH
}

// GetWrappedToken gets the Wrapped Native Token address for a specified chain
func GetWrappedToken(chainID uint64) (common.Address, bool) {
	addr, ok := WrappedNativeTokens[chainID]
	return addr, ok
}
