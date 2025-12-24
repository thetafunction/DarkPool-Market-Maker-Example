package ws

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	mmv1 "github.com/ThetaSpace/DarkPool-Market-Maker-Example/mm/v1"
)

// ConnectionState WebSocket connection state
type ConnectionState int32

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
	StateReady
)

// String returns the string representation of the state
func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "Disconnected"
	case StateConnecting:
		return "Connecting"
	case StateConnected:
		return "Connected"
	case StateReady:
		return "Ready"
	default:
		return "Unknown"
	}
}

// MessageHandler message handler callback function type
type MessageHandler func(msg *mmv1.Message) error

// ReconnectedHandler reconnection success callback function type
type ReconnectedHandler func()

// WSClient WebSocket client interface
type WSClient interface {
	// Connect establishes WebSocket connection
	Connect(ctx context.Context) error
	// Close closes the connection
	Close() error
	// Send sends a Protobuf message
	Send(msg *mmv1.Message) error
	// SetMessageHandler sets the message handler callback
	SetMessageHandler(handler MessageHandler)
	// SetReconnectedHandler sets the reconnection success callback
	SetReconnectedHandler(handler ReconnectedHandler)
	// IsConnected checks if connected
	IsConnected() bool
	// GetState gets current connection state
	GetState() ConnectionState
	// SetState sets connection state
	SetState(state ConnectionState)
	// TriggerReconnect manually triggers reconnection
	TriggerReconnect()
}

// Config WebSocket client configuration
type Config struct {
	ServerURL            string        // WebSocket server address
	APIToken             string        // API Token (JWT, for authentication)
	ReconnectInterval    time.Duration // Base reconnection interval
	MaxReconnectAttempts int           // Maximum reconnection attempts (0=unlimited)
	HeartbeatInterval    time.Duration // Heartbeat interval
	ReadTimeout          time.Duration // Read timeout
	WriteTimeout         time.Duration // Write timeout
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		ReconnectInterval:    5 * time.Second,
		MaxReconnectAttempts: 0, // Unlimited reconnection
		HeartbeatInterval:    30 * time.Second,
		ReadTimeout:          90 * time.Second,
		WriteTimeout:         10 * time.Second,
	}
}

// client WebSocket client implementation
type client struct {
	config *Config
	conn   *websocket.Conn
	state  atomic.Int32
	logger *slog.Logger

	handler            MessageHandler
	reconnectedHandler ReconnectedHandler
	mu                 sync.RWMutex
	writeMu            sync.Mutex // Protects write operation concurrency

	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	closeCh    chan struct{}
	reconnectC chan struct{}

	// Reconnection control
	reconnector *Reconnector
	heartbeat   *Heartbeat
	isReconnect bool

	// Heartbeat context
	heartbeatCtx    context.Context
	heartbeatCancel context.CancelFunc
}

// NewClient creates a new WebSocket client
func NewClient(config *Config, logger *slog.Logger) WSClient {
	if config == nil {
		config = DefaultConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}

	c := &client{
		config:     config,
		logger:     logger,
		closeCh:    make(chan struct{}),
		reconnectC: make(chan struct{}, 1),
	}

	c.state.Store(int32(StateDisconnected))

	// Create reconnector
	c.reconnector = NewReconnector(&ReconnectConfig{
		InitialInterval: config.ReconnectInterval,
		MaxInterval:     config.ReconnectInterval * 32, // Maximum 32x base interval
		MaxAttempts:     config.MaxReconnectAttempts,
	})

	return c
}

// Connect establishes WebSocket connection
func (c *client) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.GetState() != StateDisconnected {
		c.mu.Unlock()
		return fmt.Errorf("client already connected or connecting")
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.closeCh = make(chan struct{})
	c.mu.Unlock()

	return c.doConnect()
}

// doConnect performs the actual connection operation
func (c *client) doConnect() error {
	c.SetState(StateConnecting)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// Build request header, add token authentication
	header := http.Header{}
	if c.config.APIToken != "" {
		header.Set("Authorization", "Bearer "+c.config.APIToken)
	}

	conn, resp, err := dialer.DialContext(c.ctx, c.config.ServerURL, header)
	if err != nil {
		c.SetState(StateDisconnected)
		if resp != nil {
			c.logger.Error("WebSocket dial failed",
				"status", resp.StatusCode,
				"url", c.config.ServerURL,
				"error", err)
		} else {
			c.logger.Error("WebSocket dial failed",
				"url", c.config.ServerURL,
				"error", err)
		}
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	c.SetState(StateConnected)
	c.logger.Info("WebSocket connected", "url", c.config.ServerURL)

	// Start heartbeat
	c.stopHeartbeat()
	c.heartbeat = NewHeartbeat(c, &HeartbeatConfig{
		Interval:    c.config.HeartbeatInterval,
		ReadTimeout: c.config.ReadTimeout,
	}, c.logger)
	c.heartbeatCtx, c.heartbeatCancel = context.WithCancel(c.ctx)

	// Start read loop
	c.wg.Add(1)
	go c.readLoop()

	// Start heartbeat
	c.wg.Add(1)
	go c.heartbeat.Start(c.heartbeatCtx, &c.wg)

	// Reset reconnector
	c.reconnector.Reset()

	// If reconnection succeeded, call reconnection callback
	if c.isReconnect {
		c.mu.RLock()
		handler := c.reconnectedHandler
		c.mu.RUnlock()

		if handler != nil {
			c.logger.Info("WebSocket reconnected, invoking reconnected handler")
			go handler() // Async call to avoid blocking
		}
		c.isReconnect = false
	}

	return nil
}

// Close closes the connection
func (c *client) Close() error {
	c.mu.Lock()
	if c.GetState() == StateDisconnected {
		c.mu.Unlock()
		return nil
	}

	// Cancel context
	if c.cancel != nil {
		c.cancel()
	}
	c.stopHeartbeat()

	// Close closeCh
	select {
	case <-c.closeCh:
		// Already closed
	default:
		close(c.closeCh)
	}
	c.mu.Unlock()

	// Wait for all goroutines to finish
	c.wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Close WebSocket connection
	if c.conn != nil {
		_ = c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second),
		)
		_ = c.conn.Close()
		c.conn = nil
	}

	c.SetState(StateDisconnected)
	c.logger.Info("WebSocket connection closed")

	return nil
}

// Send sends a Protobuf message
func (c *client) Send(msg *mmv1.Message) error {
	if !c.IsConnected() {
		return fmt.Errorf("websocket not connected")
	}

	// Serialize message
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Lock to ensure write operation atomicity
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("websocket connection is nil")
	}

	// Set write timeout
	if err := conn.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout)); err != nil {
		c.triggerReconnect()
		return fmt.Errorf("failed to set write deadline: %w", err)
	}

	// Send binary message
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		c.triggerReconnect()
		return fmt.Errorf("failed to write message: %w", err)
	}

	c.logger.Debug("Message sent", "type", msg.Type.String())
	return nil
}

// SetMessageHandler sets the message handler callback
func (c *client) SetMessageHandler(handler MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

// SetReconnectedHandler sets the reconnection success callback
func (c *client) SetReconnectedHandler(handler ReconnectedHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reconnectedHandler = handler
}

// IsConnected checks if connected
func (c *client) IsConnected() bool {
	state := c.GetState()
	return state == StateConnected || state == StateReady
}

// GetState gets current connection state
func (c *client) GetState() ConnectionState {
	return ConnectionState(c.state.Load())
}

// SetState sets connection state
func (c *client) SetState(state ConnectionState) {
	old := ConnectionState(c.state.Swap(int32(state)))
	if old != state {
		c.logger.Info("WebSocket state changed", "from", old.String(), "to", state.String())
	}
}

// readLoop message reading loop
func (c *client) readLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.closeCh:
			return
		case <-c.ctx.Done():
			return
		default:
		}

		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			return
		}

		// Set read timeout
		if err := conn.SetReadDeadline(time.Now().Add(c.config.ReadTimeout)); err != nil {
			c.logger.Error("Failed to set read deadline", "error", err)
			c.triggerReconnect()
			return
		}

		// Read message
		wsMsgType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				c.logger.Info("WebSocket closed by server")
			} else {
				c.logger.Error("WebSocket read error", "error", err)
			}
			c.triggerReconnect()
			return
		}

		// Only handle binary messages
		if wsMsgType != websocket.BinaryMessage {
			c.logger.Warn("Received non-binary message", "type", wsMsgType)
			continue
		}

		// Deserialize message
		msg := &mmv1.Message{}
		if err := proto.Unmarshal(data, msg); err != nil {
			c.logger.Error("Failed to unmarshal message", "error", err)
			continue
		}

		c.logger.Debug("Message received", "type", msg.Type.String())

		// Update heartbeat time
		if c.heartbeat != nil {
			c.heartbeat.OnMessageReceived()
		}

		// Call handler callback
		c.mu.RLock()
		handler := c.handler
		c.mu.RUnlock()

		if handler != nil {
			if err := handler(msg); err != nil {
				c.logger.Error("Message handler error", "error", err)
			}
		}
	}
}

// triggerReconnect triggers reconnection (internal use)
func (c *client) triggerReconnect() {
	c.TriggerReconnect()
}

// TriggerReconnect manually triggers reconnection (public method)
func (c *client) TriggerReconnect() {
	select {
	case c.reconnectC <- struct{}{}:
		go c.reconnectLoop()
	default:
		// Reconnection already in progress
	}
}

// reconnectLoop reconnection loop
func (c *client) reconnectLoop() {
	c.stopHeartbeat()
	c.mu.Lock()
	// Close old connection
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()

	c.SetState(StateDisconnected)

	for {
		select {
		case <-c.closeCh:
			return
		case <-c.ctx.Done():
			return
		default:
		}

		// Check if reconnection should continue
		if !c.reconnector.ShouldReconnect() {
			c.logger.Error("Max reconnect attempts reached, giving up")
			return
		}

		// Wait for reconnection interval
		interval := c.reconnector.NextInterval()
		c.logger.Info("Reconnecting",
			"interval", interval,
			"attempt", c.reconnector.Attempts())

		select {
		case <-time.After(interval):
		case <-c.closeCh:
			return
		case <-c.ctx.Done():
			return
		}

		// Mark as reconnection state
		c.isReconnect = true

		// Attempt reconnection
		if err := c.doConnect(); err != nil {
			c.logger.Error("Reconnect failed", "error", err)
			c.isReconnect = false
			continue
		}

		// Reconnection successful, clear reconnectC
		select {
		case <-c.reconnectC:
		default:
		}
		return
	}
}

// stopHeartbeat stops current heartbeat goroutine
func (c *client) stopHeartbeat() {
	if c.heartbeatCancel != nil {
		c.heartbeatCancel()
		c.heartbeatCancel = nil
	}
	c.heartbeat = nil
}
