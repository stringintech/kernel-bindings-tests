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
				"request": {"id": "1"},
				"expected_response": {"result": true}
			}`,
			responseJSON: `{
				"result": true
			}`,
			wantErr: false,
		},
		{
			name: "success with null result explicit",
			testCaseJSON: `{
				"request": {"id": "2"},
				"expected_response": {}
			}`,
			responseJSON: `{
				"result": null
			}`,
			wantErr: false,
		},
		{
			name: "success with null result omitted",
			testCaseJSON: `{
				"request": {"id": "3"},
				"expected_response": {}
			}`,
			responseJSON: `{
			}`,
			wantErr: false,
		},
		{
			name: "error exact match",
			testCaseJSON: `{
				"request": {"id": "4"},
				"expected_response": {
					"error": {
						"code": {
							"type": "type",
							"member": "MEMBER"
						}
					}
				}
			}`,
			responseJSON: `{
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
				"request": {"id": "5"},
				"expected_response": {
					"error": {
						"code": {
							"type": "type",
							"member": "MEMBER"
						}
					}
				}
			}`,
			responseJSON: `{
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
				"request": {"id": "6"},
				"expected_response": {
					"error": {
						"code": {
							"type": "type",
							"member": "MEMBER"
						}
					}
				}
			}`,
			responseJSON: `{
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
				"request": {"id": "7"},
				"expected_response": {"result": true}
			}`,
			responseJSON: `{
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
				"request": {"id": "8"},
				"expected_response": {
					"error": {
						"code": {
							"type": "type",
							"member": "MEMBER"
						}
					}
				}
			}`,
			responseJSON: `{
				"result": true
			}`,
			wantErr:    true,
			wantErrMsg: "expected error",
		},
		{
			name: "result value mismatch",
			testCaseJSON: `{
				"request": {"id": "9"},
				"expected_response": {"result": true}
			}`,
			responseJSON: `{
				"result": false
			}`,
			wantErr:    true,
			wantErrMsg: "result mismatch",
		},
		{
			name: "protocol violation with result not null when error present",
			testCaseJSON: `{
				"request": {"id": "11"},
				"expected_response": {
					"error": {
						"code": {
							"type": "type",
							"member": "MEMBER"
						}
					}
				}
			}`,
			responseJSON: `{
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
				"request": {"id": "12"},
				"expected_response": {
					"error": {}
				}
			}`,
			responseJSON: `{
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

			err := validateResponse(&testCase, &response)

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
