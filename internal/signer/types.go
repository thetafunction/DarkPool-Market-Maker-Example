package signer

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// MMQuote matches rfq_contracts_v2 RFQLib.MMQuote, excluding mmSignature.
type MMQuote struct {
	QuoteId                   [32]byte
	Maker                     common.Address
	Vault                     common.Address
	Executor                  common.Address
	InputToken                common.Address
	OutputToken               common.Address
	AmountIn                  *big.Int
	AmountOut                 *big.Int
	Deadline                  *big.Int
	Nonce                     *big.Int
	ConfidenceExtractedValueT *big.Int
	ConfidenceExtractedValueN *big.Int
	ConfidenceExtractedValueM *big.Int
	ConfidenceExtractedValueE *big.Int
	ExtraData                 []byte
}

// MMQuoteTypeHash must stay byte-for-byte aligned with RFQLib.MM_QUOTE_TYPEHASH.
var MMQuoteTypeHash = crypto.Keccak256Hash([]byte(
	"MMQuote(bytes32 quoteId,address maker,address vault,address executor,address inputToken,address outputToken," +
		"uint256 amountIn,uint256 amountOut,uint256 deadline,uint256 nonce," +
		"uint256 confidenceExtractedValueT,uint256 confidenceExtractedValueN," +
		"uint256 confidenceExtractedValueM,uint256 confidenceExtractedValueE,bytes extraData)"))

var WrappedNativeTokens = map[uint64]common.Address{
	56:   common.HexToAddress("0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c"),
	8453: common.HexToAddress("0x4200000000000000000000000000000000000006"),
	1:    common.HexToAddress("0xc02aaa39b223fe8d0a0e5c4f27ead9083c756cc2"),
}

func GetWrappedToken(chainID uint64) (common.Address, bool) {
	addr, ok := WrappedNativeTokens[chainID]
	return addr, ok
}
