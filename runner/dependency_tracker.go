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

	// usesStatefulRefs caches whether each test (by index) or any test it transitively depends on uses a stateful ref
	usesStatefulRefs map[int]bool

	// processedTestsCount is the number of tests processed so far, used to assign sequential indices
	processedTestsCount int
}

// NewDependencyTracker creates a new dependency tracker
func NewDependencyTracker() *DependencyTracker {
	return &DependencyTracker{
		refCreators:       make(map[string]int),
		statefulRefs:      make(map[string]bool),
		depChains:         make(map[int][]int),
		stateDependencies: []int{},
		usesStatefulRefs:  make(map[int]bool),
	}
}

// OnTestExecuted is called after a test executes. It computes and returns the request chain
// for the test, then updates internal state so subsequent tests see this test's refs and mutations.
func (dt *DependencyTracker) OnTestExecuted(test *TestCase) []int {
	i := dt.processedTestsCount
	refs := extractRefsFromParams(test.Request.Params)

	requestChain := dt.buildRequestChain(i, test.Request.ID, refs)

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

	dt.processedTestsCount++
	return requestChain
}

func (dt *DependencyTracker) buildRequestChain(i int, testID string, refs []string) []int {
	var parentChains [][]int
	for _, ref := range refs {
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

	if dt.computeUsesStatefulRefs(i, refs) {
		return mergeSortedUnique(dt.depChains[i], dt.stateDependencies)
	}
	return dt.depChains[i]
}

// computeUsesStatefulRefs computes and caches whether test i (or any test it depends on)
// uses a stateful ref. deps must already be cached for all indices in depChains[i].
func (dt *DependencyTracker) computeUsesStatefulRefs(i int, refs []string) bool {
	result := false
	for _, ref := range refs {
		if dt.statefulRefs[ref] {
			result = true
			break
		}
	}
	if !result {
		for _, depIdx := range dt.depChains[i] {
			if dt.usesStatefulRefs[depIdx] {
				result = true
				break
			}
		}
	}
	dt.usesStatefulRefs[i] = result
	return result
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
