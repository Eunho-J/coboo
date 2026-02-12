package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/cayde/llm/features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator/internal/orchestrator"
)

const mcpProtocolVersion = "2024-11-05"

type toolGroup struct {
	Name        string
	Description string
	Methods     []string
}

var toolGroups = []toolGroup{
	{
		Name:        "orch_session",
		Description: "Session and workspace initialization management",
		Methods:     []string{"workspace.init", "session.open", "session.heartbeat", "session.close", "session.cleanup", "session.list", "session.context"},
	},
	{
		Name:        "orch_task",
		Description: "Task lifecycle, case execution, and resume management",
		Methods:     []string{"task.create", "task.list", "task.get", "case.begin", "step.check", "case.complete", "resume.next", "resume.candidates.list", "resume.candidates.attach"},
	},
	{
		Name:        "orch_graph",
		Description: "Dependency graph, checklists, and snapshots",
		Methods:     []string{"graph.node.create", "graph.node.list", "graph.edge.create", "graph.checklist.upsert", "graph.snapshot.create"},
	},
	{
		Name:        "orch_workspace",
		Description: "Worktree scheduling, creation, merging, and lock management",
		Methods:     []string{"scheduler.decide_worktree", "worktree.create", "worktree.list", "worktree.spawn", "worktree.merge_to_parent", "lock.acquire", "lock.heartbeat", "lock.release"},
	},
	{
		Name:        "orch_thread",
		Description: "Child thread spawning, directives, and lifecycle control",
		Methods:     []string{"thread.child.spawn", "thread.child.directive", "thread.child.list", "thread.child.interrupt", "thread.child.stop", "thread.child.status", "thread.child.wait_status", "thread.attach_info"},
	},
	{
		Name:        "orch_lifecycle",
		Description: "Current work reference tracking and acknowledgement",
		Methods:     []string{"work.current_ref", "work.current_ref.ack"},
	},
	{
		Name:        "orch_merge",
		Description: "Branch merge requests, reviews, and main-line merge operations",
		Methods:     []string{"merge.request", "merge.review_context", "merge.review.request_auto", "merge.review.thread_status", "merge.main.request", "merge.main.next", "merge.main.status", "merge.main.acquire_lock", "merge.main.release_lock"},
	},
	{
		Name:        "orch_inbox",
		Description: "Thread-to-thread messaging: send, receive, and deliver messages",
		Methods:     []string{"inbox.send", "inbox.pending", "inbox.list", "inbox.deliver"},
	},
	{
		Name:        "orch_system",
		Description: "Runtime, mirror, and plan management utilities",
		Methods:     []string{"runtime.tmux.ensure", "runtime.bundle.info", "mirror.status", "mirror.refresh", "plan.bootstrap", "plan.slice.generate", "plan.slice.replan", "plan.rollup.preview", "plan.rollup.submit", "plan.rollup.approve", "plan.rollup.reject"},
	},
}

// toolGroupMethodIndex maps each orch_* tool name to a set of its allowed methods.
var toolGroupMethodIndex map[string]map[string]bool

func init() {
	toolGroupMethodIndex = make(map[string]map[string]bool, len(toolGroups))
	for _, g := range toolGroups {
		methodSet := make(map[string]bool, len(g.Methods))
		for _, m := range g.Methods {
			methodSet[m] = true
		}
		toolGroupMethodIndex[g.Name] = methodSet
	}
}

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

type messageFormat int

const (
	messageFormatJSONLine messageFormat = iota
	messageFormatFramed
)

func main() {
	repoPath := flag.String("repo", ".", "repository root path")
	mode := flag.String("mode", "serve", "execution mode: serve|once")
	transport := flag.String("transport", "stdio", "transport mode: stdio|http")
	port := flag.Int("port", 8090, "HTTP port (only used with --transport http)")
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
		switch strings.ToLower(*transport) {
		case "http":
			runHTTPServe(service, *port)
		default:
			runServe(service)
		}
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
		payload, format, err := reader.ReadPayload()
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
		if err := writer.WritePayload(responsePayload, format); err != nil {
			fmt.Fprintf(os.Stderr, "mcp write error: %v\n", err)
			os.Exit(1)
		}
	}
}

func runHTTPServe(service *orchestrator.Service, port int) {
	mux := http.NewServeMux()

	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"read body failed"}`, http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		responsePayload, shouldRespond := handleMCPPayload(service, body)

		w.Header().Set("Content-Type", "application/json")
		if !shouldRespond {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Write(responsePayload)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","transport":"http"}`))
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("codex-orchestrator HTTP server listening on %s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

func (fr framedReader) ReadPayload() ([]byte, messageFormat, error) {
	for {
		line, err := fr.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && line == "" {
				return nil, messageFormatJSONLine, io.EOF
			}
			return nil, messageFormatJSONLine, err
		}

		line = strings.TrimRight(line, "\r\n")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// MCP stdio in Codex commonly uses JSONL (one JSON-RPC object per line).
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return []byte(trimmed), messageFormatJSONLine, nil
		}

		// Also accept LSP-style Content-Length framing for compatibility.
		if !strings.HasPrefix(strings.ToLower(trimmed), "content-length:") {
			continue
		}

		value := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(trimmed), "content-length:"))
		contentLength, convErr := strconv.Atoi(value)
		if convErr != nil || contentLength < 0 {
			return nil, messageFormatFramed, fmt.Errorf("invalid Content-Length: %q", value)
		}

		for {
			headerLine, headerErr := fr.reader.ReadString('\n')
			if headerErr != nil {
				return nil, messageFormatFramed, headerErr
			}
			headerLine = strings.TrimRight(headerLine, "\r\n")
			if strings.TrimSpace(headerLine) == "" {
				break
			}
		}

		payload := make([]byte, contentLength)
		if _, err := io.ReadFull(fr.reader, payload); err != nil {
			return nil, messageFormatFramed, err
		}
		return payload, messageFormatFramed, nil
	}
}

func (fw framedWriter) WritePayload(payload []byte, format messageFormat) error {
	if format == messageFormatFramed {
		if _, err := fmt.Fprintf(fw.writer, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
			return err
		}
		if _, err := fw.writer.Write(payload); err != nil {
			return err
		}
		return fw.writer.Flush()
	}

	if _, err := fw.writer.Write(payload); err != nil {
		return err
	}
	if _, err := fw.writer.WriteString("\n"); err != nil {
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
			"tools": buildToolsList(),
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

	if toolGroupMethodIndex[input.Name] != nil {
		return handleGroupToolCall(service, input.Name, input.Arguments)
	}
	return toolErrorResult(fmt.Sprintf("unknown tool: %s", input.Name)), nil
}

func buildToolsList() []map[string]any {
	tools := make([]map[string]any, 0, len(toolGroups))

	// Domain-specific tool groups.
	for _, g := range toolGroups {
		enumValues := make([]any, len(g.Methods))
		for i, m := range g.Methods {
			enumValues[i] = m
		}
		tools = append(tools, map[string]any{
			"name":        g.Name,
			"description": g.Description,
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"method": map[string]any{
						"type":        "string",
						"description": "Backend method name.",
						"enum":        enumValues,
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
		})
	}

	return tools
}

func handleGroupToolCall(service *orchestrator.Service, toolName string, arguments json.RawMessage) (map[string]any, error) {
	var args orchestratorCallArguments
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid %s arguments: %w", toolName, err)
	}

	method := strings.TrimSpace(args.Method)
	if method == "" {
		return nil, fmt.Errorf("%s requires arguments.method", toolName)
	}

	allowedMethods := toolGroupMethodIndex[toolName]
	if !allowedMethods[method] {
		return toolErrorResult(fmt.Sprintf("method '%s' is not valid for tool '%s'", method, toolName)), nil
	}

	params := args.Params
	if len(bytesTrimSpace(params)) == 0 {
		params = json.RawMessage(`{}`)
	}

	result, err := service.Handle(context.Background(), method, params)
	if err != nil {
		return toolErrorResult(err.Error()), nil
	}
	return toolSuccessResult(result)
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
