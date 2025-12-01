.PHONY: all build test clean runner mock-handler

BUILD_DIR := build
RUNNER_BIN := $(BUILD_DIR)/runner
MOCK_HANDLER_BIN := $(BUILD_DIR)/mock-handler

all: build test

build: runner mock-handler

runner:
	@echo "Building test runner..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(RUNNER_BIN) ./cmd/runner

mock-handler:
	@echo "Building mock handler..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(MOCK_HANDLER_BIN) ./cmd/mock-handler

test:
	@echo "Running runner unit tests..."
	go test -v ./runner/...
	@echo "Running conformance tests with mock handler..."
	$(RUNNER_BIN) -handler $(MOCK_HANDLER_BIN)

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
