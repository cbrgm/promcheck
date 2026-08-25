SHELL := bash
NAME := promcheck
IMPORT := github.com/cbrgm/promcheck
BIN := bin
DIST := dist

ifeq ($(OS), Windows_NT)
	EXECUTABLE := $(NAME).exe
	UNAME := Windows
else
	EXECUTABLE := $(NAME)
	UNAME := $(shell uname -s)
endif

GOBUILD ?= CGO_ENABLED=0 go build
PACKAGES ?= $(shell go list ./...)
SOURCES ?= $(shell find . -name "*.go" -type f)
GENERATE ?= $(PACKAGES)

TAGS ?=

ifndef OUTPUT
	ifeq ($(GITHUB_REF_TYPE), tag)
		OUTPUT ?= $(patsubst v%,%,$(GITHUB_REF_NAME))
	else
		OUTPUT ?= testing
	endif
endif

ifndef VERSION
	ifeq ($(GITHUB_REF_TYPE), tag)
		VERSION ?= $(patsubst v%,%,$(GITHUB_REF_NAME))
	else
		VERSION ?= $(patsubst v%,%,$(shell git describe --tags --dirty 2>/dev/null || echo dev))
	endif
endif

ifndef DATE
	DATE := $(shell date -u '+%Y%m%d')
endif

ifndef SHA
	SHA := $(shell git rev-parse --short HEAD)
endif

LDFLAGS += -s -w -extldflags "-static" -X "main.Version=$(VERSION)" -X "main.Revision=$(SHA)" -X "main.BuildDate=$(DATE)"
GCFLAGS += all=-N -l

.PHONY: all
all: build

.PHONY: sync
sync:
	go mod download

.PHONY: clean
clean:
	go clean -i ./...
	rm -rf $(BIN) $(DIST)

.PHONY: fmt
fmt:
	gofmt -s -w $(SOURCES)

.PHONY: vet
vet:
	go vet $(PACKAGES)

.PHONY: lint
lint:
	golangci-lint run --output.text.colors=false --output.tab.colors=false  --timeout 5m

.PHONY: generate
generate:
	go generate $(GENERATE)

.PHONY: test
test:
	go test -coverprofile coverage.out $(PACKAGES)

# Prints the release notes for the most recent tag, using the same invocation
# the go-binaries workflow publishes with. Run `make changelog CLIFF_ARGS=--unreleased`
# to preview the notes for the commits since that tag instead.
CLIFF_ARGS ?= --latest

# git-cliff queries the GitHub API to resolve pull request links and first-time
# contributors, and aborts outright once the unauthenticated quota (60 requests
# per hour) runs out. The workflow supplies a token of its own, so this only
# affects local runs: prefer an exported GITHUB_TOKEN, otherwise borrow the one
# the gh CLI already holds.
.PHONY: changelog
changelog:
	@command -v git-cliff >/dev/null 2>&1 || { echo "git-cliff not installed: https://git-cliff.org/docs/installation" >&2; exit 1; }
	@token="$${GITHUB_TOKEN:-$$(gh auth token 2>/dev/null)}"; \
	if [ -n "$$token" ]; then \
		export GITHUB_TOKEN="$$token"; \
	else \
		echo "warning: no GITHUB_TOKEN and no gh login; the unauthenticated rate limit (60/hour) may abort this run" >&2; \
		unset GITHUB_TOKEN; \
	fi; \
	git-cliff --config cliff.toml $(CLIFF_ARGS) --strip all

.PHONY: install
install: $(SOURCES)
	go install -v -tags '$(TAGS)' -ldflags '$(LDFLAGS)' ./cmd/$(NAME)

.PHONY: build
build: $(BIN)/$(EXECUTABLE)

$(BIN)/$(EXECUTABLE): $(SOURCES)
	$(GOBUILD) -v -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $@ ./cmd/$(NAME)

$(BIN)/$(EXECUTABLE)-debug: $(SOURCES)
	$(GOBUILD) -v -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -gcflags '$(GCFLAGS)' -o $@ ./cmd/$(NAME)

.PHONY: release
release: $(DIST) release-linux release-darwin release-windows release-checksum

.PHONY: release-checksum
release-checksum:
	cd $(DIST); $(foreach file,$(wildcard $(DIST)/$(EXECUTABLE)_*),sha256sum $(notdir $(file)) > $(notdir $(file)).sha256;)

$(DIST):
	mkdir -p $(DIST)

.PHONY: release-linux
release-linux: $(DIST) \
	$(DIST)/$(EXECUTABLE)_linux-386 \
	$(DIST)/$(EXECUTABLE)_linux-amd64 \
	$(DIST)/$(EXECUTABLE)_linux-arm-5 \
	$(DIST)/$(EXECUTABLE)_linux-arm-6 \
	$(DIST)/$(EXECUTABLE)_linux-arm-7 \
	$(DIST)/$(EXECUTABLE)_linux-arm64

$(DIST)/$(EXECUTABLE)_linux-386:
	GOOS=linux GOARCH=386 $(GOBUILD) -v -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $@ ./cmd/$(NAME)

$(DIST)/$(EXECUTABLE)_linux-amd64:
	GOOS=linux GOARCH=amd64 $(GOBUILD) -v -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $@ ./cmd/$(NAME)

$(DIST)/$(EXECUTABLE)_linux-arm-5:
	GOOS=linux GOARCH=arm GOARM=5 $(GOBUILD) -v -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $@ ./cmd/$(NAME)

$(DIST)/$(EXECUTABLE)_linux-arm-6:
	GOOS=linux GOARCH=arm GOARM=6 $(GOBUILD) -v -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $@ ./cmd/$(NAME)

$(DIST)/$(EXECUTABLE)_linux-arm-7:
	GOOS=linux GOARCH=arm GOARM=7 $(GOBUILD) -v -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $@ ./cmd/$(NAME)

$(DIST)/$(EXECUTABLE)_linux-arm64:
	GOOS=linux GOARCH=arm64 $(GOBUILD) -v -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $@ ./cmd/$(NAME)


.PHONY: release-darwin
release-darwin: $(DIST) \
	$(DIST)/$(EXECUTABLE)_darwin-amd64 \
	$(DIST)/$(EXECUTABLE)_darwin-arm64

$(DIST)/$(EXECUTABLE)_darwin-amd64:
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -v -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $@ ./cmd/$(NAME)

$(DIST)/$(EXECUTABLE)_darwin-arm64:
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -v -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $@ ./cmd/$(NAME)

.PHONY: release-windows
release-windows: $(DIST) \
	$(DIST)/$(EXECUTABLE)_windows-4.0-386.exe \
	$(DIST)/$(EXECUTABLE)_windows-4.0-amd64.exe

$(DIST)/$(EXECUTABLE)_windows-4.0-386.exe:
	GOOS=windows GOARCH=386 $(GOBUILD) -v -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $@ ./cmd/$(NAME)

$(DIST)/$(EXECUTABLE)_windows-4.0-amd64.exe:
	GOOS=windows GOARCH=amd64 $(GOBUILD) -v -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $@ ./cmd/$(NAME)

