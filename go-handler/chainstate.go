package main

import (
	"encoding/hex"
	"encoding/json"
	"os"

	"github.com/stringintech/go-bitcoinkernel/kernel"
)

// handleChainstateSetup initializes a chainstate and imports blocks
func handleChainstateSetup(req Request, state *SessionState) Response {
	var params struct {
		ChainType string   `json:"chain_type"`
		BlocksHex []string `json:"blocks_hex"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewErrorResponse(req.ID, ErrInvalidParams, "Failed to parse params: "+err.Error())
	}

	// Clean up any existing state
	state.Cleanup()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "conformance_test_")
	if err != nil {
		return NewErrorResponse(req.ID, ErrInternalError, "Failed to create temp dir: "+err.Error())
	}
	state.tempDir = tempDir

	// Parse chain type
	var chainType kernel.ChainType
	switch params.ChainType {
	case "mainnet":
		chainType = kernel.ChainTypeMainnet
	case "testnet":
		chainType = kernel.ChainTypeTestnet
	case "testnet4":
		chainType = kernel.ChainTypeTestnet4
	case "signet":
		chainType = kernel.ChainTypeSignet
	case "regtest":
		chainType = kernel.ChainTypeRegtest
	default:
		state.Cleanup()
		return NewErrorResponse(req.ID, ErrInvalidParams, "Unknown chain type: "+params.ChainType)
	}

	// Create chain parameters
	chainParams, err := kernel.NewChainParameters(chainType)
	if err != nil {
		state.Cleanup()
		return NewErrorResponse(req.ID, ErrKernel, "Failed to create chain parameters: "+err.Error())
	}
	defer chainParams.Destroy()

	// Create context options
	contextOpts := kernel.NewContextOptions()
	contextOpts.SetChainParams(chainParams)

	// Create context
	ctx, err := kernel.NewContext(contextOpts)
	if err != nil {
		state.Cleanup()
		return NewErrorResponse(req.ID, ErrKernel, "Failed to create context: "+err.Error())
	}
	defer ctx.Destroy()

	// Create chainstate manager options
	opts, err := kernel.NewChainstateManagerOptions(ctx, state.tempDir, state.tempDir+"/blocks")
	if err != nil {
		state.Cleanup()
		return NewErrorResponse(req.ID, ErrKernel, "Failed to create options: "+err.Error())
	}
	defer opts.Destroy()

	// Configure for in-memory testing
	opts.SetWorkerThreads(1)
	opts.UpdateBlockTreeDBInMemory(true)
	opts.UpdateChainstateDBInMemory(true)
	if err := opts.SetWipeDBs(true, true); err != nil {
		state.Cleanup()
		return NewErrorResponse(req.ID, ErrKernel, "Failed to set wipe DBs: "+err.Error())
	}

	// Create chainstate manager
	manager, err := kernel.NewChainstateManager(opts)
	if err != nil {
		state.Cleanup()
		return NewErrorResponse(req.ID, ErrKernel, "Failed to create manager: "+err.Error())
	}
	state.chainstateManager = manager

	// Initialize empty databases
	if err := manager.ImportBlocks(nil); err != nil {
		state.Cleanup()
		return NewErrorResponse(req.ID, ErrKernel, "Failed to initialize: "+err.Error())
	}

	// Process blocks
	blocksImported := 0
	for i, blockHex := range params.BlocksHex {
		blockBytes, err := hex.DecodeString(blockHex)
		if err != nil {
			return NewErrorResponse(req.ID, ErrInvalidParams, "Invalid block hex at index "+string(rune(i))+": "+err.Error())
		}

		block, err := kernel.NewBlock(blockBytes)
		if err != nil {
			return NewErrorResponse(req.ID, ErrKernel, "Failed to create block at index "+string(rune(i))+": "+err.Error())
		}

		ok, duplicate := manager.ProcessBlock(block)
		block.Destroy()

		if !ok || duplicate {
			return NewErrorResponse(req.ID, ErrKernel, "Failed to process block at index "+string(rune(i)))
		}

		blocksImported++
	}

	// Get tip height
	chain := manager.GetActiveChain()
	tipHeight := chain.GetHeight()

	result := map[string]interface{}{
		"blocks_imported": blocksImported,
		"tip_height":      tipHeight,
	}

	return NewSuccessResponse(req.ID, result)
}

// handleChainstateReadBlock reads a block by height or tip
func handleChainstateReadBlock(req Request, state *SessionState) Response {
	if state.chainstateManager == nil {
		return NewErrorResponse(req.ID, ErrInternalError, "Chainstate not initialized")
	}

	var params struct {
		Height *int32 `json:"height,omitempty"`
		Tip    *bool  `json:"tip,omitempty"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewErrorResponse(req.ID, ErrInvalidParams, "Failed to parse params: "+err.Error())
	}

	chain := state.chainstateManager.GetActiveChain()

	var blockIndex *kernel.BlockTreeEntry
	if params.Tip != nil && *params.Tip {
		blockIndex = chain.GetTip()
	} else if params.Height != nil {
		blockIndex = chain.GetByHeight(*params.Height)
	} else {
		return NewErrorResponse(req.ID, ErrInvalidParams, "Must specify either height or tip")
	}

	if blockIndex == nil {
		return NewErrorResponse(req.ID, ErrKernel, "Block not found")
	}

	block, err := state.chainstateManager.ReadBlock(blockIndex)
	if err != nil {
		return NewErrorResponse(req.ID, ErrKernel, "Failed to read block: "+err.Error())
	}
	defer block.Destroy()

	blockBytes, err := block.Bytes()
	if err != nil {
		return NewErrorResponse(req.ID, ErrKernel, "Failed to serialize block: "+err.Error())
	}

	result := map[string]interface{}{
		"block_hex": hex.EncodeToString(blockBytes),
		"height":    blockIndex.Height(),
	}

	return NewSuccessResponse(req.ID, result)
}

// handleChainstateTeardown cleans up chainstate resources
func handleChainstateTeardown(req Request, state *SessionState) Response {
	state.Cleanup()
	result := map[string]interface{}{
		"success": true,
	}
	return NewSuccessResponse(req.ID, result)
}
