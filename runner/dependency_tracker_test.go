package runner

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestDependencyTracker_BuildDependencyChains(t *testing.T) {
	// Create test cases to verify dependency chain building
	testsJSON := `[
		{
			"request": {
				"id": "test0",
				"method": "create_a",
				"params": {}
			},
			"expected_response": {"result": "$ref_a"}
		},
		{
			"request": {
				"id": "test1",
				"method": "create_b",
				"params": {"input": "$ref_a"}
			},
			"expected_response": {"result": "$ref_b"}
		},
		{
			"request": {
				"id": "test2",
				"method": "create_c",
				"params": {}
			},
			"expected_response": {"result": "$ref_c"}
		},
		{
			"request": {
				"id": "test3",
				"method": "use_multiple",
				"params": {"first": "$ref_b", "second": "$ref_c"}
			},
			"expected_response": {}
		}
	]`

	var testCases []TestCase
	if err := json.Unmarshal([]byte(testsJSON), &testCases); err != nil {
		t.Fatalf("failed to unmarshal test cases: %v", err)
	}

	// Create dependency tracker and simulate test execution
	tracker := NewDependencyTracker()

	for i := range testCases {
		test := &testCases[i]
		tracker.BuildDependenciesForTest(i, test)
		tracker.OnTestExecuted(i, test)
	}

	// Verify dependency chains
	tests := []struct {
		testIdx      int
		wantDepChain []int
		description  string
	}{
		{
			testIdx:      0,
			wantDepChain: []int{},
			description:  "test0 has no dependencies",
		},
		{
			testIdx:      1,
			wantDepChain: []int{0},
			description:  "test1 depends on test0",
		},
		{
			testIdx:      2,
			wantDepChain: []int{},
			description:  "test2 has no dependencies",
		},
		{
			testIdx:      3,
			wantDepChain: []int{0, 1, 2},
			description:  "test3 depends on test1 (which depends on test0) and test2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			got := tracker.depChains[tt.testIdx]
			if !slices.Equal(got, tt.wantDepChain) {
				t.Errorf("depChains[%d] = %v, want %v", tt.testIdx, got, tt.wantDepChain)
			}
		})
	}
}

func TestDependencyTracker_StatefulRefs(t *testing.T) {
	testsJSON := `[
		{
			"request": {
				"id": "test0",
				"method": "btck_context_create",
				"params": {}
			},
			"expected_response": {"result": "$context_ref"}
		},
		{
			"request": {
				"id": "test1",
				"method": "btck_chainstate_manager_create",
				"params": {"context": "$context_ref"}
			},
			"expected_response": {"result": "$chainman_ref"}
		},
		{
			"request": {
				"id": "test2",
				"method": "btck_block_create",
				"params": {"raw_block": "deadbeef"}
			},
			"expected_response": {"result": "$block_ref"}
		}
	]`

	var testCases []TestCase
	if err := json.Unmarshal([]byte(testsJSON), &testCases); err != nil {
		t.Fatalf("failed to unmarshal test cases: %v", err)
	}

	tracker := NewDependencyTracker()

	for i := range testCases {
		test := &testCases[i]
		tracker.BuildDependenciesForTest(i, test)
		tracker.OnTestExecuted(i, test)
	}

	// Verify that context and chainstate_manager refs are marked as stateful
	if !tracker.statefulRefs["$context_ref"] {
		t.Error("$context_ref should be marked as stateful")
	}
	if !tracker.statefulRefs["$chainman_ref"] {
		t.Error("$chainman_ref should be marked as stateful")
	}
	if tracker.statefulRefs["$block_ref"] {
		t.Error("$block_ref should NOT be marked as stateful")
	}
}

func TestDependencyTracker_StateMutations(t *testing.T) {
	testsJSON := `[
		{
			"request": {
				"id": "test0",
				"method": "btck_context_create",
				"params": {}
			},
			"expected_response": {"result": "$context_ref"}
		},
		{
			"request": {
				"id": "test1",
				"method": "btck_chainstate_manager_create",
				"params": {"context": "$context_ref"}
			},
			"expected_response": {"result": "$chainman_ref"}
		},
		{
			"request": {
				"id": "test2",
				"method": "btck_block_create",
				"params": {"raw_block": "deadbeef"}
			},
			"expected_response": {"result": "$block_ref"}
		},
		{
			"request": {
				"id": "test3",
				"method": "btck_chainstate_manager_process_block",
				"params": {"chainstate_manager": "$chainman_ref", "block": "$block_ref"}
			},
			"expected_response": {}
		},
		{
			"request": {
				"id": "test4",
				"method": "btck_block_create",
				"params": {"raw_block": "cafebabe"}
			},
			"expected_response": {"result": "$block2_ref"}
		}
	]`

	var testCases []TestCase
	if err := json.Unmarshal([]byte(testsJSON), &testCases); err != nil {
		t.Fatalf("failed to unmarshal test cases: %v", err)
	}

	tracker := NewDependencyTracker()

	for i := range testCases {
		test := &testCases[i]
		tracker.BuildDependenciesForTest(i, test)
		tracker.OnTestExecuted(i, test)
	}

	// State dependencies should include test3 (process_block) and its dependencies (0, 1, 2)
	expectedStateDeps := []int{0, 1, 2, 3}
	if !slices.Equal(tracker.stateDependencies, expectedStateDeps) {
		t.Errorf("state dependencies = %v, want %v", tracker.stateDependencies, expectedStateDeps)
	}
}

func TestDependencyTracker_BuildRequestChain(t *testing.T) {
	testsJSON := `[
		{
			"request": {
				"id": "test0",
				"method": "btck_context_create",
				"params": {}
			},
			"expected_response": {"result": "$context_ref"}
		},
		{
			"request": {
				"id": "test1",
				"method": "btck_chainstate_manager_create",
				"params": {"context": "$context_ref"}
			},
			"expected_response": {"result": "$chainman_ref"}
		},
		{
			"request": {
				"id": "test2",
				"method": "btck_block_create",
				"params": {"raw_block": "deadbeef"}
			},
			"expected_response": {"result": "$block_ref"}
		},
		{
			"request": {
				"id": "test3",
				"method": "btck_chainstate_manager_process_block",
				"params": {"chainstate_manager": "$chainman_ref", "block": "$block_ref"}
			},
			"expected_response": {}
		},
		{
			"request": {
				"id": "test4",
				"method": "btck_block_create",
				"params": {"raw_block": "cafebabe"}
			},
			"expected_response": {"result": "$block2_ref"}
		},
		{
			"request": {
				"id": "test5",
				"method": "btck_chainstate_manager_get_active_chain",
				"params": {"chainstate_manager": "$chainman_ref"}
			},
			"expected_response": {"result": "$chain_ref"}
		}
	]`

	var testCases []TestCase
	if err := json.Unmarshal([]byte(testsJSON), &testCases); err != nil {
		t.Fatalf("failed to unmarshal test cases: %v", err)
	}

	tracker := NewDependencyTracker()

	for i := range testCases {
		test := &testCases[i]
		tracker.BuildDependenciesForTest(i, test)
		tracker.OnTestExecuted(i, test)
	}

	tests := []struct {
		testIdx     int
		wantChain   []int
		description string
	}{
		{
			testIdx:     4,
			wantChain:   []int{}, // block_create doesn't use stateful refs, so no state deps included
			description: "test4 (block_create) should NOT include state dependencies",
		},
		{
			testIdx:     5,
			wantChain:   []int{0, 1, 2, 3}, // uses chainman_ref (stateful), so includes state deps
			description: "test5 (get_active_chain) should include state dependencies",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			got := tracker.BuildRequestChain(tt.testIdx, testCases)
			if !slices.Equal(got, tt.wantChain) {
				t.Errorf("BuildRequestChain(%d) = %v, want %v", tt.testIdx, got, tt.wantChain)
			}
		})
	}
}
