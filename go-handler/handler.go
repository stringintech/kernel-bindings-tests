package main

import "fmt"

// handleRequest dispatches a request to the appropriate handler
func handleRequest(req Request, state *SessionState) (resp Response) {
	// Recover from panics and return error response
	defer func() {
		if r := recover(); r != nil {
			resp = NewErrorResponse(req.ID, ErrInternalError, fmt.Sprintf("Internal error (panic): %v", r))
		}
	}()

	switch req.Method {
	// ScriptPubkey
	case "script_pubkey.verify":
		return handleScriptPubkeyVerify(req)

	// Chainstate
	case "chainstate.setup":
		return handleChainstateSetup(req, state)
	case "chainstate.read_block":
		return handleChainstateReadBlock(req, state)
	case "chainstate.teardown":
		return handleChainstateTeardown(req, state)

	default:
		return NewErrorResponse(req.ID, ErrMethodNotFound, "Unknown method: "+req.Method)
	}
}
