package runner

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestExtractRefsFromParams(t *testing.T) {
	tests := []struct {
		description string
		params      string
		wantRefs    []string
	}{
		{
			description: "direct ref at top level",
			params:      `{"input": {"ref": "$ref_a"}}`,
			wantRefs:    []string{"$ref_a"},
		},
		{
			description: "refs inside array param",
			params:      `{"items": [{"ref": "$ref_a"}, {"ref": "$ref_b"}]}`,
			wantRefs:    []string{"$ref_a", "$ref_b"},
		},
		{
			description: "mixed direct and array refs",
			params:      `{"direct": {"ref": "$ref_a"}, "items": [{"ref": "$ref_b"}]}`,
			wantRefs:    []string{"$ref_a", "$ref_b"},
		},
		{
			description: "deeply nested ref in array is not extracted",
			params:      `{"direct": {"ref": "$ref_a"}, "items": [{"nested": {"ref": "$ref_b"}}]}`,
			wantRefs:    []string{"$ref_a"},
		},
		{
			description: "non-ref values in array are ignored",
			params:      `{"items": [{"ref": "$ref_a"}, "plain_string", 42]}`,
			wantRefs:    []string{"$ref_a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			got := extractRefsFromParams([]byte(tt.params))
			slices.Sort(got)
			slices.Sort(tt.wantRefs)
			if !slices.Equal(got, tt.wantRefs) {
				t.Errorf("extractRefsFromParams(%s) = %v, want %v", tt.params, got, tt.wantRefs)
			}
		})
	}
}

func TestExtractRefsFromResult(t *testing.T) {
	tests := []struct {
		description string
		result      string
		wantRefs    []string
	}{
		{
			description: "single ref object — normal create method result",
			result:      `{"ref": "$ctx"}`,
			wantRefs:    []string{"$ctx"},
		},
		{
			description: "array of objects with nested refs — drain result",
			result:      `[{"callback": "btck_NotifyBlockTip", "entry": {"ref": "$notif_1_btck_NotifyBlockTip_entry"}}, {"callback": "btck_NotifyBlockTip", "entry": {"ref": "$notif_2_btck_NotifyBlockTip_entry"}}]`,
			wantRefs:    []string{"$notif_1_btck_NotifyBlockTip_entry", "$notif_2_btck_NotifyBlockTip_entry"},
		},
		{
			description: "primitive result produces no refs",
			result:      `42`,
			wantRefs:    nil,
		},
		{
			description: "null result produces no refs",
			result:      `null`,
			wantRefs:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			got := extractRefsFromResult(Result(tt.result))
			slices.Sort(got)
			slices.Sort(tt.wantRefs)
			if !slices.Equal(got, tt.wantRefs) {
				t.Errorf("extractRefsFromResult(%s) = %v, want %v", tt.result, got, tt.wantRefs)
			}
		})
	}
}

func TestDependencyTracker(t *testing.T) {
	type entry struct {
		tc            TestCase
		expectedChain []int
	}

	type suite struct {
		name  string
		cases []entry
	}

	suites := []suite{
		{
			// ref dependency chains are built from params: a test that uses a ref inherits the
			// full transitive chain of tests that produced it, including refs passed via arrays
			name: "ref-chains",
			cases: []entry{
				{
					tc: TestCase{
						Request: Request{
							ID:     "ref-chains#0",
							Method: "create_a",
							Params: json.RawMessage(`{}`),
							Ref:    "$ref_a",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$ref_a"}`)},
					},
					expectedChain: []int{},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "ref-chains#1",
							Method: "create_b",
							Params: json.RawMessage(`{"input": {"ref": "$ref_a"}}`),
							Ref:    "$ref_b",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$ref_b"}`)},
					},
					expectedChain: []int{0},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "ref-chains#2",
							Method: "create_c",
							Params: json.RawMessage(`{}`),
							Ref:    "$ref_c",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$ref_c"}`)},
					},
					expectedChain: []int{},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "ref-chains#3",
							Method: "use_multiple",
							Params: json.RawMessage(`{"first": {"ref": "$ref_b"}, "second": {"ref": "$ref_c"}}`),
						},
						ExpectedResponse: Response{},
					},
					expectedChain: []int{0, 1, 2},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "ref-chains#4",
							Method: "use_array",
							Params: json.RawMessage(`{"items": [{"ref": "$ref_a"}, {"ref": "$ref_c"}]}`),
						},
						ExpectedResponse: Response{},
					},
					expectedChain: []int{0, 2},
				},
			},
		},
		{
			// stateful kernel objects (context, chainman) cause state mutations to accumulate as
			// state deps; any later test that uses a stateful ref inherits those deps. state deps
			// are tracked globally, not per object instance, so a mutation on one chainman bleeds
			// into unrelated operations on a different chainman created afterwards — a known
			// documented limitation of the dependency tracker (see state-mutations#7, state-mutations#8)
			name: "state-mutations",
			cases: []entry{
				{
					tc: TestCase{
						Request: Request{
							ID:     "state-mutations#0",
							Method: "btck_context_create",
							Params: json.RawMessage(`{}`),
							Ref:    "$context",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$context"}`)},
					},
					expectedChain: []int{},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "state-mutations#1",
							Method: "btck_chainstate_manager_create",
							Params: json.RawMessage(`{"context": {"ref": "$context"}}`),
							Ref:    "$chainman",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$chainman"}`)},
					},
					expectedChain: []int{0},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "state-mutations#2",
							Method: "btck_block_create",
							Params: json.RawMessage(`{"raw_block": "deadbeef"}`),
							Ref:    "$block",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$block"}`)},
					},
					expectedChain: []int{},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "state-mutations#3",
							Method: "btck_chainstate_manager_process_block",
							Params: json.RawMessage(`{"chainstate_manager": {"ref": "$chainman"}, "block": {"ref": "$block"}}`),
						},
						ExpectedResponse: Response{},
					},
					expectedChain: []int{0, 1, 2},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "state-mutations#4",
							Method: "btck_block_create",
							Params: json.RawMessage(`{"raw_block": "cafebabe"}`),
							Ref:    "$block2",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$block2"}`)},
					},
					expectedChain: []int{},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "state-mutations#5",
							Method: "btck_chainstate_manager_get_active_chain",
							Params: json.RawMessage(`{"chainstate_manager": {"ref": "$chainman"}}`),
							Ref:    "$chain",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$chain"}`)},
					},
					expectedChain: []int{0, 1, 2, 3},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "state-mutations#6",
							Method: "btck_context_create",
							Params: json.RawMessage(`{}`),
							Ref:    "$context_b",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$context_b"}`)},
					},
					expectedChain: []int{},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "state-mutations#7",
							Method: "btck_chainstate_manager_create",
							Params: json.RawMessage(`{"context": {"ref": "$context_b"}}`),
							Ref:    "$chainman_b",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$chainman_b"}`)},
					},
					expectedChain: []int{0, 1, 2, 3, 6},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "state-mutations#8",
							Method: "btck_chainstate_manager_get_active_chain",
							Params: json.RawMessage(`{"chainstate_manager": {"ref": "$chainman_b"}}`),
							Ref:    "$chain_b",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$chain_b"}`)},
					},
					expectedChain: []int{0, 1, 2, 3, 6, 7},
				},
			},
		},
		{
			// tests callback interface dependency tracking: refs produced in a drain
			// response are stateful, so tests using them inherit all prior state dependencies
			name: "callbacks",
			cases: []entry{
				{
					tc: TestCase{
						Request: Request{
							ID:     "callbacks#0",
							Method: "notification_callbacks_create",
							Params: json.RawMessage(`{"callbacks": ["btck_NotifyBlockTip"]}`),
							Ref:    "$notif",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$notif"}`)},
					},
					expectedChain: []int{},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "callbacks#1",
							Method: "btck_context_create",
							Params: json.RawMessage(`{"notifications": {"ref": "$notif"}}`),
							Ref:    "$context",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$context"}`)},
					},
					expectedChain: []int{0},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "callbacks#2",
							Method: "btck_chainstate_manager_create",
							Params: json.RawMessage(`{"context": {"ref": "$context"}}`),
							Ref:    "$chainman",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$chainman"}`)},
					},
					expectedChain: []int{0, 1},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "callbacks#3",
							Method: "btck_block_create",
							Params: json.RawMessage(`{"raw_block": "deadbeef"}`),
							Ref:    "$block",
						},
						ExpectedResponse: Response{Result: Result(`{"ref": "$block"}`)},
					},
					expectedChain: []int{},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "callbacks#4",
							Method: "btck_chainstate_manager_process_block",
							Params: json.RawMessage(`{"chainstate_manager": {"ref": "$chainman"}, "block": {"ref": "$block"}}`),
						},
						ExpectedResponse: Response{},
					},
					expectedChain: []int{0, 1, 2, 3},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "callbacks#5",
							Method: "notification_callbacks_drain",
							Params: json.RawMessage(`{"interface": {"ref": "$notif"}}`),
						},
						ExpectedResponse: Response{Result: Result(`[{"callback": "btck_NotifyBlockTip", "entry": {"ref": "$entry"}}]`)},
					},
					expectedChain: []int{0, 1, 2, 3, 4},
				},
				{
					tc: TestCase{
						Request: Request{
							ID:     "callbacks#6",
							Method: "btck_block_tree_entry_get_height",
							Params: json.RawMessage(`{"entry": {"ref": "$entry"}}`),
						},
						ExpectedResponse: Response{Result: Result(`1`)},
					},
					expectedChain: []int{0, 1, 2, 3, 4, 5},
				},
			},
		},
	}

	for _, s := range suites {
		t.Run(s.name, func(t *testing.T) {
			tracker := NewDependencyTracker()
			for _, e := range s.cases {
				got := tracker.OnTestExecuted(&e.tc)
				t.Run(e.tc.Request.ID, func(t *testing.T) {
					if !slices.Equal(got, e.expectedChain) {
						t.Errorf("chain = %v, want %v", got, e.expectedChain)
					}
				})
			}
		})
	}
}
