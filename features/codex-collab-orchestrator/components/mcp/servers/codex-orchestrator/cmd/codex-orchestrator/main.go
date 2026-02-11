package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/cayde/llm/features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator/internal/orchestrator"
)

const mcpProtocolVersion = "2024-11-05"

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type jsonRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id"`
	Result  any           `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
}

type mcpToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type orchestratorCallArguments struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type framedReader struct {
	reader *bufio.Reader
}

type framedWriter struct {
	writer *bufio.Writer
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
	if len(bytesTrimSpace(paramBytes)) == 0 {
		paramBytes = []byte("{}")
	}

	result, err := service.Handle(context.Background(), method, paramBytes)
	response := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      "once",
	}
	if err != nil {
		response.Error = &jsonRPCError{
			Code:    -32000,
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
	reader := framedReader{reader: bufio.NewReader(os.Stdin)}
	writer := framedWriter{writer: bufio.NewWriter(os.Stdout)}

	for {
		payload, err := reader.ReadPayload()
		if err != nil {
			if err == io.EOF {
				return
			}
			fmt.Fprintf(os.Stderr, "mcp read error: %v\n", err)
			os.Exit(1)
		}

		responsePayload, shouldRespond := handleMCPPayload(service, payload)
		if !shouldRespond {
			continue
		}
		if err := writer.WritePayload(responsePayload); err != nil {
			fmt.Fprintf(os.Stderr, "mcp write error: %v\n", err)
			os.Exit(1)
		}
	}
}

func (fr framedReader) ReadPayload() ([]byte, error) {
	contentLength := -1
	seenAnyHeader := false

	for {
		line, err := fr.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && !seenAnyHeader && line == "" {
				return nil, io.EOF
			}
			return nil, err
		}
		seenAnyHeader = true
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(parts[0]), "Content-Length") {
			value := strings.TrimSpace(parts[1])
			length, convErr := strconv.Atoi(value)
			if convErr != nil || length < 0 {
				return nil, fmt.Errorf("invalid Content-Length: %q", value)
			}
			contentLength = length
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(fr.reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (fw framedWriter) WritePayload(payload []byte) error {
	if _, err := fmt.Fprintf(fw.writer, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	if _, err := fw.writer.Write(payload); err != nil {
		return err
	}
	return fw.writer.Flush()
}

func handleMCPPayload(service *orchestrator.Service, payload []byte) ([]byte, bool) {
	var request jsonRPCRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return mustMarshalResponse(jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error: &jsonRPCError{
				Code:    -32700,
				Message: "invalid JSON-RPC request",
			},
		}), true
	}

	if strings.TrimSpace(request.Method) == "" {
		return mustMarshalResponse(jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      request.ID,
			Error: &jsonRPCError{
				Code:    -32600,
				Message: "method is required",
			},
		}), true
	}

	// Notifications have no id, so no response should be written.
	if request.ID == nil {
		return nil, false
	}

	response := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      request.ID,
	}

	switch request.Method {
	case "initialize":
		response.Result = map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"serverInfo": map[string]any{
				"name":    "codex-orchestrator",
				"version": "0.1.0",
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		}
	case "ping":
		response.Result = map[string]any{}
	case "tools/list":
		response.Result = map[string]any{
			"tools": []map[string]any{
				{
					"name":        "orchestrator.call",
					"description": "Call one codex-orchestrator backend method by name.",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"method": map[string]any{
								"type":        "string",
								"description": "Backend method name (e.g. workspace.init, session.open).",
							},
							"params": map[string]any{
								"type":        "object",
								"description": "Method params object.",
								"default":     map[string]any{},
							},
						},
						"required":             []string{"method"},
						"additionalProperties": false,
					},
				},
			},
		}
	case "tools/call":
		result, err := handleToolCall(service, request.Params)
		if err != nil {
			response.Error = &jsonRPCError{
				Code:    -32000,
				Message: err.Error(),
			}
		} else {
			response.Result = result
		}
	default:
		response.Error = &jsonRPCError{
			Code:    -32601,
			Message: fmt.Sprintf("method not found: %s", request.Method),
		}
	}

	return mustMarshalResponse(response), true
}

func handleToolCall(service *orchestrator.Service, rawParams json.RawMessage) (map[string]any, error) {
	if len(bytesTrimSpace(rawParams)) == 0 {
		return nil, fmt.Errorf("tools/call params are required")
	}

	var input mcpToolCallParams
	if err := json.Unmarshal(rawParams, &input); err != nil {
		return nil, fmt.Errorf("invalid tools/call params: %w", err)
	}

	switch input.Name {
	case "orchestrator.call":
		var args orchestratorCallArguments
		if err := json.Unmarshal(input.Arguments, &args); err != nil {
			return nil, fmt.Errorf("invalid orchestrator.call arguments: %w", err)
		}
		if strings.TrimSpace(args.Method) == "" {
			return nil, fmt.Errorf("orchestrator.call requires arguments.method")
		}

		params := args.Params
		if len(bytesTrimSpace(params)) == 0 {
			params = json.RawMessage(`{}`)
		}

		result, err := service.Handle(context.Background(), args.Method, params)
		if err != nil {
			return toolErrorResult(err.Error()), nil
		}
		return toolSuccessResult(result)
	default:
		return toolErrorResult(fmt.Sprintf("unknown tool: %s", input.Name)), nil
	}
}

func toolSuccessResult(result any) (map[string]any, error) {
	text, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize tool result: %w", err)
	}
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": string(text),
			},
		},
		"structuredContent": result,
	}, nil
}

func toolErrorResult(message string) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": message,
			},
		},
		"isError": true,
	}
}

func mustMarshalResponse(response jsonRPCResponse) []byte {
	payload, err := json.Marshal(response)
	if err != nil {
		fallback := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error: &jsonRPCError{
				Code:    -32603,
				Message: "failed to encode response",
			},
		}
		payload, _ = json.Marshal(fallback)
	}
	return payload
}

func bytesTrimSpace(raw []byte) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}
