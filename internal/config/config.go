package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config application configuration
type Config struct {
	App           AppConfig           `yaml:"app"`
	Signer        SignerConfig        `yaml:"signer"`
	WebSocket     WebSocketConfig     `yaml:"websocket"`
	EIP712Domains []EIP712Domain      `yaml:"eip712Domains"`
	Quote         QuoteConfig         `yaml:"quote"`
	Depth         DepthConfig         `yaml:"depth"`
	Pairs         []PairConfig        `yaml:"pairs"`
}

// AppConfig application basic configuration
type AppConfig struct {
	Name     string `yaml:"name"`
	LogLevel string `yaml:"logLevel"` // debug, info, warn, error
}

// SignerConfig signer configuration
type SignerConfig struct {
	PrivateKey    string `yaml:"privateKey"`    // Private key (hexadecimal, highest priority)
	PrivateKeyEnv string `yaml:"privateKeyEnv"` // Private key environment variable name (fallback)
}

// GetPrivateKey gets private key (prioritizes config file, falls back to environment variable)
func (c *SignerConfig) GetPrivateKey() (string, error) {
	if c.PrivateKey != "" {
		return strings.TrimPrefix(strings.TrimSpace(c.PrivateKey), "0x"), nil
	}
	if c.PrivateKeyEnv != "" {
		key := os.Getenv(c.PrivateKeyEnv)
		if key == "" {
			return "", fmt.Errorf("environment variable %s is not set", c.PrivateKeyEnv)
		}
		return strings.TrimPrefix(strings.TrimSpace(key), "0x"), nil
	}
	return "", fmt.Errorf("neither privateKey nor privateKeyEnv is configured")
}

// WebSocketConfig WebSocket configuration
type WebSocketConfig struct {
	ServerURL            string        `yaml:"serverUrl"`
	APIToken             string        `yaml:"apiToken"`
	ReconnectInterval    time.Duration `yaml:"reconnectInterval"`
	MaxReconnectAttempts int           `yaml:"maxReconnectAttempts"` // 0 = unlimited
	HeartbeatInterval    time.Duration `yaml:"heartbeatInterval"`
	ReadTimeout          time.Duration `yaml:"readTimeout"`
	WriteTimeout         time.Duration `yaml:"writeTimeout"`
}

// EIP712Domain EIP-712 Domain configuration
type EIP712Domain struct {
	ChainID           uint64 `yaml:"chainId"`
	Name              string `yaml:"name"`
	Version           string `yaml:"version"`
	VerifyingContract string `yaml:"verifyingContract"`
}

// QuoteConfig quote configuration
type QuoteConfig struct {
	ValidDuration time.Duration `yaml:"validDuration"` // Quote validity period
}

// DepthConfig depth push configuration
type DepthConfig struct {
	Enabled      bool          `yaml:"enabled"`
	PushInterval time.Duration `yaml:"pushInterval"`
}

// PairConfig trading pair configuration
type PairConfig struct {
	ChainID            uint64 `yaml:"chainId"`
	PairID             string `yaml:"pairId"`
	PoolAddress        string `yaml:"poolAddress"`
	BaseToken          string `yaml:"baseToken"`
	QuoteToken         string `yaml:"quoteToken"`
	BaseTokenDecimals  int    `yaml:"baseTokenDecimals"`
	QuoteTokenDecimals int    `yaml:"quoteTokenDecimals"`
	FeeRate            uint32 `yaml:"feeRate"` // Fee rate (basis points)
}

// Load loads configuration from file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	cfg.setDefaults()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default values
func (c *Config) setDefaults() {
	if c.App.Name == "" {
		c.App.Name = "mm-example"
	}
	if c.App.LogLevel == "" {
		c.App.LogLevel = "info"
	}
	if c.WebSocket.ReconnectInterval == 0 {
		c.WebSocket.ReconnectInterval = 5 * time.Second
	}
	if c.WebSocket.HeartbeatInterval == 0 {
		c.WebSocket.HeartbeatInterval = 30 * time.Second
	}
	if c.WebSocket.ReadTimeout == 0 {
		c.WebSocket.ReadTimeout = 90 * time.Second
	}
	if c.WebSocket.WriteTimeout == 0 {
		c.WebSocket.WriteTimeout = 10 * time.Second
	}
	if c.Quote.ValidDuration == 0 {
		c.Quote.ValidDuration = 30 * time.Second
	}
	if c.Depth.PushInterval == 0 {
		c.Depth.PushInterval = 3 * time.Second
	}
}

// Validate validates configuration
func (c *Config) Validate() error {
	if c.WebSocket.ServerURL == "" {
		return fmt.Errorf("websocket.serverUrl is required")
	}
	if c.WebSocket.APIToken == "" {
		return fmt.Errorf("websocket.apiToken is required")
	}
	if len(c.EIP712Domains) == 0 {
		return fmt.Errorf("at least one eip712Domain is required")
	}
	for i, domain := range c.EIP712Domains {
		if domain.ChainID == 0 {
			return fmt.Errorf("eip712Domains[%d].chainId is required", i)
		}
		if domain.VerifyingContract == "" {
			return fmt.Errorf("eip712Domains[%d].verifyingContract is required", i)
		}
	}
	return nil
}

// GetEIP712Domain gets EIP-712 Domain by chain ID
func (c *Config) GetEIP712Domain(chainID uint64) *EIP712Domain {
	for _, domain := range c.EIP712Domains {
		if domain.ChainID == chainID {
			return &domain
		}
	}
	return nil
}

// GetPairConfig gets trading pair configuration by chain ID and token addresses
func (c *Config) GetPairConfig(chainID uint64, tokenIn, tokenOut string) *PairConfig {
	tokenInLower := strings.ToLower(tokenIn)
	tokenOutLower := strings.ToLower(tokenOut)

	for _, pair := range c.Pairs {
		if pair.ChainID != chainID {
			continue
		}
		baseLower := strings.ToLower(pair.BaseToken)
		quoteLower := strings.ToLower(pair.QuoteToken)

		// Bidirectional matching
		if (tokenInLower == baseLower && tokenOutLower == quoteLower) ||
			(tokenInLower == quoteLower && tokenOutLower == baseLower) {
			return &pair
		}
	}
	return nil
}
