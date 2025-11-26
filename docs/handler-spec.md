# Handler Specification

This document defines the protocol that handler binaries must implement to be compatible with the conformance test runner.

## Communication Protocol

Handlers communicate with the test runner via **stdin/stdout**:
- **Input**: JSON requests on stdin (one per line)
- **Output**: JSON responses on stdout (one per line)
- **Lifecycle**: Handler starts, processes requests until stdin closes, then exits

## Message Format

### Request

```json
{
  "id": "unique-request-id",
  "method": "method_name",
  "params": { /* method-specific parameters */ }
}
```

**Fields:**
- `id` (string, required): Unique identifier for this request
- `method` (string, required): The operation to perform. Each unique method must be implemented by the handler to exercise the corresponding binding API operation.
- `params` (object, required): Method-specific parameters (can be `null` or `{}`)

### Response

```json
{
  "id": "unique-request-id",
  "success": true
}
```

**Success response fields:**
- `id` (string, required): Must match the request ID
- `success` (boolean, required): Must be `true` on successful operation
- `error` (null or omitted): Must not be present on success

### Error Response

```json
{
  "id": "unique-request-id",
  "success": false,
  "error": {
    "code": {
      "type": "error_type",
      "member": "ERROR_MEMBER_NAME"
    }
  }
}
```

**Error response fields:**
- `id` (string, required): Must match the request ID
- `success` (boolean, required): Must be `false` on error
- `error` (object, optional): Error details. Whether this field is required depends on the specific test case.
  - `code` (object, optional): Error code details
    - `type` (string, required): Error type (e.g., "btck_ScriptVerifyStatus")
    - `member` (string, required): Specific error member (e.g., "ERROR_INVALID_FLAGS_COMBINATION")

## Handler Requirements

1. **Input Processing**: Read JSON requests line-by-line from stdin
2. **Response Order**: Responses must match request order (process sequentially)
3. **ID Matching**: Response `id` must exactly match the request `id`
4. **Error Handling**: Return error responses for invalid requests or failed operations
5. **Exit Behavior**: Exit cleanly when stdin closes

## Test Suites and Expected Responses

The conformance tests are organized into suites, each testing a specific aspect of the Bitcoin Kernel bindings. Test files are located in [`../testdata/`](../testdata/).

### Script Verification Success Cases
**File:** [`script_verify_success.json`](../testdata/script_verify_success.json)

Tests valid Bitcoin script verification scenarios across different transaction types.

**Method:** `btck_script_pubkey_verify`

**Expected Response Format:**
```json
{
  "id": "test-id",
  "success": true
}
```

### Script Verification Error Cases
**File:** [`script_verify_errors.json`](../testdata/script_verify_errors.json)

Tests error handling for invalid script verification scenarios.

**Method:** `btck_script_pubkey_verify`

**Expected Response Formats:**

**With specific error code:**
```json
{
  "id": "test-id",
  "success": false,
  "error": {
    "code": {
      "type": "btck_ScriptVerifyStatus",
      "member": "ERROR_MEMBER_NAME"
    }
  }
}
```

**Generic failure (no error details):**
```json
{
  "id": "test-id",
  "success": false
}
```

**Error Members:**

| Member | Description |
|--------|-------------|
| `ERROR_INVALID_FLAGS_COMBINATION` | Invalid or inconsistent verification flags were provided. This occurs when the supplied `script_verify_flags` combination violates internal consistency rules. |
| `ERROR_SPENT_OUTPUTS_REQUIRED` | Spent outputs are required but were not provided (e.g., for Taproot verification). |
