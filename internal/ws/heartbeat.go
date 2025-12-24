package ws

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	mmv1 "github.com/ThetaSpace/DarkPool-Market-Maker-Example/mm/v1"
)

// HeartbeatConfig heartbeat configuration
type HeartbeatConfig struct {
	Interval    time.Duration // Heartbeat interval
	ReadTimeout time.Duration // Read timeout (triggers reconnection on timeout)
}

// Heartbeat heartbeat manager
type Heartbeat struct {
	client          WSClient
	config          *HeartbeatConfig
	logger          *slog.Logger
	lastReceived    atomic.Int64 // Last message received time (Unix nanoseconds)
	timeoutDetected atomic.Bool  // Timeout detection flag (avoid duplicate logs)
}

// NewHeartbeat creates a heartbeat manager
func NewHeartbeat(client WSClient, config *HeartbeatConfig, logger *slog.Logger) *Heartbeat {
	if config == nil {
		config = &HeartbeatConfig{
			Interval:    30 * time.Second,
			ReadTimeout: 90 * time.Second,
		}
	}
	if logger == nil {
		logger = slog.Default()
	}

	h := &Heartbeat{
		client: client,
		config: config,
		logger: logger,
	}
	h.lastReceived.Store(time.Now().UnixNano())
	return h
}

// Start starts heartbeat detection
func (h *Heartbeat) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(h.config.Interval)
	defer ticker.Stop()

	h.logger.Info("Heartbeat started",
		"interval", h.config.Interval,
		"timeout", h.config.ReadTimeout)

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Heartbeat stopped")
			return
		case <-ticker.C:
			h.check()
		}
	}
}

// check checks heartbeat status
func (h *Heartbeat) check() {
	lastReceived := time.Unix(0, h.lastReceived.Load())
	elapsed := time.Since(lastReceived)

	if elapsed > h.config.ReadTimeout {
		// Timeout, trigger reconnection
		if !h.timeoutDetected.Swap(true) {
			// Only log on first timeout detection
			h.logger.Warn("Heartbeat timeout detected, triggering reconnect",
				"elapsed", elapsed,
				"timeout", h.config.ReadTimeout)
		}
		h.client.TriggerReconnect()
		return
	}

	// Reset timeout detection flag
	h.timeoutDetected.Store(false)

	// Send heartbeat ping
	if err := h.sendPing(); err != nil {
		h.logger.Error("Failed to send heartbeat ping", "error", err)
	}
}

// sendPing sends heartbeat ping
func (h *Heartbeat) sendPing() error {
	msg := &mmv1.Message{
		Type:      mmv1.MessageType_MESSAGE_TYPE_HEARTBEAT,
		Timestamp: time.Now().UnixMilli(),
		Payload: &mmv1.Message_Heartbeat{
			Heartbeat: &mmv1.Heartbeat{
				Ping: true,
				Pong: false,
			},
		},
	}

	return h.client.Send(msg)
}

// OnMessageReceived called when message is received, updates last received time
func (h *Heartbeat) OnMessageReceived() {
	h.lastReceived.Store(time.Now().UnixNano())
}

// LastReceivedTime gets the last message received time
func (h *Heartbeat) LastReceivedTime() time.Time {
	return time.Unix(0, h.lastReceived.Load())
}
