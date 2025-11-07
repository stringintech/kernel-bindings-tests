package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/stringintech/go-bitcoinkernel/kernel"
)

// handleScriptPubkeyVerify verifies a script against a transaction
func handleScriptPubkeyVerify(req Request) Response {
	var params struct {
		ScriptPubkeyHex string `json:"script_pubkey_hex"`
		Amount          int64  `json:"amount"`
		TxHex           string `json:"tx_hex"`
		InputIndex      uint   `json:"input_index"`
		Flags           string `json:"flags"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewErrorResponse(req.ID, ErrInvalidParams, "Failed to parse params: "+err.Error())
	}

	// Decode script pubkey
	var scriptBytes []byte
	var err error
	if params.ScriptPubkeyHex != "" {
		scriptBytes, err = hex.DecodeString(params.ScriptPubkeyHex)
		if err != nil {
			return NewErrorResponse(req.ID, ErrInvalidParams, "Invalid script pubkey hex: "+err.Error())
		}
	}

	// Decode transaction
	txBytes, err := hex.DecodeString(params.TxHex)
	if err != nil {
		return NewErrorResponse(req.ID, ErrInvalidParams, "Invalid transaction hex: "+err.Error())
	}

	// Create script pubkey
	scriptPubkey := kernel.NewScriptPubkey(scriptBytes)
	defer scriptPubkey.Destroy()

	// Create transaction
	tx, err := kernel.NewTransaction(txBytes)
	if err != nil {
		return NewErrorResponse(req.ID, ErrKernel, "Failed to create transaction: "+err.Error())
	}
	defer tx.Destroy()

	// Parse flags
	var flags kernel.ScriptFlags
	switch params.Flags {
	case "VERIFY_ALL_NO_TAPROOT":
		flags = kernel.ScriptFlags(kernel.ScriptFlagsVerifyAll &^ kernel.ScriptFlagsVerifyTaproot)
	case "VERIFY_ALL":
		flags = kernel.ScriptFlagsVerifyAll
	case "VERIFY_NONE":
		flags = kernel.ScriptFlagsVerifyNone
	default:
		return NewErrorResponse(req.ID, ErrInvalidParams, "Unknown flags: "+params.Flags)
	}

	// Verify script
	err = scriptPubkey.Verify(params.Amount, tx, nil, params.InputIndex, flags)

	if err != nil {
		// Check if it's a script verification error
		var scriptVerifyError *kernel.ScriptVerifyError
		if errors.As(err, &scriptVerifyError) {
			return NewErrorResponse(req.ID, ErrScriptVerify, "Script verification failed")
		}
		return NewErrorResponse(req.ID, ErrKernel, "Verification error: "+err.Error())
	}

	result := map[string]interface{}{
		"valid": true,
	}

	return NewSuccessResponse(req.ID, result)
}
