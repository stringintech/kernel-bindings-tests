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
```

### Testing the Runner

Build and test the runner:

```bash
# Build both runner and mock handler
make build

# Run runner unit tests and integration tests with mock handler
make test
```
