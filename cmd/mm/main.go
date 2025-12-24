package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"os"

	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/config"
	"github.com/ThetaSpace/DarkPool-Market-Maker-Example/internal/runner"
)

func main() {
	// Parse command line arguments
	configPath := flag.String("config", "configs/config.yaml", "Path to config file")
	flag.Parse()

	// Initialize logger
	logger := setupLogger()

	logger.Info("Starting DarkPool Market Maker Example",
		"configPath", *configPath)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	logger.Info("Config loaded successfully",
		"app", cfg.App.Name,
		"pairs", len(cfg.Pairs),
		"domains", len(cfg.EIP712Domains))

	// Create and run service
	r, err := runner.New(cfg, logger)
	if err != nil {
		logger.Error("Failed to create runner", "error", err)
		os.Exit(1)
	}

	if err := r.Run(context.Background()); err != nil {
		logger.Error("Service error", "error", err)
		os.Exit(1)
	}
}

// setupLogger initializes the logger
func setupLogger() *slog.Logger {
	// Create logs directory
	if err := os.MkdirAll("logs", 0755); err != nil {
		slog.Error("Failed to create logs directory", "error", err)
	}

	// Open log file
	logFile, err := os.OpenFile("logs/mm.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		slog.Error("Failed to open log file", "error", err)
		// Fallback to stdout
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}

	// Output to both file and stdout
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	return slog.New(slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}
