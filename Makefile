# Makefile for pinguin

.PHONY: install win ico syso versioninfo clean help

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

# Default target
help:
	@echo "Available targets:"
	@echo "  make install  - install binary for current platform"
	@echo "  make win      - build Windows executable (pinguin.exe) at latest tag"
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
		'  "IconPath": "$(ICO)"' \
		'}' > $(VERSIONINFO)

# Generate Windows resource file (icon + version info)
$(SYSO): versioninfo $(ICO)
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

# Build Windows executable
win: $(SYSO)
	@echo "Building for Windows (amd64)..."
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(WINDOWS_BINARY_NAME)
	@echo "Build complete: $(WINDOWS_BINARY_NAME)"

# Clean compiled files
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME) $(WINDOWS_BINARY_NAME) $(ICO) $(VERSIONINFO) $(SYSO)
	@echo "Clean complete"