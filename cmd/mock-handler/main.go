package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"

	"github.com/stringintech/kernel-bindings-tests/runner"
	"github.com/stringintech/kernel-bindings-tests/testdata"
)

func main() {
	// Build a map of test ID -> filename
	testIndex, err := buildTestIndex()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build test index: %v\n", err)
		os.Exit(1)
	}

	// Read requests from stdin and respond with expected results
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if err := handleRequest(line, testIndex); err != nil {
			fmt.Fprintf(os.Stderr, "Error handling request: %v\n", err)
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
		os.Exit(1)
	}
}

// buildTestIndex creates a map of test ID -> filename without loading full test data
func buildTestIndex() (map[string]string, error) {
	testFiles, err := fs.Glob(testdata.FS, "*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to find test files: %w", err)
	}

	index := make(map[string]string)
	for _, testFile := range testFiles {
		// Read file to extract test IDs only
		data, err := fs.ReadFile(testdata.FS, testFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read test file %s: %w", testFile, err)
		}

		// Parse just enough to get test IDs
		var suite struct {
			Tests []struct {
				ID string `json:"id"`
			} `json:"tests"`
		}
		if err := json.Unmarshal(data, &suite); err != nil {
			return nil, fmt.Errorf("failed to parse test file %s: %w", testFile, err)
		}

		for _, test := range suite.Tests {
			index[test.ID] = testFile
		}
	}

	return index, nil
}

// handleRequest processes a single request and outputs the expected response
func handleRequest(line string, testIndex map[string]string) error {
	// Parse request
	var req runner.Request
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return fmt.Errorf("failed to parse request: %w", err)
	}

	filename, ok := testIndex[req.ID]
	if !ok {
		resp := runner.Response{
			ID:      req.ID,
			Success: false,
			Error: &runner.Error{
				Code: runner.ErrorCode{
					Type:   "Handler",
					Member: "UNKNOWN_TEST",
				},
			},
		}
		return writeResponse(resp)
	}

	// Load the test suite containing this test case
	suite, err := runner.LoadTestSuiteFromFS(testdata.FS, filename)
	if err != nil {
		resp := runner.Response{
			ID:      req.ID,
			Success: false,
			Error: &runner.Error{
				Code: runner.ErrorCode{
					Type:   "Handler",
					Member: "LOAD_ERROR",
				},
			},
		}
		return writeResponse(resp)
	}

	// Find the specific test case
	var testCase *runner.TestCase
	for _, test := range suite.Tests {
		if test.ID == req.ID {
			testCase = &test
			break
		}
	}
	if testCase == nil {
		resp := runner.Response{
			ID:      req.ID,
			Success: false,
			Error: &runner.Error{
				Code: runner.ErrorCode{
					Type:   "Handler",
					Member: "TEST_NOT_FOUND",
				},
			},
		}
		return writeResponse(resp)
	}

	// Verify method matches
	if req.Method != testCase.Method {
		resp := runner.Response{
			ID:      req.ID,
			Success: false,
			Error: &runner.Error{
				Code: runner.ErrorCode{
					Type:   "Handler",
					Member: "METHOD_MISMATCH",
				},
			},
		}
		return writeResponse(resp)
	}

	// Build response based on expected result
	return writeResponse(runner.Response{
		ID:      req.ID,
		Success: testCase.Expected.Success,
		Error:   testCase.Expected.Error,
	})
}

// writeResponse writes a response to stdout as JSON
func writeResponse(resp runner.Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
