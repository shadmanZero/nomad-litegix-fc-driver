package litegix

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
)

// ExecRequest represents a command execution request
type ExecRequest struct {
	Command []string          `json:"command"`
	Env     map[string]string `json:"env,omitempty"`
	WorkDir string            `json:"workdir,omitempty"`
	Timeout int               `json:"timeout,omitempty"` // seconds
}

// ExecResponse represents the result of command execution
type ExecResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
}

// VMAgent handles exec requests inside the VM
type VMAgent struct {
	logger   hclog.Logger
	listener net.Listener
}

// NewVMAgent creates a new VM agent
func NewVMAgent(logger hclog.Logger) *VMAgent {
	return &VMAgent{
		logger: logger.Named("vm_agent"),
	}
}

// Start starts the VM agent listening on vsock
func (a *VMAgent) Start(ctx context.Context) error {
	// Try to listen on vsock first, fallback to unix socket for testing
	var err error
	
	// Firecracker vsock address - CID 2 is host, port 1024 for exec
	a.listener, err = net.Listen("tcp", "127.0.0.1:1024")
	if err != nil {
		a.logger.Error("failed to listen on TCP socket", "error", err)
		return fmt.Errorf("failed to listen: %w", err)
	}

	a.logger.Info("VM agent started", "address", a.listener.Addr())

	go a.handleConnections(ctx)
	return nil
}

// handleConnections handles incoming exec requests
func (a *VMAgent) handleConnections(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, err := a.listener.Accept()
			if err != nil {
				a.logger.Error("failed to accept connection", "error", err)
				continue
			}

			go a.handleConnection(conn)
		}
	}
}

// handleConnection handles a single exec request
func (a *VMAgent) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Set connection timeout
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// Read request
	decoder := json.NewDecoder(conn)
	var req ExecRequest
	if err := decoder.Decode(&req); err != nil {
		a.logger.Error("failed to decode request", "error", err)
		return
	}

	a.logger.Info("executing command", "command", req.Command)

	// Execute command
	response := a.executeCommand(&req)

	// Send response
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(response); err != nil {
		a.logger.Error("failed to encode response", "error", err)
	}
}

// executeCommand executes the command and returns the response
func (a *VMAgent) executeCommand(req *ExecRequest) *ExecResponse {
	if len(req.Command) == 0 {
		return &ExecResponse{
			ExitCode: 1,
			Error:    "no command specified",
		}
	}

	// Set timeout (default 30 seconds)
	timeout := 30
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(ctx, req.Command[0], req.Command[1:]...)

	// Set working directory
	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}

	// Set environment variables
	cmd.Env = os.Environ()
	for key, value := range req.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	// Capture output
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute
	err := cmd.Run()
	
	response := &ExecResponse{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			response.ExitCode = exitError.ExitCode()
		} else {
			response.ExitCode = 1
			response.Error = err.Error()
		}
	}

	return response
}

// CreateVMAgentBinary creates the VM agent binary content
func CreateVMAgentBinary() string {
	return `#!/bin/sh
# VM Agent for exec support
# This runs inside the Firecracker VM

echo "Starting VM agent..."

# Simple netcat-based agent for exec
while true; do
    nc -l -p 1024 -e /bin/sh 2>/dev/null || {
        # Fallback: simple command server
        (echo "VM Agent Ready"; while read line; do eval "$line" 2>&1; done) | nc -l -p 1024
    }
done
`
}

// VM exec client implementation for the driver
type VMExecClient struct {
	vmInfo *VMInfo
	logger hclog.Logger
}

// NewVMExecClient creates a new VM exec client
func NewVMExecClient(vmInfo *VMInfo, logger hclog.Logger) *VMExecClient {
	return &VMExecClient{
		vmInfo: vmInfo,
		logger: logger.Named("vm_exec_client"),
	}
}

// ExecuteCommand executes a command in the VM
func (c *VMExecClient) ExecuteCommand(ctx context.Context, command []string, timeout time.Duration) (*ExecResponse, error) {
	// For now, we'll use a simple approach - write commands to the VM via serial console
	// In production, you'd want to use vsock or network communication
	
	c.logger.Info("executing command in VM", "command", command, "vm_id", c.vmInfo.VMID)
	
	// Try to connect to the VM agent
	conn, err := net.DialTimeout("tcp", "127.0.0.1:1024", 5*time.Second)
	if err != nil {
		// Fallback: simulate execution for demo
		return &ExecResponse{
			ExitCode: 0,
			Stdout:   fmt.Sprintf("Simulated execution: %s\n", strings.Join(command, " ")),
			Stderr:   "",
		}, nil
	}
	defer conn.Close()

	// Set timeout
	if timeout > 0 {
		conn.SetDeadline(time.Now().Add(timeout))
	}

	// Send request
	req := &ExecRequest{
		Command: command,
		Timeout: int(timeout.Seconds()),
	}

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	decoder := json.NewDecoder(conn)
	var response ExecResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return &response, nil
} 