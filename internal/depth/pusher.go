package depth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/config"
	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/quote"
	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/ws"
	mmv1 "github.com/ThetaSpace/DarkPool-Market-Maker-Example/mm/v1"
)

// Pusher is the depth data pusher
// Periodically retrieves depth data and pushes via WebSocket
type Pusher struct {
	wsClient     ws.WSClient
	provider     DepthProvider
	quoteHandler *quote.Handler
	cfg          *config.Config
	logger       *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewPusher creates a new depth pusher
func NewPusher(
	wsClient ws.WSClient,
	provider DepthProvider,
	quoteHandler *quote.Handler,
	cfg *config.Config,
	logger *slog.Logger,
) *Pusher {
	return &Pusher{
		wsClient:     wsClient,
		provider:     provider,
		quoteHandler: quoteHandler,
		cfg:          cfg,
		logger:       logger.With("component", "DepthPusher"),
	}
}

// Start starts the pusher
func (p *Pusher) Start(ctx context.Context) error {
	p.ctx, p.cancel = context.WithCancel(ctx)

	// Set message handler callback
	p.wsClient.SetMessageHandler(p.handleMessage)

	// Set reconnection callback
	p.wsClient.SetReconnectedHandler(p.onReconnected)

	// Start periodic push
	if p.cfg.Depth.Enabled {
		p.wg.Add(1)
		go p.pushLoop()
	}

	p.logger.Info("Depth pusher started", "enabled", p.cfg.Depth.Enabled)
	return nil
}

// Stop stops the pusher
func (p *Pusher) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	p.logger.Info("Depth pusher stopped")
	return nil
}

// pushLoop is the periodic push loop
func (p *Pusher) pushLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.cfg.Depth.PushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.pushAllPairs()
		}
	}
}

// pushAllPairs pushes depth data for all trading pairs
func (p *Pusher) pushAllPairs() {
	// Only push when in Ready state
	if p.wsClient.GetState() != ws.StateReady {
		p.logger.Debug("WebSocket not ready, skipping depth push",
			"state", p.wsClient.GetState().String())
		return
	}

	for _, pair := range p.cfg.Pairs {
		if err := p.pushDepthSnapshot(pair); err != nil {
			p.logger.Error("Failed to push depth snapshot",
				"chainId", pair.ChainID,
				"pairId", pair.PairID,
				"error", err)
		}
	}
}

// pushDepthSnapshot pushes depth snapshot for a single trading pair
func (p *Pusher) pushDepthSnapshot(pair config.PairConfig) error {
	// Get depth data
	orderBook, err := p.provider.GetDepth(pair.ChainID, pair.PoolAddress)
	if err != nil {
		return fmt.Errorf("failed to get depth: %w", err)
	}

	// Get EIP712 Domain for MM address
	domain := p.cfg.GetEIP712Domain(pair.ChainID)
	if domain == nil {
		return fmt.Errorf("eip712 domain not found for chain %d", pair.ChainID)
	}

	// Build depth snapshot
	snapshot := p.buildDepthSnapshot(orderBook, pair, domain)

	// Build message
	msg := &mmv1.Message{
		Type:      mmv1.MessageType_MESSAGE_TYPE_DEPTH_SNAPSHOT,
		Timestamp: time.Now().UnixMilli(),
		Payload: &mmv1.Message_DepthSnapshot{
			DepthSnapshot: snapshot,
		},
	}

	// Send
	if err := p.wsClient.Send(msg); err != nil {
		return fmt.Errorf("failed to send depth snapshot: %w", err)
	}

	p.logger.Info("Depth snapshot sent",
		"chainId", pair.ChainID,
		"pairId", pair.PairID,
		"asks", len(snapshot.Asks),
		"bids", len(snapshot.Bids))

	return nil
}

// buildDepthSnapshot builds the depth snapshot message
//
// SwapEngine expected format:
// - Price: wei/wei ratio (tokenBWei / tokenAWei, no decimals adjustment)
// - Amount: tokenA (baseToken) native decimals quantity
//
// Example: tokenA = WETH (18 decimals), tokenB = USDC (6 decimals), 1 WETH = 3400 USDC
//   - Price = 3400 * 10^6 / 10^18 = 3.4e-9 = "0.0000000034"
//   - Amount = 3.28e18 = "3280000000000000000"
func (p *Pusher) buildDepthSnapshot(ob *OrderBook, pair config.PairConfig, domain *config.EIP712Domain) *mmv1.DepthSnapshot {
	// Use timestamp as sequence number
	seqId := uint64(time.Now().UnixMilli())

	// Mid price (wei/wei format)
	var midPriceStr string
	if ob.MidPrice != nil {
		// Use high precision formatting, keep 30 decimal places (wei/wei ratio can be very small)
		midPriceStr = ob.MidPrice.Text('f', 30)
	}

	// Build asks and bids
	// Price: wei/wei format, Amount: tokenA native decimals
	asks := make([]*mmv1.PriceLevel, len(ob.Asks))
	for i, level := range ob.Asks {
		asks[i] = &mmv1.PriceLevel{
			Price:  level.Price.Text('f', 30), // wei/wei format, requires high precision
			Amount: level.Amount.String(),     // tokenA native decimals
		}
	}

	bids := make([]*mmv1.PriceLevel, len(ob.Bids))
	for i, level := range ob.Bids {
		bids[i] = &mmv1.PriceLevel{
			Price:  level.Price.Text('f', 30), // wei/wei format, requires high precision
			Amount: level.Amount.String(),     // tokenA native decimals
		}
	}

	return &mmv1.DepthSnapshot{
		ChainId:     pair.ChainID,
		ChainName:   getChainName(pair.ChainID),
		PairId:      pair.PairID,
		MmId:        strings.ToLower(domain.VerifyingContract), // Use pool address as MmID
		FeeRate:     pair.FeeRate,
		MmAddress:   strings.ToLower(domain.VerifyingContract),
		PoolAddress: strings.ToLower(pair.PoolAddress),
		TokenA:      strings.ToLower(pair.BaseToken),
		TokenB:      strings.ToLower(pair.QuoteToken),
		MidPrice:    midPriceStr,
		Spread:      fmt.Sprintf("%.6f", ob.Spread),
		Asks:        asks,
		Bids:        bids,
		BlockNumber: 0, // Mock data has no block number
		SequenceId:  seqId,
	}
}

// onReconnected is the reconnection success callback
func (p *Pusher) onReconnected() {
	p.logger.Info("WebSocket reconnected, will push depth on next tick")
	// Push depth data immediately after reconnection (will only send after ConnectionAck)
}

// handleMessage handles received messages
func (p *Pusher) handleMessage(msg *mmv1.Message) error {
	switch msg.Type {
	case mmv1.MessageType_MESSAGE_TYPE_QUOTE_REQUEST:
		return p.handleQuoteRequest(msg.GetQuoteRequest())
	case mmv1.MessageType_MESSAGE_TYPE_HEARTBEAT:
		return p.handleHeartbeat(msg.GetHeartbeat())
	case mmv1.MessageType_MESSAGE_TYPE_CONNECTION_ACK:
		return p.handleConnectionAck(msg.GetConnectionAck())
	case mmv1.MessageType_MESSAGE_TYPE_ERROR:
		return p.handleError(msg.GetError())
	default:
		p.logger.Debug("Received unknown message type", "type", msg.Type)
	}
	return nil
}

// handleQuoteRequest handles quote requests
func (p *Pusher) handleQuoteRequest(req *mmv1.QuoteRequest) error {
	if req == nil {
		return nil
	}

	p.logger.Info("Received quote request",
		"quoteId", req.QuoteId,
		"chainId", req.ChainId,
		"tokenIn", req.TokenIn,
		"tokenOut", req.TokenOut,
		"amountIn", req.AmountIn)

	// Call QuoteHandler to process
	response, err := p.quoteHandler.HandleQuoteRequest(p.ctx, req)
	if err != nil {
		p.logger.Error("Quote handling failed", "error", err)
		return err
	}

	// Send response
	if err := p.wsClient.Send(response); err != nil {
		p.logger.Error("Failed to send quote response", "error", err)
		return err
	}

	p.logger.Info("Quote response sent", "quoteId", req.QuoteId, "type", response.Type)
	return nil
}

// handleHeartbeat handles heartbeat messages
func (p *Pusher) handleHeartbeat(hb *mmv1.Heartbeat) error {
	if hb == nil {
		return nil
	}

	// Received ping, reply with pong
	if hb.Ping {
		p.logger.Debug("Received ping, replying pong")
		pong := &mmv1.Message{
			Type:      mmv1.MessageType_MESSAGE_TYPE_HEARTBEAT,
			Timestamp: time.Now().UnixMilli(),
			Payload: &mmv1.Message_Heartbeat{
				Heartbeat: &mmv1.Heartbeat{Pong: true},
			},
		}
		return p.wsClient.Send(pong)
	}

	// Received pong
	if hb.Pong {
		p.logger.Debug("Received pong from server")
	}

	return nil
}

// handleConnectionAck handles connection acknowledgment
func (p *Pusher) handleConnectionAck(ack *mmv1.ConnectionAck) error {
	if ack == nil {
		return nil
	}

	if ack.Success {
		p.logger.Info("Connection successful",
			"sessionId", ack.SessionId,
			"mmAddress", ack.MmAddress)
		// Set to Ready state
		p.wsClient.SetState(ws.StateReady)

		// Push depth data immediately after successful connection
		go p.pushAllPairs()
	} else {
		p.logger.Error("Connection failed", "error", ack.ErrorMessage)
	}

	return nil
}

// handleError handles error messages
func (p *Pusher) handleError(err *mmv1.Error) error {
	if err == nil {
		return nil
	}

	p.logger.Error("Received error from server",
		"code", err.Code,
		"message", err.Message,
		"relatedQuoteId", err.RelatedQuoteId)
	return nil
}

// getChainName returns the chain name for a given chain ID
func getChainName(chainID uint64) string {
	switch chainID {
	case 1:
		return "ethereum"
	case 56:
		return "bsc"
	case 8453:
		return "base"
	case 42161:
		return "arbitrum"
	case 10:
		return "optimism"
	default:
		return fmt.Sprintf("chain_%d", chainID)
	}
}
