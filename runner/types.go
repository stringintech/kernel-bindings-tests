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

// TestExpectation defines what response is expected.
// If expecting success, result contains the expected value (or null for void/nullptr) and error must be null.
// If expecting failure, result must be null and error contains the expected error details.
type TestExpectation struct {
	Result Result `json:"result"`          // Expected return value (null for void/nullptr/error cases)
	Error  *Error `json:"error,omitempty"` // Expected error (null for success cases)
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

// Response represents a response from the handler.
// If the operation succeeds, result contains the return value (or null for void/nullptr) and error must be null.
// If the operation fails, result must be null and error contains error details.
type Response struct {
	ID     string `json:"id"`
	Result Result `json:"result"`          // Return value (null for void/nullptr/error cases)
	Error  *Error `json:"error,omitempty"` // Error details (null for success cases)
}

// Error represents an error response.
// Code can be null for generic errors without specific error codes.
type Error struct {
	Code *ErrorCode `json:"code,omitempty"`
}

type ErrorCode struct {
	Type   string `json:"type"`   // e.g., "btck_ScriptVerifyStatus"
	Member string `json:"member"` // e.g., "ERROR_INVALID_FLAGS_COMBINATION"
}

// Result is a type alias for json.RawMessage with helper methods.
type Result json.RawMessage

// MarshalJSON implements json.Marshaler by delegating to json.RawMessage.
func (r Result) MarshalJSON() ([]byte, error) {
	return (json.RawMessage)(r).MarshalJSON()
}

// UnmarshalJSON implements json.Unmarshaler by delegating to json.RawMessage.
func (r *Result) UnmarshalJSON(data []byte) error {
	return (*json.RawMessage)(r).UnmarshalJSON(data)
}

// IsNullOrOmitted checks if the result is nil or represents a JSON null value.
func (r Result) IsNullOrOmitted() bool {
	return r == nil || string(r) == "null"
}

// Normalize normalizes JSON data by parsing and re-marshaling it.
// This ensures consistent formatting and key ordering for comparison.
func (r Result) Normalize() (string, error) {
	var v interface{}
	if err := json.Unmarshal(r, &v); err != nil {
		return "", err
	}
	normalized, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(normalized), nil
}
