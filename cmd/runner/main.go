package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/stringintech/kernel-bindings-tests/runner"
	"github.com/stringintech/kernel-bindings-tests/testdata"
)

func main() {
	handlerPath := pflag.String("handler", "", "Path to handler binary")
	handlerTimeout := pflag.Duration("handler-timeout", 10*time.Second, "Max time to wait for handler to respond to each test case (e.g., 10s, 500ms)")
	timeout := pflag.Duration("timeout", 30*time.Second, "Total timeout for executing all test suites (e.g., 30s, 1m)")
	verboseCount := pflag.CountP("verbose", "v", "Verbose mode: -v shows all requests needed to reproduce failed tests, plus received/expected responses; -vv shows this for all tests (passed and failed)")
	pflag.Parse()

	// Convert verbose count to verbosity level
	verbosity := runner.VerbosityQuiet
	if *verboseCount >= 2 {
		verbosity = runner.VerbosityAlways
	} else if *verboseCount == 1 {
		verbosity = runner.VerbosityOnFailure
	}

	if *handlerPath == "" {
		fmt.Fprintf(os.Stderr, "Error: --handler flag is required\n")
		pflag.Usage()
		os.Exit(1)
	}

	// Collect embedded test files
	testFiles, err := fs.Glob(testdata.FS, "*.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding test files: %v\n", err)
		os.Exit(1)
	}

	if len(testFiles) == 0 {
		fmt.Fprintf(os.Stderr, "No test files found\n")
		os.Exit(1)
	}

	// Sort test files alphabetically for deterministic execution order
	sort.Strings(testFiles)

	// Create test runner
	testRunner, err := runner.NewTestRunner(*handlerPath, *handlerTimeout, *timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating test runner: %v\n", err)
		os.Exit(1)
	}
	defer testRunner.CloseHandler()

	// Create context with total execution timeout
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Run tests
	totalPassed := 0
	totalFailed := 0
	totalTests := 0

	for _, testFile := range testFiles {
		fmt.Printf("\n=== Running test suite: %s ===\n", testFile)

		// Load test suite from embedded FS
		suite, err := runner.LoadTestSuiteFromFS(testdata.FS, testFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading test suite: %v\n", err)
			continue
		}

		// Run suite
		result := testRunner.RunTestSuite(ctx, *suite, verbosity)
		printResults(suite, result)

		totalPassed += result.PassedTests
		totalFailed += result.FailedTests
		totalTests += result.TotalTests

		// Close handler after stateful suites to prevent state leaks.
		// A new handler process will be spawned on-demand when the next request is sent.
		if suite.Stateful {
			testRunner.CloseHandler()
		}
	}

	fmt.Printf("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Printf("TOTAL SUMMARY\n")
	fmt.Printf(strings.Repeat("=", 60) + "\n")
	fmt.Printf("Total Tests: %d\n", totalTests)
	fmt.Printf("Passed:      %d\n", totalPassed)
	fmt.Printf("Failed:      %d\n", totalFailed)
	fmt.Printf(strings.Repeat("=", 60) + "\n")

	if totalFailed > 0 {
		os.Exit(1)
	}
}

func printResults(suite *runner.TestSuite, result runner.TestResult) {
	fmt.Printf("\nTest Suite: %s\n", result.SuiteName)
	if suite.Description != "" {
		fmt.Printf("Description: %s\n", suite.Description)
	}
	fmt.Printf("Total: %d, Passed: %d, Failed: %d\n\n", result.TotalTests, result.PassedTests, result.FailedTests)

	for i, tr := range result.TestResults {
		status := "✓"
		if !tr.Passed {
			status = "✗"
		}

		// Print test ID and description if available
		if suite.Tests[i].Description != "" {
			fmt.Printf("  %s %s (%s)\n", status, tr.TestID, suite.Tests[i].Description)
		} else {
			fmt.Printf("  %s %s\n", status, tr.TestID)
		}

		// Print message indented
		fmt.Printf("      %s\n", tr.Message)
	}

	fmt.Printf("\n")
}
