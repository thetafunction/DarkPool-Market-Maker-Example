package ws

import (
	"sync/atomic"
	"time"
)

// ReconnectConfig reconnection configuration
type ReconnectConfig struct {
	InitialInterval time.Duration // Initial reconnection interval
	MaxInterval     time.Duration // Maximum reconnection interval
	MaxAttempts     int           // Maximum reconnection attempts (0=unlimited)
	Multiplier      float64       // Interval multiplier coefficient
}

// DefaultReconnectConfig returns default reconnection configuration
func DefaultReconnectConfig() *ReconnectConfig {
	return &ReconnectConfig{
		InitialInterval: 5 * time.Second,
		MaxInterval:     160 * time.Second, // 5s * 32
		MaxAttempts:     0,                 // Unlimited reconnection
		Multiplier:      2.0,
	}
}

// Reconnector reconnection manager (exponential backoff)
type Reconnector struct {
	config   *ReconnectConfig
	attempts atomic.Int32
	interval time.Duration
}

// NewReconnector creates a reconnection manager
func NewReconnector(config *ReconnectConfig) *Reconnector {
	if config == nil {
		config = DefaultReconnectConfig()
	}
	if config.Multiplier == 0 {
		config.Multiplier = 2.0
	}
	if config.MaxInterval == 0 {
		config.MaxInterval = config.InitialInterval * 32
	}

	return &Reconnector{
		config:   config,
		interval: config.InitialInterval,
	}
}

// ShouldReconnect checks if reconnection should continue
func (r *Reconnector) ShouldReconnect() bool {
	if r.config.MaxAttempts == 0 {
		return true // Unlimited reconnection
	}
	return int(r.attempts.Load()) < r.config.MaxAttempts
}

// NextInterval gets the next reconnection interval (exponential backoff)
func (r *Reconnector) NextInterval() time.Duration {
	r.attempts.Add(1)

	current := r.interval
	// Calculate next interval
	next := time.Duration(float64(r.interval) * r.config.Multiplier)
	if next > r.config.MaxInterval {
		next = r.config.MaxInterval
	}
	r.interval = next

	return current
}

// Attempts gets current attempt count
func (r *Reconnector) Attempts() int {
	return int(r.attempts.Load())
}

// Reset resets reconnection state
func (r *Reconnector) Reset() {
	r.attempts.Store(0)
	r.interval = r.config.InitialInterval
}
