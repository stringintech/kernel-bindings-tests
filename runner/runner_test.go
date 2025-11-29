package runner

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateResponse(t *testing.T) {
	tests := []struct {
		name         string
		testCaseJSON string
		responseJSON string
		wantErr      bool
		wantErrMsg   string
	}{
		{
			name: "success with boolean result",
			testCaseJSON: `{
				"id": "1",
				"expected": {"result": true}
			}`,
			responseJSON: `{
				"id": "1",
				"result": true
			}`,
			wantErr: false,
		},
		{
			name: "success with null result explicit",
			testCaseJSON: `{
				"id": "2",
				"expected": {}
			}`,
			responseJSON: `{
				"id": "2",
				"result": null
			}`,
			wantErr: false,
		},
		{
			name: "success with null result omitted",
			testCaseJSON: `{
				"id": "3",
				"expected": {}
			}`,
			responseJSON: `{
				"id": "3"
			}`,
			wantErr: false,
		},
		{
			name: "error exact match",
			testCaseJSON: `{
				"id": "4",
				"expected": {
					"error": {
						"code": {
							"type": "type",
							"member": "MEMBER"
						}
					}
				}
			}`,
			responseJSON: `{
				"id": "4",
				"result": null,
				"error": {
					"code": {
						"type": "type",
						"member": "MEMBER"
					}
				}
			}`,
			wantErr: false,
		},
		{
			name: "error type mismatch",
			testCaseJSON: `{
				"id": "5",
				"expected": {
					"error": {
						"code": {
							"type": "type",
							"member": "MEMBER"
						}
					}
				}
			}`,
			responseJSON: `{
				"id": "5",
				"error": {
					"code": {
						"type": "different_type",
						"member": "MEMBER"
					}
				}
			}`,
			wantErr:    true,
			wantErrMsg: "expected error type",
		},
		{
			name: "error member mismatch",
			testCaseJSON: `{
				"id": "6",
				"expected": {
					"error": {
						"code": {
							"type": "type",
							"member": "MEMBER"
						}
					}
				}
			}`,
			responseJSON: `{
				"id": "6",
				"error": {
					"code": {
						"type": "type",
						"member": "DIFFERENT_MEMBER"
					}
				}
			}`,
			wantErr:    true,
			wantErrMsg: "expected error member",
		},
		{
			name: "expected success got error",
			testCaseJSON: `{
				"id": "7",
				"expected": {"result": true}
			}`,
			responseJSON: `{
				"id": "7",
				"result": null,
				"error": {
					"code": {
						"type": "type",
						"member": "MEMBER"
					}
				}
			}`,
			wantErr:    true,
			wantErrMsg: "expected success with no error",
		},
		{
			name: "expected error got success",
			testCaseJSON: `{
				"id": "8",
				"expected": {
					"error": {
						"code": {
							"type": "type",
							"member": "MEMBER"
						}
					}
				}
			}`,
			responseJSON: `{
				"id": "8",
				"result": true
			}`,
			wantErr:    true,
			wantErrMsg: "expected error",
		},
		{
			name: "result value mismatch",
			testCaseJSON: `{
				"id": "9",
				"expected": {"result": true}
			}`,
			responseJSON: `{
				"id": "9",
				"result": false
			}`,
			wantErr:    true,
			wantErrMsg: "result mismatch",
		},
		{
			name: "response ID mismatch",
			testCaseJSON: `{
				"id": "10",
				"expected": {"result": true}
			}`,
			responseJSON: `{
				"id": "99",
				"result": true
			}`,
			wantErr:    true,
			wantErrMsg: "response ID mismatch",
		},
		{
			name: "protocol violation with result not null when error present",
			testCaseJSON: `{
				"id": "11",
				"expected": {
					"error": {
						"code": {
							"type": "type",
							"member": "MEMBER"
						}
					}
				}
			}`,
			responseJSON: `{
				"id": "11",
				"result": true,
				"error": {
					"code": {
						"type": "type",
						"member": "MEMBER"
					}
				}
			}`,
			wantErr:    true,
			wantErrMsg: "expected result to be null or omitted when error is present",
		},
		{
			name: "error generic without code",
			testCaseJSON: `{
				"id": "12",
				"expected": {
					"error": {}
				}
			}`,
			responseJSON: `{
				"id": "12",
				"result": null,
				"error": {}
			}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var testCase TestCase
			if err := json.Unmarshal([]byte(tt.testCaseJSON), &testCase); err != nil {
				t.Fatalf("failed to unmarshal test case: %v", err)
			}

			var response Response
			if err := json.Unmarshal([]byte(tt.responseJSON), &response); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			err := validateResponse(testCase, &response)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErrMsg)
					return
				}
				if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErrMsg)) {
					t.Errorf("expected error containing %q, got %q", tt.wantErrMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			}
		})
	}
}
