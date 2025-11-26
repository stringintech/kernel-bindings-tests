package runner

import (
	"bufio"
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
	// Check success flag matches
	if test.Expected.Success != resp.Success {
		if test.Expected.Success {
			errMsg := "Expected success=true, but got success=false"
			if resp.Error != nil {
				errMsg += fmt.Sprintf(" with error: %s.%s", resp.Error.Code.Type, resp.Error.Code.Member)
			}
			return SingleTestResult{
				TestID:  test.ID,
				Passed:  false,
				Message: errMsg,
			}
		}
		return SingleTestResult{
			TestID:  test.ID,
			Passed:  false,
			Message: "Expected success=false, but got success=true",
		}
	}

	// If we expected an error with specific code, validate it
	if test.Expected.Error != nil {
		if resp.Error == nil {
			return SingleTestResult{
				TestID:  test.ID,
				Passed:  false,
				Message: fmt.Sprintf("Expected error %s.%s, but got no error", test.Expected.Error.Code.Type, test.Expected.Error.Code.Member),
			}
		}

		// Check error code type
		if resp.Error.Code.Type != test.Expected.Error.Code.Type {
			return SingleTestResult{
				TestID:  test.ID,
				Passed:  false,
				Message: fmt.Sprintf("Expected error type %s, got %s", test.Expected.Error.Code.Type, resp.Error.Code.Type),
			}
		}

		// Check error code member
		if resp.Error.Code.Member != test.Expected.Error.Code.Member {
			return SingleTestResult{
				TestID:  test.ID,
				Passed:  false,
				Message: fmt.Sprintf("Expected error member %s, got %s", test.Expected.Error.Code.Member, resp.Error.Code.Member),
			}
		}

		return SingleTestResult{
			TestID:  test.ID,
			Passed:  true,
			Message: "Test passed (expected error matched)",
		}
	}

	// Success case with no specific error expected
	if test.Expected.Success {
		if resp.Error != nil {
			return SingleTestResult{
				TestID:  test.ID,
				Passed:  false,
				Message: fmt.Sprintf("Expected success with no error, but got error: %s.%s", resp.Error.Code.Type, resp.Error.Code.Member),
			}
		}
		return SingleTestResult{
			TestID:  test.ID,
			Passed:  true,
			Message: "Test passed",
		}
	}

	// Failure case with no specific error code (just success=false)
	return SingleTestResult{
		TestID:  test.ID,
		Passed:  true,
		Message: "Test passed (expected failure without specific error)",
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
