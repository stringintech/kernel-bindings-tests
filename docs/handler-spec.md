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
  "params": { /* method-specific parameters */ },
  "ref": "reference-name"
}
```

**Fields:**
- `id` (string, required): Unique identifier for this request
- `method` (string, required): The operation to perform. Each unique method must be implemented by the handler to exercise the corresponding binding API operation.
- `params` (object, optional): Method-specific parameters
- `ref` (string, optional): Reference name for storing the returned object. Required for methods that return object references (see [Object References and Registry](#object-references-and-registry))

### Response

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

**Fields:**
- `id` (string, required): Must match the request ID
- `result` (any, optional): The return value, or `null` for void/nullptr operations. Must be `null` on error
- `error` (object, optional): Error details. Must be `null` on success. An empty object `{}` is used to indicate an error is raised without further details, it is NOT equivalent to `null`
  - `code` (object, optional): Error code details
    - `type` (string, required): Error type (e.g., "btck_ScriptVerifyStatus")
    - `member` (string, required): Specific error member (e.g., "ERROR_INVALID_FLAGS_COMBINATION")

**Note:** Throughout this protocol, an omitted field is semantically equivalent to `null`.

## Handler Requirements

1. **Input Processing**: Read JSON requests line-by-line from stdin
2. **Response Order**: Responses must match request order (process sequentially)
3. **ID Matching**: Response `id` must exactly match the request `id`
4. **Error Handling**: Return error responses for invalid requests or failed operations
5. **Exit Behavior**: Exit cleanly when stdin closes

## Object References and Registry

Many operations return objects (contexts, blocks, chains, etc.) that must persist across requests. The protocol uses named references and a registry pattern:

**Creating Objects**: Methods that return objects require a `ref` field in the request. The handler stores the object in a registry under that name and returns the reference name as the result.

```json
// Request
{"id": "1", "method": "btck_context_create", "params": {...}, "ref": "$ctx1"}
// Response
{"id": "1", "result": "$ctx1", "error": null}
// Handler action: registry["$ctx1"] = created_context_ptr
```

**Using Objects**: When a parameter is marked as `(reference, required)`, the runner passes the reference name and the handler looks it up:

```json
// Request
{"id": "2", "method": "btck_chainstate_manager_create", "params": {"context": "$ctx1"}, "ref": "$csm1"}
// Handler action: Look up registry["$ctx1"], create manager, store as registry["$csm1"]
```

**Implementation**: Handlers must maintain a registry (map of reference names to object pointers) throughout their lifetime. Objects remain alive until explicitly destroyed or handler exit.

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
