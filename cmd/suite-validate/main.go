package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func main() {
	if err := validateSuiteDir("docs/schemas/suite-schema.json", "testdata"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// validateSuiteDir validates every JSON test suite in testdataDir against the
// suite schema in schemaPath.
func validateSuiteDir(schemaPath, testdataDir string) error {
	paths, err := filepath.Glob(filepath.Join(testdataDir, "*.json"))
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("%s: no JSON suite files found", testdataDir)
	}
	return validateSuiteFiles(schemaPath, paths)
}

// validateSuiteFiles validates JSON test suites against the suite schema.
func validateSuiteFiles(schemaPath string, suitePaths []string) error {
	if len(suitePaths) == 0 {
		return errors.New("no JSON suite files provided")
	}

	abs, err := filepath.Abs(schemaPath)
	if err != nil {
		return err
	}
	c := jsonschema.NewCompiler()
	schema, err := c.Compile(abs)
	if err != nil {
		return err
	}

	var validationErrs suiteValidationErrors
	for _, path := range suitePaths {
		if err := validateSuiteFile(schema, path); err != nil {
			validationErrs = append(validationErrs, suiteValidationError{
				Path: path,
				Err:  err,
			})
		}
	}
	if len(validationErrs) > 0 {
		return validationErrs
	}
	return nil
}

func validateSuiteFile(schema *jsonschema.Schema, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		return err
	}
	return schema.Validate(doc)
}

type suiteValidationError struct {
	Path string
	Err  error
}

type suiteValidationErrors []suiteValidationError

func (e suiteValidationErrors) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d suite validation error(s):", len(e))
	for _, suiteErr := range e {
		fmt.Fprintf(&b, "\n%s: %v", suiteErr.Path, suiteErr.Err)
	}
	return b.String()
}
