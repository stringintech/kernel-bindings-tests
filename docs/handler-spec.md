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

### Operations on Primitive Types

Test suites covering primitive kernel objects, their serialization, and related value objects.

#### Txid Operations
**File:** [`txid.json`](../testdata/txid.json)

Creates txid objects from parsed transactions, verifies byte serialization and equality, and checks copy and destroy behavior.

#### Block Hash Operations
**File:** [`block_hash.json`](../testdata/block_hash.json)

Creates block hash objects from raw 32-byte values, verifies byte serialization and equality, and checks copy and destroy behavior.

#### Script Pubkey Operations
**File:** [`script_pubkey.json`](../testdata/script_pubkey.json)

Creates script pubkey objects from raw script bytes, verifies round-trip serialization including empty scripts, and checks copy and destroy behavior.

#### Transaction Operations
**File:** [`transaction.json`](../testdata/transaction.json)

Parses raw transactions, rejects malformed inputs, verifies txid and serialization getters, checks input and output counts plus indexed accessors, and exercises copy and destroy behavior.

#### Transaction Input Operations
**File:** [`transaction_input.json`](../testdata/transaction_input.json)

Extracts transaction input objects from a parsed transaction, reads each input's outpoint index and txid, and checks copy and destroy behavior for both input and outpoint objects.

#### Transaction Output Operations
**File:** [`transaction_output.json`](../testdata/transaction_output.json)

Builds transaction output objects from a script pubkey and amount, verifies amount and script getter behavior, and checks copy and destroy behavior.

#### Block Header Operations
**File:** [`block_header.json`](../testdata/block_header.json)

Parses raw block headers, rejects short inputs, verifies header hash generation, field getters, and serialization semantics around trailing bytes, and exercises copy and destroy behavior.

#### Block Operations
**File:** [`block.json`](../testdata/block.json)

Parses full blocks, rejects malformed inputs, verifies block hash and byte serialization, checks header and transaction accessors, and exercises copy and destroy behavior using the mainnet genesis block.

### Script Verification Success Cases

Test cases where the script verification operation executes successfully and returns a boolean result (true for valid scripts, false for invalid scripts).

#### Script Verification — P2PKH
**File:** [`script_verify_p2pkh.json`](../testdata/script_verify_p2pkh.json)

Verifies a real mainnet P2PKH output against three variants of the spending transaction: a valid signature (passes with no flags and with all pre-taproot flags), a corrupted signature (always fails), and a non-DER signature (passes without `btck_ScriptVerificationFlags_DERSIG`, fails when `btck_ScriptVerificationFlags_DERSIG` is set).

#### Script Verification — P2SH Multisig
**File:** [`script_verify_p2sh_multisig.json`](../testdata/script_verify_p2sh_multisig.json)

Verifies a real mainnet P2SH 2-of-3 multisig output against three spending transaction variants: valid signatures (passes with `btck_ScriptVerificationFlags_P2SH` and with all pre-taproot flags), a corrupted signature (fails with `btck_ScriptVerificationFlags_P2SH` but passes without it), and a non-null dummy stack element (passes with `btck_ScriptVerificationFlags_P2SH` alone, fails when `btck_ScriptVerificationFlags_NULLDUMMY` is also set).

#### Script Verification — CLTV
**File:** [`script_verify_cltv.json`](../testdata/script_verify_cltv.json)

Verifies a P2SH output containing `OP_CHECKLOCKTIMEVERIFY` locked to block 100. The transaction with `locktime=100` passes with `btck_ScriptVerificationFlags_P2SH` + `btck_ScriptVerificationFlags_CHECKLOCKTIMEVERIFY` and with all pre-taproot flags. The transaction with `locktime=50` fails when `btck_ScriptVerificationFlags_CHECKLOCKTIMEVERIFY` is enforced but passes when only `btck_ScriptVerificationFlags_P2SH` is set.

#### Script Verification — CSV
**File:** [`script_verify_csv.json`](../testdata/script_verify_csv.json)

Verifies a P2SH output containing `OP_CHECKSEQUENCEVERIFY` locked to sequence 10. The transaction with `sequence=10` passes with `btck_ScriptVerificationFlags_P2SH` + `btck_ScriptVerificationFlags_CHECKSEQUENCEVERIFY` and with all pre-taproot flags. The transaction with `sequence=5` fails when `btck_ScriptVerificationFlags_CHECKSEQUENCEVERIFY` is enforced but passes when only `btck_ScriptVerificationFlags_P2SH` is set.

#### Script Verification — P2SH-P2WPKH
**File:** [`script_verify_p2sh_p2wpkh.json`](../testdata/script_verify_p2sh_p2wpkh.json)

Verifies a real mainnet P2SH-wrapped P2WPKH output against two spending transaction variants: a valid witness signature (passes with `btck_ScriptVerificationFlags_P2SH` + `btck_ScriptVerificationFlags_WITNESS` and with all pre-taproot flags) and a corrupted witness signature (fails with `btck_ScriptVerificationFlags_WITNESS` enforced, passes with `btck_ScriptVerificationFlags_P2SH` only).

#### Script Verification — P2SH-P2WSH
**File:** [`script_verify_p2sh_p2wsh.json`](../testdata/script_verify_p2sh_p2wsh.json)

Verifies a real mainnet P2SH-wrapped P2WSH output against two spending transaction variants: a valid 2-of-3 multisig witness (passes with `btck_ScriptVerificationFlags_P2SH` + `btck_ScriptVerificationFlags_WITNESS` and with all pre-taproot flags) and a corrupted witness signature (fails with `btck_ScriptVerificationFlags_WITNESS` enforced, passes with `btck_ScriptVerificationFlags_P2SH` only).

#### Script Verification — P2WPKH
**File:** [`script_verify_p2wpkh.json`](../testdata/script_verify_p2wpkh.json)

Verifies a real mainnet native P2WPKH output using the same transaction with two different `amount` values: the correct amount (5003 satoshis) passes with `btck_ScriptVerificationFlags_P2SH` + `btck_ScriptVerificationFlags_WITNESS` and with all pre-taproot flags; an incorrect amount (5002 satoshis) causes the witness commitment check to fail when `btck_ScriptVerificationFlags_WITNESS` is enforced, but passes with `btck_ScriptVerificationFlags_P2SH` only.

#### Script Verification — P2WSH
**File:** [`script_verify_p2wsh.json`](../testdata/script_verify_p2wsh.json)

Verifies a real mainnet native P2WSH output at input index 1 of a two-input transaction. A valid HTLC-style witness script passes with `btck_ScriptVerificationFlags_P2SH` + `btck_ScriptVerificationFlags_WITNESS` and with all pre-taproot flags. A transaction with a corrupted witness signature fails with `btck_ScriptVerificationFlags_WITNESS` enforced, but passes with `btck_ScriptVerificationFlags_P2SH` only.

#### Script Verification — P2TR Key-Path
**File:** [`script_verify_p2tr_keypath.json`](../testdata/script_verify_p2tr_keypath.json)

Verifies a real mainnet P2TR key-path spend. Requires one spent output to build precomputed transaction data for Taproot. A valid Schnorr signature passes with `btck_ScriptVerificationFlags_P2SH` + `btck_ScriptVerificationFlags_WITNESS` + `btck_ScriptVerificationFlags_TAPROOT` and with all flags. A corrupted Schnorr signature fails when `btck_ScriptVerificationFlags_TAPROOT` is enforced but passes with `btck_ScriptVerificationFlags_P2SH` + `btck_ScriptVerificationFlags_WITNESS` only.

#### Script Verification — P2TR Script-Path
**File:** [`script_verify_p2tr_scriptpath.json`](../testdata/script_verify_p2tr_scriptpath.json)

Verifies a real mainnet P2TR script-path spend at input index 1 of a two-input transaction. Requires two spent outputs (one per input) to build precomputed transaction data for Taproot. A valid script-path witness passes with `btck_ScriptVerificationFlags_P2SH` + `btck_ScriptVerificationFlags_WITNESS` + `btck_ScriptVerificationFlags_TAPROOT` and with all flags. A corrupted signature fails when `btck_ScriptVerificationFlags_TAPROOT` is enforced but passes with `btck_ScriptVerificationFlags_P2SH` + `btck_ScriptVerificationFlags_WITNESS` only.

### Script Verification Error Cases
**File:** [`script_verify_errors.json`](../testdata/script_verify_errors.json)

Test cases where the verification operation fails to determine validity of the script due to bad user input.

### Chain Operations
**File:** [`chain.json`](../testdata/chain.json)

Sets up blocks, checks chain state, and verifies that the chain tip changes as expected after a reorg scenario.

## Method Reference

Handler method definitions are maintained in the auto-generated [methods-spec.md](./methods-spec.md).
