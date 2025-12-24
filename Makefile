.PHONY: build run clean test proto help

# Project settings
PROJECT_NAME := mm
BINARY := bin/$(PROJECT_NAME)
CONFIG := configs/config.yaml

# Go settings
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOMOD := $(GOCMD) mod

# Default target
.DEFAULT_GOAL := help

## build: Build the binary
build:
	@echo "Building..."
	@mkdir -p bin
	@$(GOBUILD) -o $(BINARY) ./cmd/mm
	@echo "Binary built: $(BINARY)"

## run: Run the application
run: build
	@echo "Running..."
	@./$(BINARY) -config $(CONFIG)

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -rf logs/*.log
	@echo "Done."

## test: Run tests
test:
	@echo "Running tests..."
	@$(GOTEST) -v ./...

## proto: Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	@./scripts/gen-proto.sh

## tidy: Tidy go modules
tidy:
	@echo "Tidying modules..."
	@$(GOMOD) tidy

## fmt: Format code
fmt:
	@echo "Formatting code..."
	@$(GOCMD) fmt ./...

## vet: Vet code
vet:
	@echo "Vetting code..."
	@$(GOCMD) vet ./...

## lint: Run linter
lint: fmt vet
	@echo "Linting done."

## help: Show this help
help:
	@echo ""
	@echo "DarkPool Market Maker Example"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
	@echo ""
	@echo "Examples:"
	@echo "  make build         Build the binary"
	@echo "  make run           Build and run the application"
	@echo "  make test          Run tests"
	@echo "  make proto         Regenerate protobuf code"
	@echo ""
