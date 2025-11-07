package main

import (
	"encoding/json"
)

// TestCase represents a single test case
type TestCase struct {
	ID       string          `json:"id"`
	Method   string          `json:"method"`
	Params   json.RawMessage `json:"params"`
	Expected TestExpectation `json:"expected"`
}

// TestExpectation defines what response is expected
type TestExpectation struct {
	Success *json.RawMessage `json:"success,omitempty"` // Expected successful result
	Error   *ErrorPattern    `json:"error,omitempty"`   // Expected error
}

// ErrorPattern defines expected error fields
type ErrorPattern struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"` // Optional - if empty, any message is accepted
}

// TestSuite represents a collection of test cases
type TestSuite struct {
	Name  string     `json:"name"`
	Tests []TestCase `json:"tests"`
}

// Request represents a JSON-RPC style request
type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// Response represents a response from the handler
type Response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *ErrorObj       `json:"error,omitempty"`
}

// ErrorObj represents an error response
type ErrorObj struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
