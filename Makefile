# Makefile for pinguin

.PHONY: install win clean help

# Binary name
BINARY_NAME=pinguin
WINDOWS_BINARY_NAME=pinguin.exe

# Version and build
VERSION?=dev
BUILD_TIME:=$(shell date +%Y-%m-%d_%H-%M-%S)
GIT_COMMIT:=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build flags
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)"

# Default target
help:
	@echo "Available targets:"
	@echo "  make install  - install binary for current platform"
	@echo "  make win      - build Windows executable (pinguin.exe)"
	@echo "  make clean    - remove compiled files"
	@echo "  make help     - show this help"

# Install for current platform
install:
	@echo "Installing for $(shell go env GOOS)/$(shell go env GOARCH)..."
	go install $(LDFLAGS)
	@echo "Install complete"

# Build Windows executable
win:
	@echo "Building for Windows (amd64)..."
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(WINDOWS_BINARY_NAME)
	@echo "Build complete: $(WINDOWS_BINARY_NAME)"

# Clean compiled files
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME) $(WINDOWS_BINARY_NAME)
	@echo "Clean complete"