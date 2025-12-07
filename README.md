# Bitcoin Kernel Binding Conformance Tests

This repository contains a language-agnostic conformance testing framework for Bitcoin Kernel bindings.

## ⚠️ Work in Progress

## Overview

The framework ensures that all language bindings (Go, Python, Rust, etc.) behave identically by:
- Defining a standard JSON protocol for testing
- Providing shared test cases that work across all bindings
- Enforcing consistent error handling and categorization

## Architecture

```
┌─────────────┐         ┌───────────────────┐
│ Test Runner │────────▶│  Handler Binary** │
│  (Go CLI)   │ stdin   │  (Go/Rust/etc)    │
│             │◀────────│                   │
└─────────────┘ stdout  └───────────────────┘
       │                         │
       │                         │
       ▼                         ▼
  ┌─────────┐            ┌────────────────┐
  │ Test    │            │ Binding API    │
  │ Cases   │            └────────────────┘
  │ (JSON)  │
  └─────────┘
```

**This repository contains:**
1. [**Handler Specification**](./docs/handler-spec.md): Defines the protocol, message formats, and test suites that handlers must implement
2. [**Test Runner**](./cmd/runner/main.go): Spawns handler binary, sends test requests via stdin, validates responses from stdout
3. [**Test Cases**](./testdata): JSON files defining requests and expected responses
4. [**Mock Handler**](./cmd/mock-handler/main.go): Validates the runner by echoing expected responses from test cases

** **Handler binaries** are not hosted in this repository. They must be implemented separately following the [**Handler Specification**](./docs/handler-spec.md) and should:
- Implement the JSON protocol for communication with the test runner
- Call the binding API to execute operations
- Pin to a specific version/tag of this test repository

## Getting Started

### Testing Your Binding (Custom Handler)

Test your handler implementation using the test runner:

```bash
# Build the test runner
make runner

# Run the test runner against your handler binary
./build/runner --handler <path-to-your-handler>

# Configure timeouts (optional)
./build/runner --handler <path-to-your-handler> \
  --handler-timeout 30s \  # Max wait per test case (default: 10s)
  --timeout 2m             # Total execution limit (default: 30s)
```

#### Timeout Flags

- **`--handler-timeout`** (default: 10s): Maximum time to wait for handler response to each test case. Prevents hangs on unresponsive handlers.
- **`--timeout`** (default: 30s): Total execution time limit across all test suites. Ensures bounded test runs.

The runner automatically detects and recovers from crashed/unresponsive handlers, allowing remaining tests to continue.

#### Verbose Flags

- **`-v, --verbose`**: Shows request chains and responses for **failed tests only**
- **`-vv`**: Shows request chains and responses for **all tests** (passed and failed)

The request chains printed by verbose mode can be directly piped to the handler binary for manual debugging:

```bash
# Example output from -vv mode:
# ✓ chain#4 (Get active chain reference from chainstate manager)
#
#       Request chain
#       ────────────────────────────────────────
# {"id":"chain#1","method":"btck_context_create","params":{"chain_parameters":{"chain_type":"btck_ChainType_REGTEST"}},"ref":"$context_ref"}
# {"id":"chain#2","method":"btck_chainstate_manager_create","params":{"context":"$context_ref"},"ref":"$chainstate_manager_ref"}
# {"id":"chain#4","method":"btck_chainstate_manager_get_active_chain","params":{"chainstate_manager":"$chainstate_manager_ref"},"ref":"$chain_ref"}
#
#       Response:
#       ────────────────────────────────────────
#       {"result":"$chain_ref"}

# Copy the request chain and pipe it to your handler for debugging:
echo '{"id":"chain#1","method":"btck_context_create","params":{"chain_parameters":{"chain_type":"btck_ChainType_REGTEST"}},"ref":"$context_ref"}
{"id":"chain#2","method":"btck_chainstate_manager_create","params":{"context":"$context_ref"},"ref":"$chainstate_manager_ref"}
{"id":"chain#4","method":"btck_chainstate_manager_get_active_chain","params":{"chainstate_manager":"$chainstate_manager_ref"},"ref":"$chain_ref"}' | ./path/to/your/handler
```

This is particularly useful for:
- Reproducing test failures independently
- Debugging handler behavior without running the full test suite
- Testing individual operations in isolation

### Testing the Runner

Build and test the runner:

```bash
# Build both runner and mock handler
make build

# Run runner unit tests and integration tests with mock handler
make test
```
