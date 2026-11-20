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
{"id": "1", "result": {"ref": "$ctx1"}}
// Handler action: registry["$ctx1"] = created_context_ptr
```

**Using Objects**: When a parameter is marked as `(reference, required)`, the runner passes a reference type object and the handler extracts the reference name to look it up:

```json
// Request
{"id": "2", "method": "btck_chainstate_manager_create", "params": {"context": {"ref": "$ctx1"}}, "ref": "$csm1"}
// Response
{"id": "2", "result": {"ref": "$csm1"}}
// Handler action: Extract ref from params.context, look up registry["$ctx1"], create manager, store as registry["$csm1"]
```

**Implementation**: Handlers must maintain a registry (map of reference names to object pointers) throughout their lifetime. Objects remain alive until explicitly destroyed or handler exit.

## Callback Interfaces

Some kernel operations trigger callbacks — notification events and validation interface events — that fire synchronously during the operation. The protocol requires handlers to implement two callback interfaces as queueing objects: each maintains an internal invocation queue, records every callback firing into it, and exposes that queue via a drain method. This gives the runner a way to assert which callbacks fired, in what order, and what values they carried.

### Creation and Wiring

Callback interface objects are binding-level objects — not direct C kernel API handles, but wired into the kernel at context creation time. The protocol introduces dedicated methods for creating them — `notification_callbacks_create` and `validation_interface_callbacks_create` — deliberately omitting the `btck_` prefix to make clear they have no direct C API counterpart. They follow the same ref/registry pattern as kernel objects. Interface refs are passed as optional params to `btck_context_create` to wire them in:

The `callbacks` param is required and must list at least one callback name. The interface only queues invocations for the listed callbacks; any unlisted callback fires at the C level but is silently discarded.

```json
// Request
{"id": "1", "method": "notification_callbacks_create", "params": {"callbacks": ["btck_NotifyBlockTip"]}, "ref": "$notif"}
// Response
{"id": "1", "result": {"ref": "$notif"}}

// Request
{"id": "2", "method": "btck_context_create", "params": {"chain_parameters": {...}, "notifications": {"ref": "$notif"}}, "ref": "$ctx"}
// Response
{"id": "2", "result": {"ref": "$ctx"}}
```

Neither interface has a matching destroy method. Both are cleaned up implicitly when the chainstate manager associated with the wired context is destroyed.

### Invocation Recording

When a callback fires, the interface implementation must, before the callback returns, append an invocation record to its queue, preserving firing order. The record identifies the callback that fired and carries its arguments, which may be primitives (e.g. an enum like `btck_SynchronizationState`, or a number like `verification_progress`) or objects (e.g. a `btck_BlockTreeEntry`). At this point object arguments sit on the queue and are not yet directly referenceable by the runner — they only become referenceable later, when [drain](#drain) registers them in the registry.

What the record carries for each object argument depends on how the kernel passes it. For arguments the kernel passes as owned copies, carry the copy directly. For arguments passed as views (pointers or references into kernel-owned memory), the handler must judge whether the underlying memory will remain valid long enough — at least until the last request that may reference this ref: if it will (e.g. a `btck_BlockTreeEntry` view that remains valid for the chainstate manager's lifetime), carry the view directly; if it will not — because the view points to a stack-local or other short-lived storage — copy the object before the callback returns and carry the copy instead. This decision is made at callback time; drain has no visibility into argument lifetimes.

### Drain

Drain is what gives the runner access to the queued invocations: the firing order, each callback's primitive argument values, and a way to reference its object arguments in follow-up requests. The drain methods (`notification_callbacks_drain`, `validation_callbacks_drain`) are protocol-level methods with no C API counterpart. Each takes an interface ref, registers every queued object under a deterministic ref name, flushes the queue, and returns the invocation records in firing order with each object argument resolved to `{"ref": "<ref-name>"}`. Until drain is called, callback-produced refs are not in the registry, so the runner must call drain before referencing any callback-produced object in a follow-up request.

The registered ref name must follow this pattern, so the runner can predict it and use it in follow-up assertions (computing it at callback time is often more straightforward, but the spec only requires that the object end up registered under this name on drain):

```
$<interface_ref>_<n>_<callback_typedef>_<arg_name>
```

- `<interface_ref>`: ref name of the interface object, without the leading `$`
- `<n>`: ordinal position of this invocation in the queue since the last drain, counting across all callback types, starting at 1
- `<callback_typedef>`: exact C typedef name from `bitcoinkernel.h`
- `<arg_name>`: C parameter name of the object argument

Examples: `$notif_1_btck_NotifyBlockTip_entry`, `$vi_1_btck_ValidationInterfaceBlockChecked_block`, `$vi_2_btck_ValidationInterfaceBlockConnected_entry`.

Both callback families are synchronous — all callbacks triggered during a kernel operation complete before the operation returns — so a drain issued after a kernel operation (e.g. `btck_chainstate_manager_process_block`) will always see the complete set of records for that call. An empty array means no callbacks fired since the last drain.

```json
// Request
{"id": "3", "method": "notification_callbacks_drain", "params": {"interface": {"ref": "$notif"}}}
// Response
{"id": "3", "result": [{"callback": "btck_NotifyBlockTip", "state": "btck_SynchronizationState_POST_INIT", "entry": {"ref": "$notif_1_btck_NotifyBlockTip_entry"}, "verification_progress": 1.0}]}
```

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

Parses raw block headers, rejects short inputs, verifies serialization, field getters, and exercises copy and destroy behavior.

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

### Callback Interfaces
**File:** [`callbacks.json`](../testdata/callbacks.json)

Registers both a notification callbacks interface and a validation interface, wires both into a context, drains init-time invocations after chainstate manager creation, processes a block, drains both interfaces, and asserts on notification entry height, validation mode, and validation entry height.

## Method Reference

Handler method definitions are maintained in the auto-generated [methods-spec.md](./methods-spec.md).
