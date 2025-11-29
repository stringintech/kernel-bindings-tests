package runner

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

// TestRunner executes test suites against a handler binary
type TestRunner struct {
	handlerPath string
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stdout      *bufio.Scanner
	stderr      io.ReadCloser
}

// NewTestRunner creates a new test runner
func NewTestRunner(handlerPath string) (*TestRunner, error) {
	if _, err := os.Stat(handlerPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("handler binary not found: %s", handlerPath)
	}

	cmd := exec.Command(handlerPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start handler: %w", err)
	}

	return &TestRunner{
		handlerPath: handlerPath,
		cmd:         cmd,
		stdin:       stdin,
		stdout:      bufio.NewScanner(stdout),
		stderr:      stderr,
	}, nil
}

// Close terminates the handler process
func (tr *TestRunner) Close() error {
	if tr.stdin != nil {
		tr.stdin.Close()
	}
	if tr.cmd != nil {
		return tr.cmd.Wait()
	}
	return nil
}

// SendRequest sends a request and reads the response
func (tr *TestRunner) SendRequest(req Request) (*Response, error) {
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	if _, err := tr.stdin.Write(append(reqData, '\n')); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response
	if !tr.stdout.Scan() {
		if err := tr.stdout.Err(); err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		// Handler closed stdout prematurely.
		// Kill the process immediately to force stderr to close.
		// Without this, there's a rare scenario where stdout closes but stderr remains open,
		// causing io.ReadAll(tr.stderr) below to block indefinitely waiting for stderr EOF.
		if tr.cmd.Process != nil {
			tr.cmd.Process.Kill()
		}
		if stderrOut, err := io.ReadAll(tr.stderr); err == nil && len(stderrOut) > 0 {
			return nil, fmt.Errorf("handler closed unexpectedly: %s", bytes.TrimSpace(stderrOut))
		}
		return nil, fmt.Errorf("handler closed unexpectedly")
	}
	respLine := tr.stdout.Text()
	var resp Response
	if err := json.Unmarshal([]byte(respLine), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

// RunTestSuite executes a test suite
func (tr *TestRunner) RunTestSuite(suite TestSuite) TestResult {
	result := TestResult{
		SuiteName:  suite.Name,
		TotalTests: len(suite.Tests),
	}

	for _, test := range suite.Tests {
		err := tr.runTest(test)
		testResult := SingleTestResult{
			TestID: test.ID,
			Passed: err == nil,
		}
		if err != nil {
			testResult.Message = err.Error()
		}

		result.TestResults = append(result.TestResults, testResult)
		if testResult.Passed {
			result.PassedTests++
		} else {
			result.FailedTests++
		}
	}

	return result
}

// runTest executes a single test case and validates the response.
// Returns an error if communication with the handler fails or validation fails.
func (tr *TestRunner) runTest(test TestCase) error {
	req := Request{
		ID:     test.ID,
		Method: test.Method,
		Params: test.Params,
	}

	resp, err := tr.SendRequest(req)
	if err != nil {
		return err
	}

	return validateResponse(test, resp)
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
