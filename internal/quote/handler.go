package quote

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/config"
	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/signer"
	mmv1 "github.com/ThetaSpace/DarkPool-Market-Maker-Example/mm/v1"
)

// WrappedNativeTokens maps chain IDs to their Wrapped Native Token addresses
// chainId -> wrapped token address
var WrappedNativeTokens = map[uint64]common.Address{
	56:   common.HexToAddress("0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c"), // BSC: WBNB
	8453: common.HexToAddress("0x4200000000000000000000000000000000000006"), // Base: WETH
	1:    common.HexToAddress("0xc02aaa39b223fe8d0a0e5c4f27ead9083c756cc2"), // Ethereum: WETH
}

// Handler is the quote handler
// Receives QuoteRequest, calls QuoteStrategy to calculate quote, signs and returns QuoteResponse
type Handler struct {
	strategy QuoteStrategy
	signer   signer.Signer
	cfg      *config.Config
	logger   *slog.Logger
}

// NewHandler creates a new quote handler
func NewHandler(strategy QuoteStrategy, s signer.Signer, cfg *config.Config, logger *slog.Logger) *Handler {
	return &Handler{
		strategy: strategy,
		signer:   s,
		cfg:      cfg,
		logger:   logger.With("component", "QuoteHandler"),
	}
}

// HandleQuoteRequest processes a quote request
// Returns QuoteResponse or QuoteReject message
func (h *Handler) HandleQuoteRequest(ctx context.Context, req *mmv1.QuoteRequest) (*mmv1.Message, error) {
	h.logger.Info("received quote request",
		"quoteId", req.QuoteId,
		"chainId", req.ChainId,
		"tokenIn", req.TokenIn,
		"tokenOut", req.TokenOut,
		"amountIn", req.AmountIn)

	// 1. Validate request parameters
	if err := h.validateRequest(req); err != nil {
		h.logger.Error("request validation failed", "error", err)
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR, err.Error()), nil
	}

	// 2. Get EIP712 Domain (for signing)
	domain := h.cfg.GetEIP712Domain(req.ChainId)
	if domain == nil {
		h.logger.Error("chain not configured", "chainId", req.ChainId)
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_PAIR_NOT_SUPPORTED,
			fmt.Sprintf("chain %d not configured", req.ChainId)), nil
	}

	// 3. Handle zero address (native token): replace with chain's Wrapped Token
	tokenIn := common.HexToAddress(req.TokenIn)
	tokenOut := common.HexToAddress(req.TokenOut)

	if tokenIn == (common.Address{}) {
		wrappedToken, ok := WrappedNativeTokens[req.ChainId]
		if !ok {
			h.logger.Error("wrapped token not found for tokenIn", "chainId", req.ChainId)
			return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR,
				fmt.Sprintf("wrapped token not configured for chain %d", req.ChainId)), nil
		}
		tokenIn = wrappedToken
		h.logger.Info("tokenIn is zero address, using wrapped token", "wrappedToken", tokenIn.Hex())
	}

	if tokenOut == (common.Address{}) {
		wrappedToken, ok := WrappedNativeTokens[req.ChainId]
		if !ok {
			h.logger.Error("wrapped token not found for tokenOut", "chainId", req.ChainId)
			return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR,
				fmt.Sprintf("wrapped token not configured for chain %d", req.ChainId)), nil
		}
		tokenOut = wrappedToken
		h.logger.Info("tokenOut is zero address, using wrapped token", "wrappedToken", tokenOut.Hex())
	}

	// 4. Get trading pair configuration
	if h.cfg.GetPairConfig(req.ChainId, tokenIn.Hex(), tokenOut.Hex()) == nil {
		h.logger.Error("pair not found", "chainId", req.ChainId, "tokenIn", tokenIn.Hex(), "tokenOut", tokenOut.Hex())
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_PAIR_NOT_SUPPORTED,
			fmt.Sprintf("pair not found for tokens %s-%s", tokenIn.Hex(), tokenOut.Hex())), nil
	}

	// 5. Parse input amount (swap-engine sends native decimals)
	amountIn, ok := new(big.Int).SetString(req.AmountIn, 10)
	if !ok {
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR, "invalid amount_in"), nil
	}

	h.logger.Info("amountIn received (native decimals)",
		"tokenIn", tokenIn.Hex(),
		"amountIn", amountIn.String())

	// 6. Call strategy to calculate quote
	quoteParams := &QuoteParams{
		ChainID:     req.ChainId,
		TokenIn:     tokenIn,
		TokenOut:    tokenOut,
		AmountIn:    amountIn,
		SlippageBps: req.SlippageBps,
	}

	quoteResult, err := h.strategy.CalculateQuote(ctx, quoteParams)
	if err != nil {
		h.logger.Error("quote calculation failed", "error", err)
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INSUFFICIENT_LIQUIDITY, err.Error()), nil
	}

	// 7. amountOut uses native decimals (no 18d conversion)
	h.logger.Info("quote calculated (native decimals)",
		"amountOut", quoteResult.AmountOut.String(),
		"amountOutMinimum", quoteResult.AmountOutMinimum.String())

	// 8. ExtraData is optional; demo keeps it empty
	extraData := []byte{}

	// 9. Parse nonce
	nonce, ok := new(big.Int).SetString(req.Nonce, 10)
	if !ok {
		nonce = big.NewInt(0)
	}

	// 10. Build MMQuote (for EIP-712 signing)
	// Note: signing uses native decimals, from/to are both user addresses
	userAddr := common.HexToAddress(req.Recipient)
	mmQuote := &signer.MMQuote{
		Pool:        common.HexToAddress(domain.VerifyingContract),
		From:        userAddr,
		To:          userAddr,
		InputToken:  common.HexToAddress(req.TokenIn),  // Use original TokenIn
		OutputToken: common.HexToAddress(req.TokenOut), // Use original TokenOut
		AmountIn:    amountIn,                          // Native decimals
		AmountOut:   quoteResult.AmountOutMinimum,      // Native decimals
		Deadline:    big.NewInt(req.Deadline),
		Nonce:       nonce,
		ExtraData:   extraData,
	}

	// 11. EIP-712 signing
	signature, err := h.signer.SignMMQuote(req.ChainId, mmQuote)
	if err != nil {
		h.logger.Error("signing failed", "error", err)
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR, "signing failed"), nil
	}
	h.logger.Info("quote signed successfully", "quoteId", req.QuoteId)

	// 12. Build response (using native decimals)
	validUntil := time.Now().Add(h.cfg.Quote.ValidDuration).UnixMilli()

	response := &mmv1.QuoteResponse{
		QuoteId: req.QuoteId,
		ChainId: req.ChainId,
		MmId:    strings.ToLower(h.signer.GetAddress().Hex()),
		Status:  mmv1.QuoteStatus_QUOTE_STATUS_SUCCESS,
		Quote: &mmv1.QuoteInfo{
			TokenIn:          strings.ToLower(req.TokenIn),
			TokenOut:         strings.ToLower(req.TokenOut),
			AmountIn:         req.AmountIn,
			AmountOut:        quoteResult.AmountOut.String(),        // Native decimals
			AmountOutMinimum: quoteResult.AmountOutMinimum.String(), // Native decimals
			Price:            quoteResult.ExecutionPrice.String(),
			PriceImpact:      fmt.Sprintf("%.4f", quoteResult.PriceImpact),
		},
		Order: &mmv1.SignedOrder{
			Signer:    strings.ToLower(h.signer.GetAddress().Hex()),
			Pool:      strings.ToLower(domain.VerifyingContract),
			Nonce:     req.Nonce,
			AmountIn:  amountIn.String(),                     // Native decimals
			AmountOut: quoteResult.AmountOutMinimum.String(), // Native decimals (matches signature)
			Deadline:  req.Deadline,
			ExtraData: extraData,
			Signature: signature,
		},
		ValidUntil: validUntil,
	}

	return &mmv1.Message{
		Type:      mmv1.MessageType_MESSAGE_TYPE_QUOTE_RESPONSE,
		Timestamp: time.Now().UnixMilli(),
		Payload: &mmv1.Message_QuoteResponse{
			QuoteResponse: response,
		},
	}, nil
}

// validateRequest validates quote request parameters
func (h *Handler) validateRequest(req *mmv1.QuoteRequest) error {
	if req.QuoteId == "" {
		return fmt.Errorf("quote_id is required")
	}
	if req.ChainId == 0 {
		return fmt.Errorf("chain_id is required")
	}
	if req.TokenIn == "" {
		return fmt.Errorf("token_in is required")
	}
	if req.TokenOut == "" {
		return fmt.Errorf("token_out is required")
	}
	if req.AmountIn == "" || req.AmountIn == "0" {
		return fmt.Errorf("amount_in is required and must be positive")
	}
	if req.Recipient == "" {
		return fmt.Errorf("recipient is required")
	}
	if req.Deadline == 0 {
		return fmt.Errorf("deadline is required")
	}
	// Check if deadline has already expired
	if req.Deadline < time.Now().Unix() {
		return fmt.Errorf("deadline already expired")
	}
	return nil
}

// buildRejectMessage builds a rejection message
func (h *Handler) buildRejectMessage(req *mmv1.QuoteRequest, reason mmv1.RejectReason, message string) *mmv1.Message {
	return &mmv1.Message{
		Type:      mmv1.MessageType_MESSAGE_TYPE_QUOTE_REJECT,
		Timestamp: time.Now().UnixMilli(),
		Payload: &mmv1.Message_QuoteReject{
			QuoteReject: &mmv1.QuoteReject{
				QuoteId: req.QuoteId,
				ChainId: req.ChainId,
				MmId:    strings.ToLower(h.signer.GetAddress().Hex()),
				Reason:  reason,
				Message: message,
			},
		},
	}
}
