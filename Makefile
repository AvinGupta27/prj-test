# ──────────────────────────────────────────────────────────────────────────────
# code-go-automation — Test Runner
#
# Usage:
#   make <target>
#
# Examples:
#   make reveal
#   make buy-reveal
#   make supply
#   make all
# ──────────────────────────────────────────────────────────────────────────────

.PHONY: all reveal buy-reveal supply create-pack build tidy help clean-reports

# Default: print help
.DEFAULT_GOAL := help

# ── TEST TARGETS ──────────────────────────────────────────────────────────────

## reveal: Reveal all unrevealed packs for every user in users.json
reveal:
	go test ./tests/pack/... -v -run TestPackSuite/TestReveal -count=1

## buy-reveal: Buy packs then reveal them for every user in users.json
buy-reveal:
	go test ./tests/pack/... -v -run TestPackSuite/TestBuyAndReveal -count=1

packs:
	go test ./tests/pack/... -v -run TestPackSuite -count=1

## supply: Assert supply integrity — total allocations must not exceed maxSupply
supply:
	go test ./tests/supply/... -v -run TestSupplySuite/TestSupplyIntegrity -count=1

## create-pack: Create a randomised pack via the admin proxy API
create-pack:
	go test ./tests/pack/... -v -run TestPackSuite/TestCreatePack -count=1

clean-reports:
	rm -rf reports/*

## all: Run all test suites
all:
	go test ./tests/... -v -count=1

# ── DEV TARGETS ───────────────────────────────────────────────────────────────

## build: Compile the project (no tests)
build:
	go build ./...

## tidy: Tidy and verify go.mod / go.sum
tidy:
	go mod tidy
	go mod verify

# ── HELP ──────────────────────────────────────────────────────────────────────

## help: List all available targets
help:
	@echo ""
	@echo "  code-go-automation — available targets:"
	@echo ""
	@grep -E '^## ' Makefile | sed 's/## /    make /' | column -t -s ':'
	@echo ""
