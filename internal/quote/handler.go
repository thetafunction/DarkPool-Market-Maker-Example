package quote

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"google.golang.org/protobuf/proto"

	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/config"
	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/signer"
	mmv1 "github.com/ThetaSpace/DarkPool-Market-Maker-Example/mm/v1"
)

type Handler struct {
	strategy QuoteStrategy
	signer   signer.Signer
	cfg      *config.Config
	logger   *slog.Logger
}

func NewHandler(strategy QuoteStrategy, s signer.Signer, cfg *config.Config, logger *slog.Logger) *Handler {
	return &Handler{
		strategy: strategy,
		signer:   s,
		cfg:      cfg,
		logger:   logger.With("component", "QuoteHandler"),
	}
}

func (h *Handler) HandleQuoteRequest(ctx context.Context, req *mmv1.QuoteRequest) (*mmv1.Message, error) {
	h.logger.Info("received quote request",
		"quoteId", req.QuoteId,
		"chainId", req.ChainId,
		"protocolVersion", req.ProtocolVersion)

	if err := h.validateEnvelope(req); err != nil {
		h.logger.Error("quote envelope validation failed", "error", err)
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR, err.Error()), nil
	}

	switch req.ProtocolVersion {
	case "v1":
		return h.handleQuoteRequestV1(ctx, req)
	default:
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_VERSION_NOT_SUPPORTED,
			fmt.Sprintf("unsupported protocol version: %s", req.ProtocolVersion)), nil
	}
}

func (h *Handler) handleQuoteRequestV1(ctx context.Context, req *mmv1.QuoteRequest) (*mmv1.Message, error) {
	var reqV1 mmv1.QuoteRequestV1
	if err := proto.Unmarshal(req.QuoteRequestData, &reqV1); err != nil {
		h.logger.Error("failed to unmarshal QuoteRequestV1", "error", err)
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR,
			"failed to unmarshal QuoteRequestV1"), nil
	}

	if err := h.validateRequestV1(req, &reqV1); err != nil {
		h.logger.Error("quote request validation failed", "error", err)
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR, err.Error()), nil
	}

	domain := h.cfg.GetEIP712Domain(req.ChainId)
	if domain == nil {
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_PAIR_NOT_SUPPORTED,
			fmt.Sprintf("chain %d not configured", req.ChainId)), nil
	}

	tokenIn := common.HexToAddress(reqV1.TokenIn)
	tokenOut := common.HexToAddress(reqV1.TokenOut)
	lookupTokenIn := tokenIn
	lookupTokenOut := tokenOut

	if lookupTokenIn == (common.Address{}) {
		wrappedToken, ok := signer.GetWrappedToken(req.ChainId)
		if !ok {
			return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR,
				fmt.Sprintf("wrapped token not configured for chain %d", req.ChainId)), nil
		}
		lookupTokenIn = wrappedToken
	}

	if lookupTokenOut == (common.Address{}) {
		wrappedToken, ok := signer.GetWrappedToken(req.ChainId)
		if !ok {
			return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR,
				fmt.Sprintf("wrapped token not configured for chain %d", req.ChainId)), nil
		}
		lookupTokenOut = wrappedToken
	}

	if h.cfg.GetPairConfig(req.ChainId, lookupTokenIn.Hex(), lookupTokenOut.Hex()) == nil {
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_PAIR_NOT_SUPPORTED,
			fmt.Sprintf("pair not found for tokens %s-%s", lookupTokenIn.Hex(), lookupTokenOut.Hex())), nil
	}

	amountIn, ok := new(big.Int).SetString(reqV1.AmountIn, 10)
	if !ok {
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR, "invalid amount_in"), nil
	}

	quoteParams := &QuoteParams{
		ChainID:  req.ChainId,
		TokenIn:  lookupTokenIn,
		TokenOut: lookupTokenOut,
		AmountIn: amountIn,
	}

	quoteResult, err := h.strategy.CalculateQuote(ctx, quoteParams)
	if err != nil {
		h.logger.Error("quote calculation failed", "error", err)
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INSUFFICIENT_LIQUIDITY, err.Error()), nil
	}

	nonce, ok := new(big.Int).SetString(reqV1.Nonce, 10)
	if !ok {
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR, "invalid nonce"), nil
	}

	quoteID, err := parseQuoteIDBytes32(req.QuoteId)
	if err != nil {
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR, err.Error()), nil
	}

	normalizedCEV, cevT, cevN, cevM, cevE := normalizeConfidenceExtractedValue(reqV1.GetConfidenceExtractedValue())
	extraData := []byte{}
	mmQuote := &signer.MMQuote{
		QuoteId:                   quoteID,
		Maker:                     h.signer.GetAddress(),
		Vault:                     common.HexToAddress(domain.RFQVault),
		Executor:                  common.HexToAddress(reqV1.Executor),
		InputToken:                tokenIn,
		OutputToken:               tokenOut,
		AmountIn:                  amountIn,
		AmountOut:                 quoteResult.AmountOutMinimum,
		Deadline:                  big.NewInt(reqV1.Deadline),
		Nonce:                     nonce,
		ConfidenceExtractedValueT: cevT,
		ConfidenceExtractedValueN: cevN,
		ConfidenceExtractedValueM: cevM,
		ConfidenceExtractedValueE: cevE,
		ExtraData:                 extraData,
	}

	signature, err := h.signer.SignMMQuote(req.ChainId, mmQuote)
	if err != nil {
		h.logger.Error("signing failed", "error", err)
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR, "signing failed"), nil
	}

	mmQuoteV1 := &mmv1.MMQuoteV1{
		Maker:                    strings.ToLower(h.signer.GetAddress().Hex()),
		Vault:                    strings.ToLower(domain.RFQVault),
		Executor:                 strings.ToLower(reqV1.Executor),
		TokenIn:                  reqV1.TokenIn,
		TokenOut:                 reqV1.TokenOut,
		AmountIn:                 amountIn.String(),
		AmountOut:                quoteResult.AmountOutMinimum.String(),
		Deadline:                 strconv.FormatInt(reqV1.Deadline, 10),
		Nonce:                    nonce.String(),
		ConfidenceExtractedValue: normalizedCEV,
		ExtraData:                extraData,
		MmSignature:              signature,
		QuoteId:                  req.QuoteId,
	}

	mmQuoteData, err := proto.Marshal(mmQuoteV1)
	if err != nil {
		h.logger.Error("failed to marshal MMQuoteV1", "error", err)
		return h.buildRejectMessage(req, mmv1.RejectReason_REJECT_REASON_INTERNAL_ERROR,
			"failed to marshal MMQuoteV1"), nil
	}

	response := &mmv1.QuoteResponse{
		QuoteId:         req.QuoteId,
		ChainId:         req.ChainId,
		MmId:            strings.ToLower(h.signer.GetAddress().Hex()),
		Status:          mmv1.QuoteStatus_QUOTE_STATUS_SUCCESS,
		ProtocolVersion: "v1",
		MmQuoteData:     mmQuoteData,
	}

	return &mmv1.Message{
		Type:      mmv1.MessageType_MESSAGE_TYPE_QUOTE_RESPONSE,
		Timestamp: time.Now().UnixMilli(),
		Payload: &mmv1.Message_QuoteResponse{
			QuoteResponse: response,
		},
	}, nil
}

func (h *Handler) validateEnvelope(req *mmv1.QuoteRequest) error {
	if req.QuoteId == "" {
		return fmt.Errorf("quote_id is required")
	}
	if req.ChainId == 0 {
		return fmt.Errorf("chain_id is required")
	}
	if req.ProtocolVersion == "" {
		return fmt.Errorf("protocol_version is required")
	}
	if len(req.QuoteRequestData) == 0 {
		return fmt.Errorf("quote_request_data is required")
	}
	return nil
}

func (h *Handler) validateRequestV1(req *mmv1.QuoteRequest, reqV1 *mmv1.QuoteRequestV1) error {
	if reqV1.TokenIn == "" {
		return fmt.Errorf("token_in is required")
	}
	if reqV1.TokenOut == "" {
		return fmt.Errorf("token_out is required")
	}
	if reqV1.AmountIn == "" || reqV1.AmountIn == "0" {
		return fmt.Errorf("amount_in is required and must be positive")
	}
	if reqV1.Executor == "" {
		return fmt.Errorf("executor is required")
	}
	if reqV1.Deadline <= 0 {
		return fmt.Errorf("deadline is required")
	}
	if reqV1.Deadline < time.Now().Unix() {
		return fmt.Errorf("deadline already expired")
	}
	if reqV1.Nonce == "" {
		return fmt.Errorf("nonce is required")
	}
	return nil
}

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

func normalizeConfidenceExtractedValue(cev *mmv1.ConfidenceExtractedValue) (*mmv1.ConfidenceExtractedValue, *big.Int, *big.Int, *big.Int, *big.Int) {
	parseOrZero := func(raw string) *big.Int {
		if raw == "" {
			return big.NewInt(0)
		}
		n, ok := new(big.Int).SetString(raw, 10)
		if !ok {
			return big.NewInt(0)
		}
		return n
	}

	if cev == nil {
		cev = &mmv1.ConfidenceExtractedValue{}
	}

	t := parseOrZero(cev.ConfidenceExtractedValueT)
	n := parseOrZero(cev.ConfidenceExtractedValueN)
	m := parseOrZero(cev.ConfidenceExtractedValueM)
	e := parseOrZero(cev.ConfidenceExtractedValueE)

	return &mmv1.ConfidenceExtractedValue{
		ConfidenceExtractedValueT: t.String(),
		ConfidenceExtractedValueN: n.String(),
		ConfidenceExtractedValueM: m.String(),
		ConfidenceExtractedValueE: e.String(),
	}, t, n, m, e
}

func parseQuoteIDBytes32(quoteID string) ([32]byte, error) {
	var out [32]byte
	if quoteID == "" {
		return out, fmt.Errorf("quote_id is required")
	}

	s := strings.TrimPrefix(strings.TrimPrefix(quoteID, "0x"), "0X")
	if len(s) != 64 {
		return out, fmt.Errorf("quote_id must be 32-byte hex")
	}

	raw, err := hex.DecodeString(s)
	if err != nil {
		return out, fmt.Errorf("quote_id must be valid hex")
	}
	copy(out[:], raw)
	return out, nil
}
