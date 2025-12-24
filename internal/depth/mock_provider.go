package depth

import (
	"fmt"
	"math/big"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// MockProvider is a mock depth data provider
// For demonstration and testing only, generates random but reasonable depth data
// Third-party MMs should replace with real data sources (on-chain reads, CEX APIs, etc.)
type MockProvider struct {
	// prices stores the base price for each trading pair
	// key: "chainId:baseToken:quoteToken" (lowercase addresses)
	prices map[string]*big.Float
	mu     sync.RWMutex
	rng    *rand.Rand
}

// NewMockProvider creates a mock depth data provider
func NewMockProvider() *MockProvider {
	return &MockProvider{
		prices: make(map[string]*big.Float),
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// SetBasePrice sets the base price
func (p *MockProvider) SetBasePrice(chainID uint64, baseToken, quoteToken string, price float64) {
	key := buildPriceKey(chainID, baseToken, quoteToken)
	p.mu.Lock()
	p.prices[key] = big.NewFloat(price)
	p.mu.Unlock()
}

// GetDepth gets depth data
func (p *MockProvider) GetDepth(chainID uint64, poolAddress string) (*OrderBook, error) {
	// Mock implementation: directly use poolAddress as part of the key
	// Real implementation should read from on-chain or get from CEX API
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Find matching price configuration
	var basePrice *big.Float
	var baseToken, quoteToken string

	for key, price := range p.prices {
		parts := strings.Split(key, ":")
		if len(parts) == 3 {
			keyChainID := parts[0]
			if keyChainID == fmt.Sprintf("%d", chainID) {
				basePrice = price
				baseToken = parts[1]
				quoteToken = parts[2]
				break
			}
		}
	}

	if basePrice == nil {
		return nil, fmt.Errorf("no price configured for chain %d", chainID)
	}

	// Generate mock order book
	ob := NewOrderBook(baseToken, quoteToken)
	ob.MidPrice = basePrice

	// Generate bids and asks (10 price levels each)
	ob.Asks = p.generateAsks(basePrice, 10)
	ob.Bids = p.generateBids(basePrice, 10)

	// Calculate spread
	if len(ob.Asks) > 0 && len(ob.Bids) > 0 {
		bestAsk, _ := ob.Asks[0].Price.Float64()
		bestBid, _ := ob.Bids[0].Price.Float64()
		if bestBid > 0 {
			ob.Spread = (bestAsk - bestBid) / bestBid * 100
		}
	}

	return ob, nil
}

// generateAsks generates asks (price ascending)
func (p *MockProvider) generateAsks(midPrice *big.Float, levels int) []PriceLevel {
	asks := make([]PriceLevel, levels)
	midPriceFloat, _ := midPrice.Float64()

	for i := 0; i < levels; i++ {
		// Price increases: midPrice * (1 + 0.001 * (i+1) + random noise)
		priceIncrease := 1 + 0.001*float64(i+1) + p.rng.Float64()*0.0005
		price := big.NewFloat(midPriceFloat * priceIncrease)

		// Random amount (1-100 tokens, in 18 decimals format)
		// amount = (1 + random * 99) * 1e18
		amountFloat := (1 + p.rng.Float64()*99)
		amount := big.NewInt(int64(amountFloat * 1e18))

		asks[i] = NewPriceLevel(price, amount)
	}

	return asks
}

// generateBids generates bids (price descending)
func (p *MockProvider) generateBids(midPrice *big.Float, levels int) []PriceLevel {
	bids := make([]PriceLevel, levels)
	midPriceFloat, _ := midPrice.Float64()

	for i := 0; i < levels; i++ {
		// Price decreases: midPrice * (1 - 0.001 * (i+1) - random noise)
		priceDecrease := 1 - 0.001*float64(i+1) - p.rng.Float64()*0.0005
		price := big.NewFloat(midPriceFloat * priceDecrease)

		// Random amount (1-100 tokens, in 18 decimals format)
		amountFloat := (1 + p.rng.Float64()*99)
		amount := big.NewInt(int64(amountFloat * 1e18))

		bids[i] = NewPriceLevel(price, amount)
	}

	return bids
}

// buildPriceKey builds the price lookup key
func buildPriceKey(chainID uint64, baseToken, quoteToken string) string {
	return fmt.Sprintf("%d:%s:%s", chainID,
		strings.ToLower(baseToken),
		strings.ToLower(quoteToken))
}

// DefaultMockProvider creates a mock provider with default prices
func DefaultMockProvider() *MockProvider {
	provider := NewMockProvider()

	// BSC: WBNB/USDT = 600 USDT
	provider.SetBasePrice(56,
		"0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c", // WBNB
		"0x55d398326f99059ff775485246999027b3197955", // USDT
		600)

	// Base: WETH/USDC = 3500 USDC
	provider.SetBasePrice(8453,
		"0x4200000000000000000000000000000000000006", // WETH
		"0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", // USDC
		3500)

	return provider
}
