package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
)

const protocolVersion = "2024-11-05"

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Server struct {
	info    ServerInfo
	logger  *slog.Logger
	mu      sync.RWMutex
	tools   map[string]Tool
	writeMu sync.Mutex
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Handler     ToolHandler    `json:"-"`
}

type ToolHandler func(ctx context.Context, args json.RawMessage) (any, error)

// Content is a single MCP content block.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolResult is a complete tool result. Handlers may return one directly to
// control the content blocks and isError flag; any other return value is
// JSON-encoded into a single text block.
type ToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// TextResult builds a successful single-block text result.
func TextResult(text string) ToolResult {
	return ToolResult{Content: []Content{{Type: "text", Text: text}}}
}

// ErrorResult builds an error result with an optional "Possible solutions"
// block, mirroring the Coding-Solo reference response shape.
func ErrorResult(message string, possibleSolutions ...string) ToolResult {
	content := []Content{{Type: "text", Text: message}}
	if len(possibleSolutions) > 0 {
		var b strings.Builder
		b.WriteString("Possible solutions:\n- ")
		b.WriteString(strings.Join(possibleSolutions, "\n- "))
		content = append(content, Content{Type: "text", Text: b.String()})
	}
	return ToolResult{Content: content, IsError: true}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func NewServer(info ServerInfo, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		info:   info,
		logger: logger,
		tools:  make(map[string]Tool),
	}
}

func (s *Server) RegisterTool(t Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[t.Name] = t
}

func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context: %w", err)
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			s.logger.Warn("malformed request", "error", err)
			continue
		}
		resp, ok := s.handle(ctx, req)
		if !ok {
			continue
		}
		if err := s.write(w, resp); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	return nil
}

func (s *Server) handle(ctx context.Context, req request) (response, bool) {
	// A request without an id is a JSON-RPC notification: process it but never
	// send a response, even for unknown methods.
	if len(req.ID) == 0 {
		return response{}, false
	}

	resp := response{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": s.info,
		}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": s.listTools()}
	case "tools/call":
		result, err := s.callTool(ctx, req.Params)
		if err != nil {
			resp.Error = &rpcError{Code: -32602, Message: err.Error()}
		} else {
			resp.Result = result
		}
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}

	return resp, true
}

func (s *Server) listTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tools := make([]Tool, 0, len(s.tools))
	for _, t := range s.tools {
		tools = append(tools, Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
	return tools
}

type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) callTool(ctx context.Context, params json.RawMessage) (any, error) {
	var p callParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	s.mu.RLock()
	tool, ok := s.tools[p.Name]
	s.mu.RUnlock()
	if !ok {
		return nil, errors.New("unknown tool: " + p.Name)
	}
	out, err := tool.Handler(ctx, p.Arguments)
	if err != nil {
		return ErrorResult(err.Error()), nil
	}
	switch result := out.(type) {
	case ToolResult:
		return result, nil
	case *ToolResult:
		return *result, nil
	}
	text, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encode tool result: %w", err)
	}
	return TextResult(string(text)), nil
}

func (s *Server) write(w io.Writer, resp response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	data = append(data, '\n')
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write response: %w", err)
	}
	return nil
}
