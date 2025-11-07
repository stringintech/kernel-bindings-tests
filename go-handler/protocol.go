package main

import (
	"encoding/json"
)

type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type Response struct {
	ID     string      `json:"id"`
	Result interface{} `json:"result,omitempty"`
	Error  *ErrorObj   `json:"error,omitempty"`
}

type ErrorObj struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Standard error codes
const (
	ErrInvalidRequest = "INVALID_REQUEST"
	ErrMethodNotFound = "METHOD_NOT_FOUND"
	ErrInvalidParams  = "INVALID_PARAMS"
	ErrKernel         = "KERNEL_ERROR"
	ErrScriptVerify   = "SCRIPT_VERIFY_ERROR"
	ErrInternalError  = "INTERNAL_ERROR"
)

// NewErrorResponse creates an error response
func NewErrorResponse(id, code, message string) Response {
	return Response{
		ID: id,
		Error: &ErrorObj{
			Code:    code,
			Message: message,
		},
	}
}

// NewSuccessResponse creates a success response
func NewSuccessResponse(id string, result interface{}) Response {
	return Response{
		ID:     id,
		Result: result,
	}
}
