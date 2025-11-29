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
- `params` (object, optional): Method-specific parameters

### Success Response

```json
{
  "id": "unique-request-id",
  "result": {},
  "error": null
}
```

**Success response fields:**
- `id` (string, required): Must match the request ID
- `result` (any, optional): The return value, or `null` for void/nullptr operations
- `error`: Must be `null` on success

**Note:** Throughout this protocol, an omitted field is semantically equivalent to `null`.

### Error Response

```json
{
  "id": "unique-request-id",
  "result": null,
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
- `result`: Must be `null` on error
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

Test cases where the script verification operation executes successfully and returns a boolean result (true for valid scripts, false for invalid scripts).

**Method:** `btck_script_pubkey_verify`

**Expected Response Format:**
```json
{
  "id": "test-id",
  "result": true
}
```
or
```json
{
  "id": "test-id",
  "result": false
}
```

### Script Verification Error Cases
**File:** [`script_verify_errors.json`](../testdata/script_verify_errors.json)

Test cases where the verification operation fails to determine validity of the script due to bad user input.

**Method:** `btck_script_pubkey_verify`

**Expected Response Format:**
```json
{
  "id": "test-id",
  "result": null,
  "error": {
    "code": {
      "type": "btck_ScriptVerifyStatus",
      "member": "ERROR_MEMBER_NAME"
    }
  }
}
```

**Error Members:**

| Member | Description |
|--------|-------------|
| `ERROR_INVALID_FLAGS_COMBINATION` | Invalid or inconsistent verification flags were provided. This occurs when the supplied `script_verify_flags` combination violates internal consistency rules. |
| `ERROR_SPENT_OUTPUTS_REQUIRED` | Spent outputs are required but were not provided (e.g., for Taproot verification). |
