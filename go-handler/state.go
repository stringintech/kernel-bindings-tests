package main

import (
	"os"

	"github.com/stringintech/go-bitcoinkernel/kernel"
)

// SessionState holds stateful resources for the test session
type SessionState struct {
	chainstateManager *kernel.ChainstateManager
	tempDir           string
}

// NewSessionState creates a new session state
func NewSessionState() *SessionState {
	return &SessionState{}
}

// Cleanup destroys all resources and removes temp directories
func (s *SessionState) Cleanup() {
	if s.chainstateManager != nil {
		s.chainstateManager.Destroy()
		s.chainstateManager = nil
	}
	if s.tempDir != "" {
		os.RemoveAll(s.tempDir)
		s.tempDir = ""
	}
}
