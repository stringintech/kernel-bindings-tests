package runner

import (
	"encoding/json"
	"fmt"
)

// statefulCreatorMethods contains methods that create stateful objects.
// Refs created by these methods are tracked as stateful, meaning tests
// using these refs depend on mutable state.
var statefulCreatorMethods = map[string]bool{
	"btck_context_create":            true,
	"btck_chainstate_manager_create": true,
}

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

	// statefulRefs tracks refs created by stateful methods.
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
	// Track ref creation using the request's ref field
	if test.Request.Ref != "" {
		dt.refCreators[test.Request.Ref] = testIndex

		// Mark refs from stateful methods
		if statefulCreatorMethods[test.Request.Method] {
			dt.statefulRefs[test.Request.Ref] = true
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

// extractRefsFromParams extracts all reference names from params JSON.
// Searches for ref objects with structure {"ref": "..."} at the first level of params.
func extractRefsFromParams(params json.RawMessage) []string {
	var refs []string

	var paramsMap map[string]json.RawMessage
	if err := json.Unmarshal(params, &paramsMap); err != nil {
		return refs
	}

	for _, value := range paramsMap {
		if ref, ok := ParseRefObject(value); ok {
			refs = append(refs, ref)
		}
	}
	return refs
}
