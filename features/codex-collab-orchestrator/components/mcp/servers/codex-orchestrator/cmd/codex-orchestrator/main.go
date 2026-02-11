package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/cayde/llm/features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator/internal/orchestrator"
)

type rpcRequest struct {
	ID     any             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	ID     any       `json:"id"`
	Result any       `json:"result,omitempty"`
	Error  *rpcError `json:"error,omitempty"`
}

func main() {
	repoPath := flag.String("repo", ".", "repository root path")
	mode := flag.String("mode", "serve", "execution mode: serve|once")
	method := flag.String("method", "", "method for once mode")
	params := flag.String("params", "{}", "JSON params for once mode")
	flag.Parse()

	service, err := orchestrator.NewService(*repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize service: %v\n", err)
		os.Exit(1)
	}
	defer service.Close()

	switch strings.ToLower(*mode) {
	case "once":
		runOnce(service, *method, *params)
	case "serve":
		runServe(service)
	default:
		fmt.Fprintf(os.Stderr, "invalid mode: %s\n", *mode)
		os.Exit(2)
	}
}

func runOnce(service *orchestrator.Service, method, params string) {
	if strings.TrimSpace(method) == "" {
		fmt.Fprintln(os.Stderr, "--method is required when mode=once")
		os.Exit(2)
	}

	paramBytes := []byte(params)
	if strings.TrimSpace(params) == "" {
		paramBytes = []byte("{}")
	}

	result, err := service.Handle(context.Background(), method, paramBytes)
	response := rpcResponse{
		ID: "once",
	}
	if err != nil {
		response.Error = &rpcError{
			Code:    "method_error",
			Message: err.Error(),
		}
	} else {
		response.Result = result
	}

	encoded, marshalErr := json.MarshalIndent(response, "", "  ")
	if marshalErr != nil {
		fmt.Fprintf(os.Stderr, "failed to encode response: %v\n", marshalErr)
		os.Exit(1)
	}
	fmt.Println(string(encoded))

	if err != nil {
		os.Exit(1)
	}
}

func runServe(service *orchestrator.Service) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024), 4*1024*1024)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		response := handleLine(service, line)
		payload, err := json.Marshal(response)
		if err != nil {
			fallback := rpcResponse{
				ID: nil,
				Error: &rpcError{
					Code:    "encode_error",
					Message: err.Error(),
				},
			}
			payload, _ = json.Marshal(fallback)
		}

		_, _ = writer.Write(payload)
		_, _ = writer.WriteString("\n")
		_ = writer.Flush()
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "stdin read error: %v\n", err)
		os.Exit(1)
	}
}

func handleLine(service *orchestrator.Service, line string) rpcResponse {
	var request rpcRequest
	if err := json.Unmarshal([]byte(line), &request); err != nil {
		return rpcResponse{
			ID: nil,
			Error: &rpcError{
				Code:    "parse_error",
				Message: err.Error(),
			},
		}
	}

	if strings.TrimSpace(request.Method) == "" {
		return rpcResponse{
			ID: request.ID,
			Error: &rpcError{
				Code:    "invalid_request",
				Message: "method is required",
			},
		}
	}

	result, err := service.Handle(context.Background(), request.Method, request.Params)
	if err != nil {
		return rpcResponse{
			ID: request.ID,
			Error: &rpcError{
				Code:    "method_error",
				Message: err.Error(),
			},
		}
	}

	return rpcResponse{
		ID:     request.ID,
		Result: result,
	}
}
