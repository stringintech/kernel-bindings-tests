package runner

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// TestRunner executes test suites against a handler binary
type TestRunner struct {
	handlerPath string
	handler     *Handler
}

// NewTestRunner creates a new test runner
func NewTestRunner(handlerPath string) (*TestRunner, error) {
	if _, err := os.Stat(handlerPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("handler binary not found: %s", handlerPath)
	}

	handler, err := NewHandler(HandlerConfig{Path: handlerPath})
	if err != nil {
		return nil, err
	}

	return &TestRunner{
		handlerPath: handlerPath,
		handler:     handler,
	}, nil
}

// SendRequest sends a request to the handler, spawning a new handler if needed
func (tr *TestRunner) SendRequest(req Request) error {
	if tr.handler == nil {
		handler, err := NewHandler(HandlerConfig{Path: tr.handlerPath})
		if err != nil {
			return fmt.Errorf("failed to spawn new handler: %w", err)
		}
		tr.handler = handler
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}
	if err := tr.handler.SendLine(reqData); err != nil {
		slog.Warn("Failed to write request, cleaning up handler (will spawn new one for remaining tests)", "error", err)
		tr.CloseHandler()
		return fmt.Errorf("failed to write request: %w", err)
	}
	return nil
}

// ReadResponse reads and unmarshals a response from the handler
func (tr *TestRunner) ReadResponse() (*Response, error) {
	line, err := tr.handler.ReadLine()
	if err != nil {
		slog.Warn("Failed to read response, cleaning up handler (will spawn new one for remaining tests)", "error", err)
		tr.CloseHandler()
		return nil, err
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CloseHandler closes the handler and sets it to nil
func (tr *TestRunner) CloseHandler() {
	if tr.handler == nil {
		return
	}
	tr.handler.Close()
	tr.handler = nil
}

// RunTestSuite executes a test suite
func (tr *TestRunner) RunTestSuite(suite TestSuite) TestResult {
	result := TestResult{
		SuiteName:  suite.Name,
		TotalTests: len(suite.Tests),
	}

	for _, test := range suite.Tests {
		testResult := tr.runTest(test)
		result.TestResults = append(result.TestResults, testResult)
		if testResult.Passed {
			result.PassedTests++
		} else {
			result.FailedTests++
		}
	}

	return result
}

// runTest executes a single test case by sending a request, reading the response,
// and validating the result matches expected output
func (tr *TestRunner) runTest(test TestCase) SingleTestResult {
	req := Request{
		ID:     test.ID,
		Method: test.Method,
		Params: test.Params,
	}

	err := tr.SendRequest(req)
	if err != nil {
		return SingleTestResult{
			TestID:  test.ID,
			Passed:  false,
			Message: fmt.Sprintf("Failed to send request: %v", err),
		}
	}

	resp, err := tr.ReadResponse()
	if err != nil {
		return SingleTestResult{
			TestID:  test.ID,
			Passed:  false,
			Message: fmt.Sprintf("Failed to read response: %v", err),
		}
	}

	if err := validateResponse(test, resp); err != nil {
		return SingleTestResult{
			TestID:  test.ID,
			Passed:  false,
			Message: fmt.Sprintf("Invalid response: %s", err.Error()),
		}
	}
	return SingleTestResult{
		TestID: test.ID,
		Passed: true,
	}
}

// validateResponse validates that a response matches the expected test outcome.
// Returns an error if:
//
//	Response ID does not match the request ID
//	Response does not match the expected outcome (error or success)
func validateResponse(test TestCase, resp *Response) error {
	if resp.ID != test.ID {
		return fmt.Errorf("response ID mismatch: expected %s, got %s", test.ID, resp.ID)
	}

	if test.Expected.Error != nil {
		return validateResponseForError(test, resp)
	}

	return validateResponseForSuccess(test, resp)
}

// validateResponseForError validates that a response correctly represents an error case.
// It ensures the response contains an error, the result is null or omitted, and if an
// error code is expected, it matches the expected type and member.
func validateResponseForError(test TestCase, resp *Response) error {
	if test.Expected.Error == nil {
		panic("validateResponseForError expects non-nil error")
	}

	if resp.Error == nil {
		if test.Expected.Error.Code != nil {
			return fmt.Errorf("expected error %s.%s, but got no error",
				test.Expected.Error.Code.Type, test.Expected.Error.Code.Member)
		}
		return fmt.Errorf("expected error, but got no error")
	}

	if !resp.Result.IsNullOrOmitted() {
		return fmt.Errorf("expected result to be null or omitted when error is present, got: %s", string(resp.Result))
	}

	if test.Expected.Error.Code != nil {
		if resp.Error.Code == nil {
			return fmt.Errorf("expected error code %s.%s, but got error with no code",
				test.Expected.Error.Code.Type, test.Expected.Error.Code.Member)
		}

		if resp.Error.Code.Type != test.Expected.Error.Code.Type {
			return fmt.Errorf("expected error type %s, got %s", test.Expected.Error.Code.Type, resp.Error.Code.Type)
		}

		if resp.Error.Code.Member != test.Expected.Error.Code.Member {
			return fmt.Errorf("expected error member %s, got %s", test.Expected.Error.Code.Member, resp.Error.Code.Member)
		}
	}
	return nil
}

// validateResponseForSuccess validates that a response correctly represents a success case.
// It ensures the response contains no error, and if a result is expected, it matches the
// expected value.
func validateResponseForSuccess(test TestCase, resp *Response) error {
	if test.Expected.Error != nil {
		panic("validateResponseForSuccess expects nil error")
	}

	if resp.Error != nil {
		if resp.Error.Code != nil {
			return fmt.Errorf("expected success with no error, but got error: %s.%s", resp.Error.Code.Type, resp.Error.Code.Member)
		}
		return fmt.Errorf("expected success with no error, but got error")
	}

	if test.Expected.Result.IsNullOrOmitted() {
		if !resp.Result.IsNullOrOmitted() {
			return fmt.Errorf("expected null or omitted result, got: %s", string(resp.Result))
		}
		return nil
	}

	expectedNorm, err := test.Expected.Result.Normalize()
	if err != nil {
		return fmt.Errorf("failed to normalize expected result: %w", err)
	}

	actualNorm, err := resp.Result.Normalize()
	if err != nil {
		return fmt.Errorf("failed to normalize actual result: %w", err)
	}

	if expectedNorm != actualNorm {
		return fmt.Errorf("result mismatch: expected %s, got %s", expectedNorm, actualNorm)
	}
	return nil
}

// TestResult contains results from running a test suite
type TestResult struct {
	SuiteName   string
	TotalTests  int
	PassedTests int
	FailedTests int
	TestResults []SingleTestResult
}

// SingleTestResult contains the result of a single test
type SingleTestResult struct {
	TestID  string
	Passed  bool
	Message string
}

// LoadTestSuiteFromFS loads a test suite from an embedded filesystem
func LoadTestSuiteFromFS(fsys embed.FS, filePath string) (*TestSuite, error) {
	data, err := fs.ReadFile(fsys, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var suite TestSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Set suite name from filename if not specified
	if suite.Name == "" {
		suite.Name = filepath.Base(filePath)
	}

	return &suite, nil
}
