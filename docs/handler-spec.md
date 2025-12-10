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
- `result` (any, optional): The return value, or `null` for void/nullptr operations. Must be `null` on error. For methods that return object references, the result is a reference type object (see [Reference Type](#reference-type))
- `error` (object, optional): Error details. Must be `null` on success. An empty object `{}` is used to indicate an error is raised without further details, it is NOT equivalent to `null`
  - `code` (object, optional): Error code details
    - `type` (string, required): Error type (e.g., "btck_ScriptVerifyStatus")
    - `member` (string, required): Specific error member (e.g., "ERROR_INVALID_FLAGS_COMBINATION")

### Reference Type

For methods that return object references, the result is an object containing the reference name:

```json
{
  "ref": "reference-name"
}
```

**Fields:**
- `ref` (string, required): The reference name from the request's `ref` field

**Note:** Throughout this protocol, an omitted field is semantically equivalent to `null`.

## Handler Requirements

1. **Input Processing**: Read JSON requests line-by-line from stdin
2. **Response Order**: Responses must match request order (process sequentially)
3. **Error Handling**: Return error responses for invalid requests or failed operations
4. **Exit Behavior**: Exit cleanly when stdin closes

## Object References and Registry

Many operations return objects (contexts, blocks, chains, etc.) that must persist across requests. The protocol uses named references and a registry pattern:

**Creating Objects**: Methods that return objects require a `ref` field in the request. The handler stores the object in a registry under that name and returns a reference type object containing the reference name.

```json
// Request
{"id": "1", "method": "btck_context_create", "params": {...}, "ref": "$ctx1"}
// Response
{"id": "1", "result": {"ref": "$ctx1"}, "error": null}
// Handler action: registry["$ctx1"] = created_context_ptr
```

**Using Objects**: When a parameter is marked as `(reference, required)`, the runner passes a reference type object and the handler extracts the reference name to look it up:

```json
// Request
{"id": "2", "method": "btck_chainstate_manager_create", "params": {"context": {"ref": "$ctx1"}}, "ref": "$csm1"}
// Response
{"id": "2", "result": {"ref": "$csm1"}, "error": null}
// Handler action: Extract ref from params.context, look up registry["$ctx1"], create manager, store as registry["$csm1"]
```

**Implementation**: Handlers must maintain a registry (map of reference names to object pointers) throughout their lifetime. Objects remain alive until explicitly destroyed or handler exit.

## Test Suites Overview

The conformance tests are organized into suites, each testing a specific aspect of the Bitcoin Kernel bindings. Test files are located in [`../testdata/`](../testdata/).

### Script Verification Success Cases
**File:** [`script_verify_success.json`](../testdata/script_verify_success.json)

Test cases where the script verification operation executes successfully and returns a boolean result (true for valid scripts, false for invalid scripts).

### Script Verification Error Cases
**File:** [`script_verify_errors.json`](../testdata/script_verify_errors.json)

Test cases where the verification operation fails to determine validity of the script due to bad user input.

### Chain Operations
**File:** [`chain.json`](../testdata/chain.json)

Sets up blocks, checks chain state, and verifies that the chain tip changes as expected after a reorg scenario.

## Method Reference

Methods are grouped by functional area. Each method documents its parameters, return values, and possible errors.

### Context Management

#### `btck_context_create`

Creates a context with specified chain parameters.

**Parameters:**
- `chain_parameters` (object, required):
  - `chain_type` (string, required): Chain type ("btck_ChainType_MAINNET", "btck_ChainType_TESTNET", "btck_ChainType_TESTNET_4", "btck_ChainType_SIGNET", "btck_ChainType_REGTEST")

**Result:** Reference type - Object containing the reference name from the request `ref` field (e.g., `{"ref": "$context"}`)

**Error:** `{}` when operation fails (C API returned null)

---

### Chainstate Manager Operations

#### `btck_chainstate_manager_create`

Creates a chainstate manager from a context.

**Parameters:**
- `context` (reference, required): Context reference from `btck_context_create`

**Result:** Reference type - Object containing the reference name from the request `ref` field (e.g., `{"ref": "$chainstate_manager"}`)

**Error:** `{}` when operation fails (C API returned null)

---

#### `btck_chainstate_manager_get_active_chain`

Retrieves the currently active chain from the chainstate manager.

**Parameters:**
- `chainstate_manager` (reference, required): Chainstate manager reference

**Result:** Reference type - Object containing the reference name from the request `ref` field (e.g., `{"ref": "$chain"}`)

**Error:** `null` (cannot return error)

---

#### `btck_chainstate_manager_process_block`

Processes a block through validation checks, disk storage, and UTXO set validation; successful processing does not indicate block validity.

**Parameters:**
- `chainstate_manager` (reference, required): Chainstate manager reference
- `block` (reference, required): Block reference from `btck_block_create`

**Result:** Object containing:
- `new_block` (boolean): `true` if this block was not processed before, `false` otherwise

**Error:** `{}` when processing fails

---

#### `btck_chainstate_manager_destroy`

Destroys a chainstate manager and frees associated resources.

**Parameters:**
- `chainstate_manager` (reference, required): Chainstate manager reference to destroy

**Result:** `null` (void operation)

**Error:** `null` (cannot return error)

---

### Chain Operations

#### `btck_chain_get_height`

Gets the current height of the active chain.

**Parameters:**
- `chain` (reference, required): Chain reference from `btck_chainstate_manager_get_active_chain`

**Result:** Integer - The chain height (0 = genesis)

**Error:** `null` (cannot return error)

---

#### `btck_chain_get_by_height`

Retrieves a block tree entry at a specific height in the chain.

**Parameters:**
- `chain` (reference, required): Chain reference
- `block_height` (integer, required): Height to query

**Result:** Reference type - Object containing the reference name from the request `ref` field (e.g., `{"ref": "$block_tree_entry"}`)

**Error:** `{}` when height is out of bounds (C API returned null)

---

#### `btck_chain_contains`

Checks whether a block tree entry is part of the active chain.

**Parameters:**
- `chain` (reference, required): Chain reference
- `block_tree_entry` (reference, required): Block tree entry reference to check

**Result:** Boolean - true if block is in the active chain, false otherwise

**Error:** `null` (cannot return error)

---

### Block Operations

#### `btck_block_create`

Creates a block object from raw block data.

**Parameters:**
- `raw_block` (string, required): Hex-encoded raw block data

**Result:** Reference type - Object containing the reference name from the request `ref` field (e.g., `{"ref": "$block"}`)

**Error:** `{}` when operation fails (C API returned null)

---

#### `btck_block_tree_entry_get_block_hash`

Gets the block hash from a block tree entry.

**Parameters:**
- `block_tree_entry` (reference, required): Block tree entry reference from `btck_chain_get_by_height`

**Result:** String - The block hash (hex-encoded, 64 characters)

**Error:** `null` (cannot return error)

---

### Script Verification

#### `btck_script_pubkey_verify`

Verifies a script pubkey against spending conditions.

**Parameters:**
- `script_pubkey` (string): Hex-encoded script pubkey to be spent
- `amount` (number): Amount of the script pubkey's associated output. May be zero if the witness flag is not set
- `tx_to` (string): Hex-encoded transaction spending the script_pubkey
- `input_index` (number): Index of the input in tx_to spending the script_pubkey
- `flags` (array of strings): Script verification flags controlling validation constraints. Valid flags include:
  - `btck_ScriptVerificationFlags_P2SH`
  - `btck_ScriptVerificationFlags_DERSIG`
  - `btck_ScriptVerificationFlags_NULLDUMMY`
  - `btck_ScriptVerificationFlags_CHECKLOCKTIMEVERIFY`
  - `btck_ScriptVerificationFlags_CHECKSEQUENCEVERIFY`
  - `btck_ScriptVerificationFlags_WITNESS`
  - `btck_ScriptVerificationFlags_TAPROOT`
- `spent_outputs` (array of objects): Array of outputs spent by the transaction. May be empty if the taproot flag is not set. Each object contains:
  - `script_pubkey` (string): Hex-encoded script pubkey of the spent output
  - `amount` (number): Amount in satoshis of the spent output

**Result:** Boolean - true if script is valid, false if invalid

**Error:** On error, returns error code with type `btck_ScriptVerifyStatus` and member can be one of:
- `ERROR_INVALID_FLAGS_COMBINATION` - Invalid or inconsistent verification flags were provided. This occurs when the supplied `script_verify_flags` combination violates internal consistency rules.
- `ERROR_SPENT_OUTPUTS_REQUIRED` - Spent outputs are required but were not provided (e.g., for Taproot verification).
