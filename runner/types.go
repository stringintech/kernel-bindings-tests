package runner

import (
	"encoding/json"
)

// TestCase represents a single test case
type TestCase struct {
	ID          string          `json:"id"`
	Description string          `json:"description,omitempty"`
	Method      string          `json:"method"`
	Params      json.RawMessage `json:"params"`
	Expected    TestExpectation `json:"expected"`
}

// TestExpectation defines what response is expected
type TestExpectation struct {
	Success bool   `json:"success"`         // Whether operation should succeed
	Error   *Error `json:"error,omitempty"` // Expected error details
}

// TestSuite represents a collection of test cases
type TestSuite struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Tests       []TestCase `json:"tests"`
}

// Request represents a request sent to the handler
type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// Response represents a response from the handler
type Response struct {
	ID      string `json:"id"`
	Success bool   `json:"success"`         // Whether operation succeeded
	Error   *Error `json:"error,omitempty"` // Error details (if success=false)
}

// Error represents an error response
type Error struct {
	Code ErrorCode `json:"code"`
}

type ErrorCode struct {
	Type   string `json:"type"`   // e.g., "btck_ScriptVerifyStatus"
	Member string `json:"member"` // e.g., "ERROR_INVALID_FLAGS_COMBINATION"
}
