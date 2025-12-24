package depth

import (
	"math/big"
	"testing"
)

func TestNewOrderBook(t *testing.T) {
	baseToken := "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c"
	quoteToken := "0x55d398326f99059fF775485246999027B3197955"

	ob := NewOrderBook(baseToken, quoteToken)
	if ob == nil {
		t.Fatal("NewOrderBook returned nil")
	}

	if ob.BaseToken != baseToken {
		t.Errorf("BaseToken = %s, want %s", ob.BaseToken, baseToken)
	}
	if ob.QuoteToken != quoteToken {
		t.Errorf("QuoteToken = %s, want %s", ob.QuoteToken, quoteToken)
	}
	if ob.MidPrice == nil {
		t.Error("MidPrice should not be nil")
	}
	if len(ob.Bids) != 0 {
		t.Error("Bids should be empty")
	}
	if len(ob.Asks) != 0 {
		t.Error("Asks should be empty")
	}
}

func TestNewPriceLevel(t *testing.T) {
	price := big.NewFloat(600.0)
	amount := big.NewInt(1000000000000000000)

	level := NewPriceLevel(price, amount)
	if level.Price.Cmp(price) != 0 {
		t.Errorf("Price = %v, want %v", level.Price, price)
	}
	if level.Amount.Cmp(amount) != 0 {
		t.Errorf("Amount = %v, want %v", level.Amount, amount)
	}
}

func TestMockProvider_SetBasePrice(t *testing.T) {
	provider := NewMockProvider()

	provider.SetBasePrice(56, "0xbase", "0xquote", 600.0)

	// Verify price was set
	key := buildPriceKey(56, "0xbase", "0xquote")
	provider.mu.RLock()
	price, ok := provider.prices[key]
	provider.mu.RUnlock()

	if !ok {
		t.Error("Price was not set")
	}

	priceFloat, _ := price.Float64()
	if priceFloat != 600.0 {
		t.Errorf("Price = %f, want 600.0", priceFloat)
	}
}

func TestMockProvider_GetDepth(t *testing.T) {
	provider := DefaultMockProvider()

	// Test BSC
	ob, err := provider.GetDepth(56, "0x172fcD41E0913e95784454622d1c3724f546f849")
	if err != nil {
		t.Fatalf("GetDepth failed: %v", err)
	}

	if ob == nil {
		t.Fatal("GetDepth returned nil")
	}

	// Verify mid price
	if ob.MidPrice == nil || ob.MidPrice.Sign() == 0 {
		t.Error("MidPrice should be positive")
	}

	// Verify asks and bids are not empty
	if len(ob.Asks) == 0 {
		t.Error("Asks should not be empty")
	}
	if len(ob.Bids) == 0 {
		t.Error("Bids should not be empty")
	}

	// Verify asks price ascending (each price should be >= midPrice)
	midPriceFloat, _ := ob.MidPrice.Float64()
	for i, ask := range ob.Asks {
		askPrice, _ := ask.Price.Float64()
		if askPrice < midPriceFloat*0.99 { // Allow small margin
			t.Errorf("Ask[%d] price %f should be >= midPrice %f", i, askPrice, midPriceFloat)
		}
	}

	// Verify bids price descending (each price should be <= midPrice)
	for i, bid := range ob.Bids {
		bidPrice, _ := bid.Price.Float64()
		if bidPrice > midPriceFloat*1.01 { // Allow small margin
			t.Errorf("Bid[%d] price %f should be <= midPrice %f", i, bidPrice, midPriceFloat)
		}
	}
}

func TestMockProvider_GetDepth_ChainNotConfigured(t *testing.T) {
	provider := NewMockProvider() // Empty provider

	_, err := provider.GetDepth(999, "0xpool")
	if err == nil {
		t.Error("GetDepth should fail for unconfigured chain")
	}
}

func TestDefaultMockProvider(t *testing.T) {
	provider := DefaultMockProvider()

	// Verify BSC configuration
	ob, err := provider.GetDepth(56, "")
	if err != nil {
		t.Errorf("BSC should be configured: %v", err)
	}
	if ob == nil {
		t.Error("BSC OrderBook should not be nil")
	}

	// Verify Base configuration
	ob, err = provider.GetDepth(8453, "")
	if err != nil {
		t.Errorf("Base should be configured: %v", err)
	}
	if ob == nil {
		t.Error("Base OrderBook should not be nil")
	}
}

func TestBuildPriceKey(t *testing.T) {
	key := buildPriceKey(56, "0xBase", "0xQuote")
	expected := "56:0xbase:0xquote"

	if key != expected {
		t.Errorf("buildPriceKey = %s, want %s", key, expected)
	}
}

func TestGetChainName(t *testing.T) {
	tests := []struct {
		chainID  uint64
		expected string
	}{
		{1, "ethereum"},
		{56, "bsc"},
		{8453, "base"},
		{42161, "arbitrum"},
		{10, "optimism"},
		{999, "chain_999"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			name := getChainName(tt.chainID)
			if name != tt.expected {
				t.Errorf("getChainName(%d) = %s, want %s", tt.chainID, name, tt.expected)
			}
		})
	}
}

func TestMockProvider_GenerateAsks(t *testing.T) {
	provider := NewMockProvider()
	midPrice := big.NewFloat(600.0)

	asks := provider.generateAsks(midPrice, 5)

	if len(asks) != 5 {
		t.Errorf("asks length = %d, want 5", len(asks))
	}

	// Verify price ascending
	for i := 0; i < len(asks)-1; i++ {
		current, _ := asks[i].Price.Float64()
		next, _ := asks[i+1].Price.Float64()
		if current >= next {
			t.Errorf("asks[%d] price %f should be < asks[%d] price %f", i, current, i+1, next)
		}
	}

	// Verify amount is positive
	for i, ask := range asks {
		if ask.Amount.Sign() <= 0 {
			t.Errorf("asks[%d] amount should be positive", i)
		}
	}
}

func TestMockProvider_GenerateBids(t *testing.T) {
	provider := NewMockProvider()
	midPrice := big.NewFloat(600.0)

	bids := provider.generateBids(midPrice, 5)

	if len(bids) != 5 {
		t.Errorf("bids length = %d, want 5", len(bids))
	}

	// Verify price descending
	for i := 0; i < len(bids)-1; i++ {
		current, _ := bids[i].Price.Float64()
		next, _ := bids[i+1].Price.Float64()
		if current <= next {
			t.Errorf("bids[%d] price %f should be > bids[%d] price %f", i, current, i+1, next)
		}
	}

	// Verify amount is positive
	for i, bid := range bids {
		if bid.Amount.Sign() <= 0 {
			t.Errorf("bids[%d] amount should be positive", i)
		}
	}
}

func TestOrderBook_Spread(t *testing.T) {
	provider := DefaultMockProvider()

	ob, _ := provider.GetDepth(56, "")

	// Verify spread calculation
	if ob.Spread < 0 {
		t.Error("Spread should be non-negative")
	}

	// Spread should be a reasonable value (less than 10%)
	if ob.Spread > 10 {
		t.Errorf("Spread = %f%%, seems too high", ob.Spread)
	}
}
