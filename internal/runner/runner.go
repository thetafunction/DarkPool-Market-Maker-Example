package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/config"
	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/depth"
	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/quote"
	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/signer"
	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/ws"
)

// Runner is the service runner
// Responsible for orchestrating and starting all components
type Runner struct {
	cfg          *config.Config
	logger       *slog.Logger
	wsClient     ws.WSClient
	signer       signer.Signer
	quoteHandler *quote.Handler
	depthPusher  *depth.Pusher
}

// New creates a service runner
func New(cfg *config.Config, logger *slog.Logger) (*Runner, error) {
	r := &Runner{
		cfg:    cfg,
		logger: logger,
	}

	// 1. Initialize EIP-712 Domain Manager
	domainManager := signer.NewDomainManager()
	for _, domain := range cfg.EIP712Domains {
		domainManager.AddPoolDomainWithConfig(
			domain.ChainID,
			domain.Name,
			domain.Version,
			domain.VerifyingContract,
		)
		logger.Info("Registered EIP-712 domain",
			"chainId", domain.ChainID,
			"verifyingContract", domain.VerifyingContract)
	}

	// 2. Initialize signer
	s, err := signer.NewSignerFromConfig(&signer.SignerConfig{
		PrivateKey:    cfg.Signer.PrivateKey,
		PrivateKeyEnv: cfg.Signer.PrivateKeyEnv,
	}, domainManager)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}
	r.signer = s
	logger.Info("Signer initialized", "address", s.GetAddress().Hex())

	// 3. Initialize WebSocket client
	wsConfig := &ws.Config{
		ServerURL:            cfg.WebSocket.ServerURL,
		APIToken:             cfg.WebSocket.APIToken,
		ReconnectInterval:    cfg.WebSocket.ReconnectInterval,
		MaxReconnectAttempts: cfg.WebSocket.MaxReconnectAttempts,
		HeartbeatInterval:    cfg.WebSocket.HeartbeatInterval,
		ReadTimeout:          cfg.WebSocket.ReadTimeout,
		WriteTimeout:         cfg.WebSocket.WriteTimeout,
	}
	r.wsClient = ws.NewClient(wsConfig, logger)

	// 4. Initialize quote strategy (using mock strategy)
	strategy := quote.DefaultMockStrategy()
	logger.Info("Quote strategy initialized (mock)")

	// 5. Initialize quote handler
	r.quoteHandler = quote.NewHandler(strategy, s, cfg, logger)

	// 6. Initialize depth data provider (using mock provider)
	depthProvider := depth.DefaultMockProvider()
	logger.Info("Depth provider initialized (mock)")

	// 7. Initialize depth pusher
	r.depthPusher = depth.NewPusher(r.wsClient, depthProvider, r.quoteHandler, s, cfg, logger)

	return r, nil
}

// Run runs the service
func (r *Runner) Run(ctx context.Context) error {
	r.logger.Info("Starting Market Maker service",
		"app", r.cfg.App.Name,
		"wsServer", r.cfg.WebSocket.ServerURL)

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Listen for system signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start WebSocket connection
	r.logger.Info("Connecting to WebSocket server...")
	if err := r.wsClient.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	// Start depth pusher
	if err := r.depthPusher.Start(ctx); err != nil {
		return fmt.Errorf("failed to start depth pusher: %w", err)
	}

	r.logger.Info("Market Maker service started successfully")
	r.logger.Info("Waiting for messages...")

	// Wait for signal or context cancellation
	select {
	case sig := <-sigCh:
		r.logger.Info("Received signal, shutting down", "signal", sig)
	case <-ctx.Done():
		r.logger.Info("Context cancelled, shutting down")
	}

	// Graceful shutdown
	return r.Shutdown()
}

// Shutdown gracefully shuts down the service
func (r *Runner) Shutdown() error {
	r.logger.Info("Shutting down Market Maker service...")

	// Stop depth pusher
	if r.depthPusher != nil {
		if err := r.depthPusher.Stop(); err != nil {
			r.logger.Error("Failed to stop depth pusher", "error", err)
		}
	}

	// Close WebSocket connection
	if r.wsClient != nil {
		if err := r.wsClient.Close(); err != nil {
			r.logger.Error("Failed to close WebSocket", "error", err)
		}
	}

	r.logger.Info("Market Maker service stopped")
	return nil
}
