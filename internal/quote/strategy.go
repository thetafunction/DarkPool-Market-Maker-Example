package quote

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// QuoteStrategy is the quote strategy interface
// Third-party MMs need to implement this interface to provide custom quoting logic
type QuoteStrategy interface {
	// CalculateQuote calculates a quote
	// Input: chain ID, token pair, input amount (native decimals)
	// Output: output amount, minimum output, price impact, etc.
	CalculateQuote(ctx context.Context, params *QuoteParams) (*QuoteResult, error)
}

// QuoteParams represents quote request parameters
type QuoteParams struct {
	ChainID  uint64         // Chain ID
	TokenIn  common.Address // Input token address
	TokenOut common.Address // Output token address
	AmountIn *big.Int       // Input amount (native decimals)
}

// QuoteResult represents the quote result
type QuoteResult struct {
	AmountOut        *big.Int   // Output amount (native decimals)
	AmountOutMinimum *big.Int   // Minimum output amount (native decimals)
	ExecutionPrice   *big.Float // Execution price (outputToken/inputToken)
	PriceImpact      float64    // Price impact (percentage, e.g., 0.05 means 0.05%)
}

// NewQuoteResult creates a quote result
func NewQuoteResult(amountOut *big.Int) *QuoteResult {
	return &QuoteResult{
		AmountOut:        amountOut,
		AmountOutMinimum: amountOut, // No slippage deduction
		ExecutionPrice:   big.NewFloat(0),
		PriceImpact:      0,
	}
}
