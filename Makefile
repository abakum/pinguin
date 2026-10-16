# Makefile for pinguin

.PHONY: install win mips pi2 pi3 ico syso versioninfo clean help

# Binary name
BINARY_NAME=pinguin
WINDOWS_BINARY_NAME=pinguin.exe

# Version and build: VERSION is the highest git tag (override with VERSION=x.y.z)
VERSION?=$(shell t=$$(git tag --sort=-v:refname 2>/dev/null | head -n1); echo $${t:-dev})
BUILD_TIME:=$(shell date +%Y-%m-%d_%H-%M-%S)
GIT_COMMIT:=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
AUTHOR:=$(shell git config user.name 2>/dev/null || echo "unknown")

# Numeric version parts (for Windows VS_VERSIONINFO); default 0 for non-numeric VERSION
VER_NUM:=$(shell echo $(VERSION) | sed -E 's/^v?([0-9]+\.[0-9]+\.[0-9]+).*/\1/')
VERSION_MAJOR:=$(if $(word 1,$(subst ., ,$(VER_NUM))),$(word 1,$(subst ., ,$(VER_NUM))),0)
VERSION_MINOR:=$(if $(word 2,$(subst ., ,$(VER_NUM))),$(word 2,$(subst ., ,$(VER_NUM))),0)
VERSION_PATCH:=$(if $(word 3,$(subst ., ,$(VER_NUM))),$(word 3,$(subst ., ,$(VER_NUM))),0)

# Windows resources
ICON=pinguin.png
ICO=pinguin.ico
VERSIONINFO=versioninfo.json
SYSO=resource_windows_amd64.syso

# Build flags
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)"

# CLI sender: same source, own syso with asInvoker manifest
# (icon and versioninfo kept, only admin elevation excluded)
WINDOWS_PINGIT=pingit.exe
PINGIT_SYSO=resource_pingit_windows_amd64.syso
ADMIN_MANIFEST=app.manifest
PINGIT_MANIFEST=app_noadmin.manifest
MANIFEST?=$(ADMIN_MANIFEST)

# Default target
help:
	@echo "Available targets:"
	@echo "  make install  - install binary for current platform (longpoll bot, also does CLI sends)"
	@echo "  make win      - build Windows pinguin.exe (admin manifest) and pingit.exe (same source, no manifest)"
	@echo "  make mips     - build linux/mips pinguin (CGO off, softfloat, for LEDE/OpenWrt routers)"
	@echo "  make pi2      - build linux/armv7 pinguin-armv7 (Raspberry Pi 2 and other 32-bit ARMv7 boards)"
	@echo "  make pi3      - build linux/arm64 pinguin-arm64 (Raspberry Pi 3/4/5 with 64-bit OS)"
	@echo "  make ico      - generate $(ICO) from $(ICON)"
	@echo "  make syso     - increment patch tag (vX.Y.Z), push tag to origin, then make win"
	@echo "  make clean    - remove compiled files"
	@echo "  make help     - show this help"

# Install for current platform
install:
	@echo "Installing for $(shell go env GOOS)/$(shell go env GOARCH)..."
	go install $(LDFLAGS)
	@echo "Install complete"

# Generate Windows icon from PNG (only when PNG changes)
$(ICO): $(ICON) cmd/mkico/main.go
	@echo "Generating $(ICO) from $(ICON)..."
	go run ./cmd/mkico $(ICON) $(ICO)
	@echo "Icon complete: $(ICO)"

ico: $(ICO)

# Version info manifest (always rebuilt: embeds VERSION, commit, build time)
versioninfo:
	@printf '%s\n' \
		'{' \
		'  "FixedFileInfo": {' \
		'    "FileVersion": {' \
		'      "Major": $(VERSION_MAJOR), "Minor": $(VERSION_MINOR), "Patch": $(VERSION_PATCH), "Build": 0' \
		'    },' \
		'    "ProductVersion": {' \
		'      "Major": $(VERSION_MAJOR), "Minor": $(VERSION_MINOR), "Patch": $(VERSION_PATCH), "Build": 0' \
		'    },' \
		'    "FileFlagsMask": "3f",' \
		'    "FileFlags": "00",' \
		'    "FileOS": "040004",' \
		'    "FileType": "01",' \
		'    "FileSubType": "00"' \
		'  },' \
		'  "StringFileInfo": {' \
		'    "Comments": "commit $(GIT_COMMIT), built $(BUILD_TIME)",' \
		'    "CompanyName": "$(AUTHOR)",' \
		'    "FileDescription": "pinguin",' \
		'    "FileVersion": "$(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_PATCH)",' \
		'    "InternalName": "pinguin",' \
		'    "LegalCopyright": "Copyright (c) $(AUTHOR)",' \
		'    "OriginalFilename": "pinguin.exe",' \
		'    "ProductName": "pinguin",' \
		'    "ProductVersion": "$(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_PATCH) ($(GIT_COMMIT))"' \
		'  },' \
	'  "IconPath": "$(ICO)",' \
	'  "ManifestPath": "$(MANIFEST)"' \
	'}' > $(VERSIONINFO)

# Generate Windows resource file (icon + version info)
$(SYSO): versioninfo $(ICO) app.manifest
	@echo "Generating $(SYSO)..."
	go tool github.com/josephspurrier/goversioninfo/cmd/goversioninfo -o $(SYSO) $(VERSIONINFO)
	@echo "Resource complete: $(SYSO)"

# Increment patch part of the highest git tag, tag HEAD as vX.Y.Z, push tag, then build pinguin.exe
syso:
	@base=$$(echo $(VERSION) | sed -E 's/^v?([0-9]+\.[0-9]+\.[0-9]+).*/\1/'); \
		[ -n "$$base" ] || base=0.0.0; \
		major=$${base%%.*}; rest=$${base#*.}; minor=$${rest%%.*}; patch=$${rest##*.}; \
		next="v$$major.$$minor.$$((patch+1))"; \
		echo "Tagging $(GIT_COMMIT) as $$next"; \
		git tag $$next && git push origin $$next && $(MAKE) win VERSION=$$next

# Build Windows executables: pingit.exe with asInvoker syso (icon and
# versioninfo kept, no admin elevation), then pinguin.exe with admin manifest
win:
	@echo "Building $(WINDOWS_PINGIT) (no admin manifest)..."
	rm -f $(SYSO) $(PINGIT_SYSO)
	@$(MAKE) versioninfo MANIFEST=$(PINGIT_MANIFEST)
	go tool github.com/josephspurrier/goversioninfo/cmd/goversioninfo -o $(PINGIT_SYSO) $(VERSIONINFO)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(WINDOWS_PINGIT)
	rm -f $(PINGIT_SYSO)
	@echo "Building $(WINDOWS_BINARY_NAME) (admin manifest)..."
	@$(MAKE) versioninfo MANIFEST=$(ADMIN_MANIFEST)
	go tool github.com/josephspurrier/goversioninfo/cmd/goversioninfo -o $(SYSO) $(VERSIONINFO)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(WINDOWS_BINARY_NAME)
	@echo "Build complete: $(WINDOWS_BINARY_NAME), $(WINDOWS_PINGIT)"

# Build linux/mips binary for LEDE/OpenWrt routers: no cgo (pure Go ping),
# softfloat for mips cores without an FPU, trimmed paths and stripped
mips:
	CGO_ENABLED=0 GOOS=linux GOARCH=mips GOMIPS=softfloat go build -trimpath -ldflags="-s -w" -o $(BINARY_NAME)
	@echo "Build complete: $(BINARY_NAME) (linux/mips softfloat)"

# Build linux/armv7 binary for Raspberry Pi 2 (hardfloat ARMv7, 32-bit OS)
pi2:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags="-s -w" -o $(BINARY_NAME)-armv7
	@echo "Build complete: $(BINARY_NAME)-armv7 (linux/armv7)"

# Build linux/arm64 binary for Raspberry Pi 3/4/5 on a 64-bit OS
pi3:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o $(BINARY_NAME)-arm64
	@echo "Build complete: $(BINARY_NAME)-arm64 (linux/arm64)"

# Clean compiled files
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME) $(BINARY_NAME)-armv7 $(BINARY_NAME)-arm64 pingit $(WINDOWS_BINARY_NAME) $(WINDOWS_PINGIT) $(ICO) $(VERSIONINFO) $(SYSO) $(PINGIT_SYSO)
	@echo "Clean complete"