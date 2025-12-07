package runner

import (
	"encoding/json"
	"fmt"
)

// stateMutatingMethods contains methods that mutate internal state.
// Tests using these methods are assumed to affect all subsequent tests
// and are included in dependency chains printed in verbose mode.
var stateMutatingMethods = map[string]bool{
	"btck_chainstate_manager_process_block": true,
}

// DependencyTracker manages test dependencies and builds request chains for verbose output.
// It tracks both explicit ref dependencies and implicit state dependencies.
type DependencyTracker struct {
	// refCreators maps reference names to the test index that created them
	refCreators map[string]int

	// statefulRefs tracks refs created by stateful methods (btck_context_create, btck_chainstate_manager_create).
	// Tests using these refs depend on mutable state.
	statefulRefs map[string]bool

	// depChains maps test index to its dependency chain (tests it depends on via ref usage)
	depChains map[int][]int

	// stateDependencies is a cumulative list of all tests affecting state (state-mutating
	// tests and their complete dependency chains)
	stateDependencies []int
}

// NewDependencyTracker creates a new dependency tracker
func NewDependencyTracker() *DependencyTracker {
	return &DependencyTracker{
		refCreators:       make(map[string]int),
		statefulRefs:      make(map[string]bool),
		depChains:         make(map[int][]int),
		stateDependencies: []int{},
	}
}

// BuildDependenciesForTest analyzes a test's parameters to build its complete transitive
// dependency chain. When a test uses refs created by earlier tests, this records all direct
// dependencies (tests that created those refs) and indirect dependencies (their dependencies).
// Must be called after all previous tests have been processed.
func (dt *DependencyTracker) BuildDependenciesForTest(testIndex int, test *TestCase) {
	// Build dependency chain for current test based on refs it uses
	var parentChains [][]int
	for _, ref := range extractRefsFromParams(test.Request.Params) {
		if creatorIdx, exists := dt.refCreators[ref]; exists {
			// Add the creator as a direct dependency
			parentChains = append(parentChains, []int{creatorIdx})
			// Add transitive dependencies (creator's dependencies)
			if chain, hasChain := dt.depChains[creatorIdx]; hasChain {
				parentChains = append(parentChains, chain)
			}
		} else {
			panic(fmt.Sprintf("test %d (%s) uses undefined reference %s - no prior test created this ref",
				testIndex, test.Request.ID, ref))
		}
	}
	dt.depChains[testIndex] = mergeSortedUnique(parentChains...)
}

// OnTestExecuted is called after a test executes successfully. It tracks the ref
// created by the test, marks it as stateful if needed, and updates state dependencies
// for state-mutating methods.
func (dt *DependencyTracker) OnTestExecuted(testIndex int, test *TestCase) {
	if ref := extractRefFromExpected(test.ExpectedResponse); ref != "" {
		dt.refCreators[ref] = testIndex

		// Mark refs from stateful methods
		if test.Request.Method == "btck_context_create" || test.Request.Method == "btck_chainstate_manager_create" {
			dt.statefulRefs[ref] = true
		}
	}

	// Track state-mutating tests and their dependencies
	if stateMutatingMethods[test.Request.Method] {
		mutatorChain := append(dt.depChains[testIndex], testIndex)
		dt.stateDependencies = mergeSortedUnique(dt.stateDependencies, mutatorChain)
	}
}

// BuildRequestChain builds the complete dependency chain for a test
func (dt *DependencyTracker) BuildRequestChain(testIndex int, allTests []TestCase) []int {
	refDepChain := dt.depChains[testIndex]

	// Only include state dependencies if the test's dep chain contains any stateful refs
	if dt.testUsesStatefulRefs(testIndex, allTests) {
		return mergeSortedUnique(refDepChain, dt.stateDependencies)
	}

	return refDepChain
}

// testUsesStatefulRefs checks if a test's dependency chain includes any stateful refs
func (dt *DependencyTracker) testUsesStatefulRefs(testIndex int, allTests []TestCase) bool {
	// Check all tests in the dependency chain
	for _, depIdx := range dt.depChains[testIndex] {
		for _, ref := range extractRefsFromParams(allTests[depIdx].Request.Params) {
			if dt.statefulRefs[ref] {
				return true
			}
		}
	}

	// Check the test itself
	for _, ref := range extractRefsFromParams(allTests[testIndex].Request.Params) {
		if dt.statefulRefs[ref] {
			return true
		}
	}
	return false
}

// extractRefFromExpected extracts a reference name from the expected result if it's a
// string starting with "$". Returns empty string if not a reference.
func extractRefFromExpected(expected Response) string {
	var resultStr string
	if err := json.Unmarshal(expected.Result, &resultStr); err != nil {
		return ""
	}
	if len(resultStr) > 1 && resultStr[0] == '$' {
		return resultStr
	}
	return ""
}

// extractRefsFromParams extracts all reference names from params JSON.
func extractRefsFromParams(params json.RawMessage) []string {
	var refs []string

	// Parse params as a generic structure
	var paramsMap map[string]interface{}
	if err := json.Unmarshal(params, &paramsMap); err != nil {
		return refs
	}

	// Find all string values starting with "$" (assumes refs are at first level only)
	for _, value := range paramsMap {
		if str, ok := value.(string); ok && len(str) > 1 && str[0] == '$' {
			refs = append(refs, str)
		}
	}
	return refs
}
