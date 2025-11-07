package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	// Create session state
	state := NewSessionState()
	defer state.Cleanup()

	// Read requests from stdin line by line
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()

		// Parse request
		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp := NewErrorResponse("", ErrInvalidRequest, "Failed to parse JSON: "+err.Error())
			sendResponse(resp)
			continue
		}

		resp := handleRequest(req, state)
		sendResponse(resp)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
		os.Exit(1)
	}
}

// sendResponse writes a response to stdout as JSON
func sendResponse(resp Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling response: %v\n", err)
		return
	}

	fmt.Println(string(data))
}
