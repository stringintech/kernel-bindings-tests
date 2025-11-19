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
  "success": { /* method-specific result */ }
}
```

**Success response fields:**
- `id` (string, required): Must match the request ID
- `success` (any, required): Method-specific result data. Must be present on success (can be empty `{}`)
- `error` (null or omitted): Must not be present on success

### Error Response

```json
{
  "id": "unique-request-id",
  "error": {
    "type": "error_category",
    "variant": "specific_error"
  }
}
```

**Error response fields:**
- `id` (string, required): Must match the request ID
- `success` (null or omitted): Must not be present on error
- `error` (object, required): Error details
  - `type` (string, required): Error category/type
  - `variant` (string, optional): Specific error variant within the type. Whether the runner expects this field depends on the specific test case

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

**Method:** `script_pubkey.verify`

**Expected Response Format:**
```json
{
  "id": "test-id",
  "success": {}
}
```

### Script Verification Error Cases
**File:** [`script_verify_errors.json`](../testdata/script_verify_errors.json)

Tests error handling for invalid script verification scenarios.

**Method:** `script_pubkey.verify`

**Expected Response Format:**
```json
{
  "id": "test-id",
  "error": {
    "type": "ScriptVerify",
    "variant": "ErrorVariant"
  }
}
```

**Error Variants:**

| Variant | Description |
|---------|-------------|
| `TxInputIndex` | The specified input index is out of bounds. The `input_index` parameter is greater than or equal to the number of inputs in the transaction. |
| `InvalidFlags` | Invalid verification flags were provided. The flags parameter contains bits that don't correspond to any defined verification flag. |
| `InvalidFlagsCombination` | Invalid or inconsistent verification flags were provided. This occurs when the supplied `script_verify_flags` combination violates internal consistency rules. |
| `SpentOutputsMismatch` | The spent_outputs array length doesn't match the input count. When spent_outputs is non-empty, it must contain exactly one output for each input in the transaction. |
| `SpentOutputsRequired` | Spent outputs are required but were not provided. |
| `Invalid` | Script verification failed. |