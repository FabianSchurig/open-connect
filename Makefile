# Open-Connect — root Makefile (Epic A).
#
# Targets:
#   make bootstrap   verify all required toolchains are installed
#   make proto       regenerate Go (and Rust at build-time) bindings via buf
#   make test        run all hermetic tests (Go + Rust)
#   make lint        run all linters (go vet, clippy -D warnings, FR-30 lint)
#   make build       build all binaries
#   make fr30-lint   run FR-30 conventions check on device-profile scripts
#
# The CI workflow shells out to these targets so behaviour is identical
# locally and in CI (Epic C).

GO ?= go
CARGO ?= cargo
BUF ?= buf

DEVICE_PROFILE_SCRIPTS := $(wildcard device-profiles/*/*.sh)

.PHONY: bootstrap proto test lint build fr30-lint go-test go-lint go-build rust-test rust-lint rust-build clean

bootstrap:
	@command -v $(GO)    >/dev/null || { echo "missing: go";    exit 1; }
	@command -v $(CARGO) >/dev/null || { echo "missing: cargo"; exit 1; }
	@command -v $(BUF)   >/dev/null || { echo "missing: buf";   exit 1; }
	@echo "toolchains OK:"
	@$(GO) version
	@$(CARGO) --version
	@$(BUF) --version

proto:
	$(BUF) generate

test: go-test rust-test

go-test:
	$(GO) test -race -cover ./...

rust-test:
	$(CARGO) test --workspace --all-targets

lint: go-lint rust-lint fr30-lint

go-lint:
	$(GO) vet ./...

rust-lint:
	$(CARGO) fmt --all -- --check
	$(CARGO) clippy --workspace --all-targets -- -D warnings

fr30-lint:
	@if [ -n "$(DEVICE_PROFILE_SCRIPTS)" ]; then \
		bash scripts/lint-device-profile-scripts.sh $(DEVICE_PROFILE_SCRIPTS); \
	else \
		echo "no device-profile scripts to lint"; \
	fi

build: go-build rust-build

go-build:
	$(GO) build ./...

rust-build:
	$(CARGO) build --workspace --release

clean:
	$(GO) clean ./...
	$(CARGO) clean
