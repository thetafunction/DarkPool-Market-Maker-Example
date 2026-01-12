package quote

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// MockStrategy is a mock quote strategy
// For demonstration and testing only, third-party MMs should replace with real quoting logic
type MockStrategy struct {
	// SpreadBps is the bid-ask spread (basis points)
	// Example: 50 means 0.5% spread
	SpreadBps uint32

	// Prices is the mock price configuration
	// key: "chainId:tokenIn:tokenOut" (lowercase addresses)
	// value: price (outputToken/inputToken)
	Prices map[string]*big.Float
}

// NewMockStrategy creates a mock quote strategy
func NewMockStrategy(spreadBps uint32) *MockStrategy {
	return &MockStrategy{
		SpreadBps: spreadBps,
		Prices:    make(map[string]*big.Float),
	}
}

// SetPrice sets a mock price
func (s *MockStrategy) SetPrice(chainID uint64, tokenIn, tokenOut common.Address, price *big.Float) {
	key := s.buildPriceKey(chainID, tokenIn, tokenOut)
	s.Prices[key] = price
}

// buildPriceKey builds the price lookup key
func (s *MockStrategy) buildPriceKey(chainID uint64, tokenIn, tokenOut common.Address) string {
	return fmt.Sprintf("%d:%s:%s",
		chainID,
		strings.ToLower(tokenIn.Hex()),
		strings.ToLower(tokenOut.Hex()))
}

// CalculateQuote calculates a mock quote
func (s *MockStrategy) CalculateQuote(ctx context.Context, params *QuoteParams) (*QuoteResult, error) {
	// Look up price
	price := s.getPrice(params.ChainID, params.TokenIn, params.TokenOut)
	if price == nil {
		return nil, fmt.Errorf("price not found for %s -> %s on chain %d",
			params.TokenIn.Hex(), params.TokenOut.Hex(), params.ChainID)
	}

	// Calculate output amount
	// amountOut = amountIn * price * (1 - spread/10000)
	amountInFloat := new(big.Float).SetInt(params.AmountIn)
	amountOutFloat := new(big.Float).Mul(amountInFloat, price)

	// Apply spread
	spreadFactor := new(big.Float).SetFloat64(float64(10000-s.SpreadBps) / 10000)
	amountOutFloat.Mul(amountOutFloat, spreadFactor)

	// Convert to integer
	amountOut := new(big.Int)
	amountOutFloat.Int(amountOut)

	if amountOut.Sign() <= 0 {
		return nil, fmt.Errorf("calculated amount out is zero or negative")
	}

	// Build result
	result := NewQuoteResult(amountOut, params.SlippageBps)
	result.ExecutionPrice = price
	result.PriceImpact = float64(s.SpreadBps) / 100 // Simplified: spread equals price impact

	return result, nil
}

// getPrice gets price (supports bidirectional lookup)
func (s *MockStrategy) getPrice(chainID uint64, tokenIn, tokenOut common.Address) *big.Float {
	// Forward lookup
	key := s.buildPriceKey(chainID, tokenIn, tokenOut)
	if price, ok := s.Prices[key]; ok {
		return price
	}

	// Reverse lookup
	reverseKey := s.buildPriceKey(chainID, tokenOut, tokenIn)
	if reversePrice, ok := s.Prices[reverseKey]; ok {
		// Return reciprocal
		one := big.NewFloat(1)
		return new(big.Float).Quo(one, reversePrice)
	}

	return nil
}

// DefaultMockStrategy creates a mock strategy with default prices
func DefaultMockStrategy() *MockStrategy {
	strategy := NewMockStrategy(50) // 0.5% spread

	// BSC: WBNB/USDT = 600 USDT
	strategy.SetPrice(56,
		common.HexToAddress("0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c"), // WBNB
		common.HexToAddress("0x55d398326f99059fF775485246999027B3197955"), // USDT
		big.NewFloat(600))

	// Base: WETH/USDC = 3500 USDC
	strategy.SetPrice(8453,
		common.HexToAddress("0x4200000000000000000000000000000000000006"), // WETH
		common.HexToAddress("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"), // USDC
		big.NewFloat(3500))

	return strategy
}
