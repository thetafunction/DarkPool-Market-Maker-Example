package depth

import (
	"math/big"
)

// DepthProvider is the depth data provider interface
// Third-party MMs need to implement this interface to provide real depth data
type DepthProvider interface {
	// GetDepth retrieves depth data for a specified pair
	// chainID: Chain ID
	// pairID: trading pair identifier
	// Returns OrderBook or error
	GetDepth(chainID uint64, pairID string) (*OrderBook, error)
}

// OrderBook represents the order book data structure
//
// SwapEngine expected format:
// - Price: wei/wei ratio (tokenBWei / tokenAWei, no decimals adjustment)
// - Amount: tokenA (baseToken) native decimals quantity
//
// Example: tokenA = WETH (18 decimals), tokenB = USDC (6 decimals), 1 WETH = 3400 USDC
//   - Price = 3400 * 10^6 / 10^18 = 3.4e-9
//   - Amount = 3.28e18 (i.e., 3.28 WETH in wei)
type OrderBook struct {
	MidPrice   *big.Float   // Mid price (wei/wei format: tokenBWei / tokenAWei)
	Spread     float64      // Bid-ask spread (percentage)
	Bids       []PriceLevel // Bids (descending by price) - Amount is tokenA quantity
	Asks       []PriceLevel // Asks (ascending by price) - Amount is tokenA quantity
	BaseToken  string       // tokenA address (Amount is denominated in this)
	QuoteToken string       // tokenB address
}

// PriceLevel represents a price level in the order book
type PriceLevel struct {
	Price  *big.Float // Price (wei/wei format: tokenBWei / tokenAWei)
	Amount *big.Int   // Amount (tokenA native decimals, e.g., WETH is 18 decimals)
}

// NewOrderBook creates a new order book
func NewOrderBook(baseToken, quoteToken string) *OrderBook {
	return &OrderBook{
		MidPrice:   big.NewFloat(0),
		Spread:     0,
		Bids:       make([]PriceLevel, 0),
		Asks:       make([]PriceLevel, 0),
		BaseToken:  baseToken,
		QuoteToken: quoteToken,
	}
}

// NewPriceLevel creates a new price level
func NewPriceLevel(price *big.Float, amount *big.Int) PriceLevel {
	return PriceLevel{
		Price:  price,
		Amount: amount,
	}
}
