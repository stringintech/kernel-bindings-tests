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

	// tests accumulates each test as it is executed, for internal param lookups
	tests []TestCase
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

// OnTestExecuted is called after a test executes. It computes and returns the request chain
// for the test, then updates internal state so subsequent tests see this test's refs and mutations.
func (dt *DependencyTracker) OnTestExecuted(test *TestCase) []int {
	dt.tests = append(dt.tests, *test)
	i := len(dt.tests) - 1

	requestChain := dt.buildRequestChain(i, test.Request.ID)

	// Track ref creation using the request's ref field.
	if test.Request.Ref != "" {
		dt.refCreators[test.Request.Ref] = i
		if statefulCreatorMethods[test.Request.Method] {
			dt.statefulRefs[test.Request.Ref] = true
		}
	}

	// Track state mutations so future tests that touch stateful objects include this test in their chain.
	if stateMutatingMethods[test.Request.Method] {
		mutatorChain := append(dt.depChains[i], i)
		dt.stateDependencies = mergeSortedUnique(dt.stateDependencies, mutatorChain)
	}

	return requestChain
}

func (dt *DependencyTracker) buildRequestChain(i int, testID string) []int {
	var parentChains [][]int
	for _, ref := range extractRefsFromParams(dt.tests[i].Request.Params) {
		if creatorIdx, exists := dt.refCreators[ref]; exists {
			parentChains = append(parentChains, []int{creatorIdx})
			if chain, hasChain := dt.depChains[creatorIdx]; hasChain {
				parentChains = append(parentChains, chain)
			}
		} else {
			panic(fmt.Sprintf("test %s uses undefined reference %s - no prior test created this ref",
				testID, ref))
		}
	}
	dt.depChains[i] = mergeSortedUnique(parentChains...)

	if dt.testUsesStatefulRefs(i) {
		return mergeSortedUnique(dt.depChains[i], dt.stateDependencies)
	}
	return dt.depChains[i]
}

// testUsesStatefulRefs checks if a test's dependency chain includes any stateful refs.
func (dt *DependencyTracker) testUsesStatefulRefs(i int) bool {
	for _, depIdx := range dt.depChains[i] {
		for _, ref := range extractRefsFromParams(dt.tests[depIdx].Request.Params) {
			if dt.statefulRefs[ref] {
				return true
			}
		}
	}
	for _, ref := range extractRefsFromParams(dt.tests[i].Request.Params) {
		if dt.statefulRefs[ref] {
			return true
		}
	}
	return false
}

// extractRefsFromParams extracts all reference names from params JSON.
// Searches for ref objects with structure {"ref": "..."} at the top level of params,
// and also inside array values one level deep.
func extractRefsFromParams(params json.RawMessage) []string {
	var refs []string

	var paramsMap map[string]json.RawMessage
	if err := json.Unmarshal(params, &paramsMap); err != nil {
		return refs
	}

	for _, value := range paramsMap {
		if ref, ok := ParseRefObject(value); ok {
			refs = append(refs, ref)
			continue
		}
		// Check if value is an array and scan its elements for ref objects
		var arr []json.RawMessage
		if err := json.Unmarshal(value, &arr); err == nil {
			for _, elem := range arr {
				if ref, ok := ParseRefObject(elem); ok {
					refs = append(refs, ref)
				}
			}
		}
	}
	return refs
}
