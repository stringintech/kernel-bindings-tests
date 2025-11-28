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

// runTest executes a single test case
func (tr *TestRunner) runTest(test TestCase) SingleTestResult {
	req := Request{
		ID:     test.ID,
		Method: test.Method,
		Params: test.Params,
	}

	resp, err := tr.SendRequest(req)
	if err != nil {
		return SingleTestResult{
			TestID:  test.ID,
			Passed:  false,
			Message: fmt.Sprintf("Failed to send request: %v", err),
		}
	}
	if resp.ID != test.ID {
		return SingleTestResult{
			TestID:  test.ID,
			Passed:  false,
			Message: fmt.Sprintf("Response ID mismatch: expected %s, got %s", test.ID, resp.ID),
		}
	}
	return validateResponse(test, resp)
}

// validateResponse checks if response matches expected result
func validateResponse(test TestCase, resp *Response) SingleTestResult {
	// Check if we expected an error
	if test.Expected.Error != nil {
		if resp.Error == nil {
			return SingleTestResult{
				TestID:  test.ID,
				Passed:  false,
				Message: fmt.Sprintf("Expected error type %s, but got no error", test.Expected.Error.Type),
			}
		}

		// Check error type
		if resp.Error.Type != test.Expected.Error.Type {
			return SingleTestResult{
				TestID:  test.ID,
				Passed:  false,
				Message: fmt.Sprintf("Expected error type %s, got %s", test.Expected.Error.Type, resp.Error.Type),
			}
		}

		// Check error variant if specified
		if test.Expected.Error.Variant != "" && resp.Error.Variant != test.Expected.Error.Variant {
			return SingleTestResult{
				TestID:  test.ID,
				Passed:  false,
				Message: fmt.Sprintf("Expected error variant %s, got %s", test.Expected.Error.Variant, resp.Error.Variant),
			}
		}

		return SingleTestResult{
			TestID:  test.ID,
			Passed:  true,
			Message: "Test passed (expected error matched)",
		}
	}

	// Check if we expected success
	if test.Expected.Success != nil {
		if resp.Error != nil {
			errMsg := fmt.Sprintf("Expected success, but got error: %s", resp.Error.Type)
			if resp.Error.Variant != "" {
				errMsg += fmt.Sprintf(" (variant: %s)", resp.Error.Variant)
			}
			return SingleTestResult{
				TestID:  test.ID,
				Passed:  false,
				Message: errMsg,
			}
		}

		if resp.Success == nil {
			return SingleTestResult{
				TestID:  test.ID,
				Passed:  false,
				Message: "Expected success, but response contained no success field",
			}
		}

		// Normalize JSON for comparison
		var expectedData, actualData interface{}
		if err := json.Unmarshal(bytes.TrimSpace(*test.Expected.Success), &expectedData); err != nil {
			return SingleTestResult{
				TestID:  test.ID,
				Passed:  false,
				Message: fmt.Sprintf("Invalid expected JSON: %v", err),
			}
		}
		if err := json.Unmarshal(bytes.TrimSpace(*resp.Success), &actualData); err != nil {
			return SingleTestResult{
				TestID:  test.ID,
				Passed:  false,
				Message: fmt.Sprintf("Invalid response JSON: %v", err),
			}
		}
		expectedNormalized, _ := json.Marshal(expectedData)
		actualNormalized, _ := json.Marshal(actualData)

		if !bytes.Equal(expectedNormalized, actualNormalized) {
			return SingleTestResult{
				TestID:  test.ID,
				Passed:  false,
				Message: fmt.Sprintf("Success mismatch:\nExpected: %s\nActual:   %s", string(expectedNormalized), string(actualNormalized)),
			}
		}
		return SingleTestResult{
			TestID:  test.ID,
			Passed:  true,
			Message: "Test passed",
		}
	}
	return SingleTestResult{
		TestID:  test.ID,
		Passed:  false,
		Message: "Test has no expected result defined",
	}
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
