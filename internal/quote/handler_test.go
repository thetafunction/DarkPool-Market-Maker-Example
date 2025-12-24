package quote

import (
	"context"
	"log/slog"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/config"
	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/signer"
	mmv1 "github.com/ThetaSpace/DarkPool-Market-Maker-Example/mm/v1"
)

// mockSigner is a mock signer for testing
type mockSigner struct {
	address common.Address
}

func (m *mockSigner) SignMMQuote(chainID uint64, quote *signer.MMQuote) ([]byte, error) {
	// Return fixed 65-byte signature
	sig := make([]byte, 65)
	sig[64] = 27
	return sig, nil
}

func (m *mockSigner) GetAddress() common.Address {
	return m.address
}

func testConfig() *config.Config {
	return &config.Config{
		App: config.AppConfig{
			Name:     "test",
			LogLevel: "info",
		},
		Quote: config.QuoteConfig{
			ValidDuration: 30 * time.Second,
		},
		EIP712Domains: []config.EIP712Domain{
			{
				ChainID:           56,
				Name:              "RFQ Pool",
				Version:           "1",
				VerifyingContract: "0x28D3a265f6d40867986004029ee91F4C9532fCC5",
			},
		},
		Pairs: []config.PairConfig{
			{
				ChainID:            56,
				PairID:             "WBNB-USDT",
				PoolAddress:        "0x172fcD41E0913e95784454622d1c3724f546f849",
				BaseToken:          "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c",
				QuoteToken:         "0x55d398326f99059fF775485246999027B3197955",
				BaseTokenDecimals:  18,
				QuoteTokenDecimals: 18,
				FeeRate:            2500,
			},
		},
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNewHandler(t *testing.T) {
	strategy := DefaultMockStrategy()
	s := &mockSigner{address: common.HexToAddress("0x1234567890123456789012345678901234567890")}
	cfg := testConfig()
	logger := testLogger()

	handler := NewHandler(strategy, s, cfg, logger)
	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}
}

func TestHandler_HandleQuoteRequest_Success(t *testing.T) {
	strategy := DefaultMockStrategy()
	s := &mockSigner{address: common.HexToAddress("0x1234567890123456789012345678901234567890")}
	cfg := testConfig()
	logger := testLogger()

	handler := NewHandler(strategy, s, cfg, logger)

	req := &mmv1.QuoteRequest{
		QuoteId:     "test-quote-123",
		ChainId:     56,
		MmId:        "test-mm",
		TokenIn:     "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c", // WBNB
		TokenOut:    "0x55d398326f99059fF775485246999027B3197955", // USDT
		AmountIn:    "1000000000000000000",                        // 1e18 (1 WBNB)
		Recipient:   "0xabcdef1234567890abcdef1234567890abcdef12",
		Deadline:    time.Now().Add(5 * time.Minute).Unix(),
		Nonce:       "1",
		SlippageBps: 50,
	}

	ctx := context.Background()
	msg, err := handler.HandleQuoteRequest(ctx, req)
	if err != nil {
		t.Fatalf("HandleQuoteRequest failed: %v", err)
	}

	if msg == nil {
		t.Fatal("HandleQuoteRequest returned nil message")
	}

	if msg.Type != mmv1.MessageType_MESSAGE_TYPE_QUOTE_RESPONSE {
		t.Errorf("Message type = %v, want QUOTE_RESPONSE", msg.Type)
	}

	resp := msg.GetQuoteResponse()
	if resp == nil {
		t.Fatal("QuoteResponse is nil")
	}

	if resp.QuoteId != req.QuoteId {
		t.Errorf("QuoteId = %s, want %s", resp.QuoteId, req.QuoteId)
	}

	if resp.Status != mmv1.QuoteStatus_QUOTE_STATUS_SUCCESS {
		t.Errorf("Status = %v, want SUCCESS", resp.Status)
	}

	if resp.Quote == nil {
		t.Fatal("Quote is nil")
	}

	if resp.Order == nil {
		t.Fatal("Order is nil")
	}

	// Verify signature exists
	if len(resp.Order.Signature) != 65 {
		t.Errorf("Signature length = %d, want 65", len(resp.Order.Signature))
	}
}

func TestHandler_HandleQuoteRequest_InvalidQuoteId(t *testing.T) {
	strategy := DefaultMockStrategy()
	s := &mockSigner{address: common.HexToAddress("0x1234567890123456789012345678901234567890")}
	cfg := testConfig()
	logger := testLogger()

	handler := NewHandler(strategy, s, cfg, logger)

	req := &mmv1.QuoteRequest{
		QuoteId: "", // Empty quoteId
		ChainId: 56,
	}

	ctx := context.Background()
	msg, err := handler.HandleQuoteRequest(ctx, req)
	if err != nil {
		t.Fatalf("HandleQuoteRequest should not return error: %v", err)
	}

	if msg.Type != mmv1.MessageType_MESSAGE_TYPE_QUOTE_REJECT {
		t.Errorf("Message type = %v, want QUOTE_REJECT", msg.Type)
	}
}

func TestHandler_HandleQuoteRequest_ChainNotConfigured(t *testing.T) {
	strategy := DefaultMockStrategy()
	s := &mockSigner{address: common.HexToAddress("0x1234567890123456789012345678901234567890")}
	cfg := testConfig()
	logger := testLogger()

	handler := NewHandler(strategy, s, cfg, logger)

	req := &mmv1.QuoteRequest{
		QuoteId:   "test-quote-123",
		ChainId:   999, // Unconfigured chain
		TokenIn:   "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c",
		TokenOut:  "0x55d398326f99059fF775485246999027B3197955",
		AmountIn:  "1000000000000000000",
		Recipient: "0xabcdef1234567890abcdef1234567890abcdef12",
		Deadline:  time.Now().Add(5 * time.Minute).Unix(),
		Nonce:     "1",
	}

	ctx := context.Background()
	msg, _ := handler.HandleQuoteRequest(ctx, req)

	if msg.Type != mmv1.MessageType_MESSAGE_TYPE_QUOTE_REJECT {
		t.Errorf("Message type = %v, want QUOTE_REJECT", msg.Type)
	}

	reject := msg.GetQuoteReject()
	if reject.Reason != mmv1.RejectReason_REJECT_REASON_PAIR_NOT_SUPPORTED {
		t.Errorf("Reject reason = %v, want PAIR_NOT_SUPPORTED", reject.Reason)
	}
}

func TestHandler_HandleQuoteRequest_PairNotConfigured(t *testing.T) {
	strategy := DefaultMockStrategy()
	s := &mockSigner{address: common.HexToAddress("0x1234567890123456789012345678901234567890")}
	cfg := testConfig()
	logger := testLogger()

	handler := NewHandler(strategy, s, cfg, logger)

	req := &mmv1.QuoteRequest{
		QuoteId:   "test-quote-123",
		ChainId:   56,
		TokenIn:   "0x1111111111111111111111111111111111111111", // Unconfigured token
		TokenOut:  "0x2222222222222222222222222222222222222222",
		AmountIn:  "1000000000000000000",
		Recipient: "0xabcdef1234567890abcdef1234567890abcdef12",
		Deadline:  time.Now().Add(5 * time.Minute).Unix(),
		Nonce:     "1",
	}

	ctx := context.Background()
	msg, _ := handler.HandleQuoteRequest(ctx, req)

	if msg.Type != mmv1.MessageType_MESSAGE_TYPE_QUOTE_REJECT {
		t.Errorf("Message type = %v, want QUOTE_REJECT", msg.Type)
	}
}

func TestHandler_HandleQuoteRequest_DeadlineExpired(t *testing.T) {
	strategy := DefaultMockStrategy()
	s := &mockSigner{address: common.HexToAddress("0x1234567890123456789012345678901234567890")}
	cfg := testConfig()
	logger := testLogger()

	handler := NewHandler(strategy, s, cfg, logger)

	req := &mmv1.QuoteRequest{
		QuoteId:   "test-quote-123",
		ChainId:   56,
		TokenIn:   "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c",
		TokenOut:  "0x55d398326f99059fF775485246999027B3197955",
		AmountIn:  "1000000000000000000",
		Recipient: "0xabcdef1234567890abcdef1234567890abcdef12",
		Deadline:  time.Now().Add(-1 * time.Minute).Unix(), // Expired
		Nonce:     "1",
	}

	ctx := context.Background()
	msg, _ := handler.HandleQuoteRequest(ctx, req)

	if msg.Type != mmv1.MessageType_MESSAGE_TYPE_QUOTE_REJECT {
		t.Errorf("Message type = %v, want QUOTE_REJECT", msg.Type)
	}
}

func TestHandler_HandleQuoteRequest_ZeroAddressToken(t *testing.T) {
	strategy := DefaultMockStrategy()
	s := &mockSigner{address: common.HexToAddress("0x1234567890123456789012345678901234567890")}
	cfg := testConfig()
	logger := testLogger()

	handler := NewHandler(strategy, s, cfg, logger)

	// Use zero address to represent native token
	req := &mmv1.QuoteRequest{
		QuoteId:     "test-quote-123",
		ChainId:     56,
		TokenIn:     "0x0000000000000000000000000000000000000000", // Native token
		TokenOut:    "0x55d398326f99059fF775485246999027B3197955", // USDT
		AmountIn:    "1000000000000000000",
		Recipient:   "0xabcdef1234567890abcdef1234567890abcdef12",
		Deadline:    time.Now().Add(5 * time.Minute).Unix(),
		Nonce:       "1",
		SlippageBps: 50,
	}

	ctx := context.Background()
	msg, err := handler.HandleQuoteRequest(ctx, req)
	if err != nil {
		t.Fatalf("HandleQuoteRequest failed: %v", err)
	}

	// Should succeed (zero address replaced with WBNB)
	if msg.Type != mmv1.MessageType_MESSAGE_TYPE_QUOTE_RESPONSE {
		t.Errorf("Message type = %v, want QUOTE_RESPONSE", msg.Type)
	}
}

func TestWrappedNativeTokens(t *testing.T) {
	// Verify wrapped token configuration
	tests := []struct {
		chainID uint64
		exists  bool
	}{
		{56, true},   // BSC
		{8453, true}, // Base
		{1, true},    // Ethereum
		{999, false}, // Unconfigured
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			_, ok := WrappedNativeTokens[tt.chainID]
			if ok != tt.exists {
				t.Errorf("WrappedNativeTokens[%d] exists = %v, want %v", tt.chainID, ok, tt.exists)
			}
		})
	}
}

func TestMockStrategy(t *testing.T) {
	strategy := DefaultMockStrategy()

	params := &QuoteParams{
		ChainID:     56,
		TokenIn:     common.HexToAddress("0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c"), // WBNB
		TokenOut:    common.HexToAddress("0x55d398326f99059fF775485246999027B3197955"), // USDT
		AmountIn:    big.NewInt(1000000000000000000), // 1 WBNB
		SlippageBps: 50,
	}

	ctx := context.Background()
	result, err := strategy.CalculateQuote(ctx, params)
	if err != nil {
		t.Fatalf("CalculateQuote failed: %v", err)
	}

	if result == nil {
		t.Fatal("CalculateQuote returned nil result")
	}

	// Verify output is positive
	if result.AmountOut.Sign() <= 0 {
		t.Error("AmountOut should be positive")
	}

	// Verify AmountOutMinimum <= AmountOut
	if result.AmountOutMinimum.Cmp(result.AmountOut) > 0 {
		t.Error("AmountOutMinimum should be <= AmountOut")
	}
}

func TestMockStrategy_PriceNotFound(t *testing.T) {
	strategy := NewMockStrategy(50)

	params := &QuoteParams{
		ChainID:  999, // Unconfigured chain
		TokenIn:  common.HexToAddress("0x1111111111111111111111111111111111111111"),
		TokenOut: common.HexToAddress("0x2222222222222222222222222222222222222222"),
		AmountIn: big.NewInt(1000000000000000000),
	}

	ctx := context.Background()
	_, err := strategy.CalculateQuote(ctx, params)
	if err == nil {
		t.Error("CalculateQuote should fail when price not found")
	}
}
