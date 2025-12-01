package runner

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"time"
)

var (
	// ErrHandlerTimeout indicates the handler did not respond within the timeout
	ErrHandlerTimeout = errors.New("handler timeout")
	// ErrHandlerClosed indicates the handler closed stdout unexpectedly
	ErrHandlerClosed = errors.New("handler closed unexpectedly")
)

// HandlerConfig configures a handler process
type HandlerConfig struct {
	Path string
	Args []string
	Env  []string
}

// Handler manages a conformance handler process communicating via stdin/stdout
type Handler struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	stderr io.ReadCloser
}

// NewHandler spawns a new handler process with the given configuration
func NewHandler(cfg HandlerConfig) (*Handler, error) {
	cmd := exec.Command(cfg.Path, cfg.Args...)
	if cfg.Env != nil {
		cmd.Env = append(cmd.Environ(), cfg.Env...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start() automatically closes all pipes on failure, no manual cleanup needed
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start handler: %w", err)
	}

	return &Handler{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewScanner(stdout),
		stderr: stderr,
	}, nil
}

// SendLine writes a line to the handler's stdin
func (h *Handler) SendLine(line []byte) error {
	_, err := h.stdin.Write(append(line, '\n'))
	return err
}

// ReadLine reads a line from the handler's stdout with a 10-second timeout
func (h *Handler) ReadLine() ([]byte, error) {
	// Use a timeout for Scan() in case the handler hangs
	scanDone := make(chan bool, 1)
	go func() {
		scanDone <- h.stdout.Scan()
	}()

	var baseErr error
	select {
	case ok := <-scanDone:
		if ok {
			return h.stdout.Bytes(), nil
		}
		if err := h.stdout.Err(); err != nil {
			return nil, err
		}
		// EOF - handler closed stdout prematurely, fall through to kill and capture stderr
		baseErr = ErrHandlerClosed
	case <-time.After(10 * time.Second):
		// Timeout - handler didn't respond, fall through to kill and capture stderr
		baseErr = ErrHandlerTimeout
	}

	// Kill the process immediately to force stderr to close.
	// Without this, there's a rare scenario where stdout closes but stderr remains open,
	// causing io.ReadAll(h.stderr) below to block indefinitely waiting for stderr EOF.
	if h.cmd.Process != nil {
		h.cmd.Process.Kill()
	}

	// Capture stderr to provide diagnostic information when the handler fails.
	if stderrOut, err := io.ReadAll(h.stderr); err == nil && len(stderrOut) > 0 {
		return nil, fmt.Errorf("%w: %s", baseErr, bytes.TrimSpace(stderrOut))
	}
	return nil, baseErr
}

// Close closes stdin and waits for the handler to exit with a 5-second timeout.
// If the handler doesn't exit within the timeout, it is killed.
func (h *Handler) Close() {
	if h.stdin != nil {
		// Close stdin to signal the handler that we're done sending requests.
		// Per the handler specification, the handler should exit cleanly when stdin closes.
		h.stdin.Close()
	}
	if h.cmd != nil {
		// Wait for the handler to exit cleanly in response to stdin closing.
		// Wait() automatically closes all remaining pipes after the process exits.
		// Use a timeout in case the handler doesn't respect the protocol.
		done := make(chan error, 1)
		go func() {
			done <- h.cmd.Wait()
		}()

		select {
		case err := <-done:
			if err != nil {
				slog.Warn("Handler exit with error", "error", err)
			}
		case <-time.After(5 * time.Second):
			slog.Warn("Handler did not exit within a 5-second timeout, killing process")
			if h.cmd.Process != nil {
				h.cmd.Process.Kill()
				// Call Wait() again to let the process finish cleanup (closing pipes, etc.)
				// No timeout needed since Kill() should guarantee the process will exit
				h.cmd.Wait()
			}
		}
	}
}
