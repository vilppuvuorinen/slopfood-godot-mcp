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

// latestProtocolVersion is the newest revision this server speaks. Starting
// with 2026-07-28 the protocol is stateless: clients learn the server's
// identity and capabilities from server/discover and carry their protocol
// version and capabilities in each request's _meta instead of running the
// legacy initialize handshake.
const latestProtocolVersion = "2026-07-28"

// latestLegacyProtocolVersion is the newest revision that still uses the
// initialize handshake. It caps initialize negotiation so a legacy client is
// never handed a stateless revision, which is only reachable via discover.
const latestLegacyProtocolVersion = "2025-11-25"

// protocolVersion20260728 is the first stateless revision (SEP-2575). Protocol
// versions are date strings, so they compare meaningfully as strings.
const protocolVersion20260728 = "2026-07-28"

// supportedProtocolVersions lists every revision this server can negotiate,
// newest first. 2024-11-05 is kept so older clients still connect.
var supportedProtocolVersions = []string{
	"2026-07-28",
	"2025-11-25",
	"2025-06-18",
	"2025-03-26",
	"2024-11-05",
}

// Per-request and per-response _meta keys defined by the stateless model.
const (
	metaKeyProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	metaKeyClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	metaKeyServerInfo         = "io.modelcontextprotocol/serverInfo"
)

// resultTypeComplete marks a result that needs no further client input. The
// stateless protocol requires a resultType on every result.
const resultTypeComplete = "complete"

// MCP-specific JSON-RPC error codes allocated by 2026-07-28 (SEP-2575).
const (
	codeMethodNotFound             = -32601
	codeInvalidParams              = -32602
	codeUnsupportedProtocolVersion = -32022
)

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Server struct {
	info         ServerInfo
	logger       *slog.Logger
	Instructions string
	mu           sync.RWMutex
	tools        map[string]Tool
	writeMu      sync.Mutex
}

type Tool struct {
	Name         string           `json:"name"`
	Title        string           `json:"title,omitempty"`
	Description  string           `json:"description"`
	InputSchema  map[string]any   `json:"inputSchema"`
	OutputSchema map[string]any   `json:"outputSchema,omitempty"`
	Annotations  *ToolAnnotations `json:"annotations,omitempty"`
	Handler      ToolHandler      `json:"-"`
}

// ToolAnnotations carries optional hints about a tool. All fields are hints and
// may be omitted; clients must not rely on them.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

// Bool returns a pointer to b, for setting the optional annotation hints.
func Bool(b bool) *bool { return &b }

type ToolHandler func(ctx context.Context, args json.RawMessage) (any, error)

// Content is a single MCP content block. Only the fields relevant to Type are
// populated: text for "text", data/mimeType for "image" and "audio", and
// uri/name for "resource_link".
type Content struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Data        string `json:"data,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	URI         string `json:"uri,omitempty"`
	Name        string `json:"name,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// ToolResult is a complete tool result. Handlers may return one directly to
// control the content blocks and isError flag; any other return value is
// JSON-encoded into a single text block.
type ToolResult struct {
	Content           []Content      `json:"content"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
	ResultType        string         `json:"resultType,omitempty"`
	Meta              map[string]any `json:"_meta,omitempty"`
}

// TextResult builds a successful single-block text result.
func TextResult(text string) ToolResult {
	return ToolResult{Content: []Content{{Type: "text", Text: text}}}
}

// StructuredResult builds a result that carries text plus the structured value
// returned in the structuredContent field.
func StructuredResult(text string, value any) ToolResult {
	return ToolResult{
		Content:           []Content{{Type: "text", Text: text}},
		StructuredContent: value,
	}
}

// JSONResult builds a text result from a JSON payload and, when the payload
// parses, exposes it as structuredContent as well.
func JSONResult(text string) ToolResult {
	result := TextResult(text)
	var value any
	if err := json.Unmarshal([]byte(text), &value); err == nil {
		result.StructuredContent = value
	}
	return result
}

// ImageContent builds an image content block from base64 data.
func ImageContent(data, mimeType string) Content {
	return Content{Type: "image", Data: data, MimeType: mimeType}
}

// AudioContent builds an audio content block from base64 data.
func AudioContent(data, mimeType string) Content {
	return Content{Type: "audio", Data: data, MimeType: mimeType}
}

// ResourceLinkContent builds a resource_link content block.
func ResourceLinkContent(uri, name string) Content {
	return Content{Type: "resource_link", URI: uri, Name: name}
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

type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

// negotiateLegacyProtocolVersion follows the initialize handshake's version
// negotiation: echo the client's version when it is a supported legacy
// revision, otherwise offer the newest legacy revision. Stateless revisions
// are reached through server/discover, never through initialize.
func negotiateLegacyProtocolVersion(requested string) string {
	if requested < protocolVersion20260728 {
		for _, version := range supportedProtocolVersions {
			if version == requested {
				return requested
			}
		}
	}
	return latestLegacyProtocolVersion
}

// isSupportedProtocolVersion reports whether version is one this server speaks.
func isSupportedProtocolVersion(version string) bool {
	for _, supported := range supportedProtocolVersions {
		if supported == version {
			return true
		}
	}
	return false
}

// requestMeta is the per-request _meta the stateless protocol carries.
type requestMeta struct {
	// modern is true when the request selected the stateless protocol by
	// declaring protocolVersion >= 2026-07-28.
	modern bool
	// version is the declared protocol version, empty on the legacy path.
	version string
}

// parseRequestMeta detects whether a request's params select the stateless
// protocol. It mirrors the reference implementation: a request is legacy
// unless its _meta carries io.modelcontextprotocol/protocolVersion at a
// 2026-07-28-or-newer revision, and a stateless request must also declare
// io.modelcontextprotocol/clientCapabilities.
func parseRequestMeta(params json.RawMessage) (requestMeta, *rpcError) {
	if len(params) == 0 {
		return requestMeta{}, nil
	}
	var p struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return requestMeta{}, nil
	}
	raw, ok := p.Meta[metaKeyProtocolVersion]
	if !ok {
		return requestMeta{}, nil
	}
	var version string
	if err := json.Unmarshal(raw, &version); err != nil || version < protocolVersion20260728 {
		return requestMeta{}, nil
	}
	if _, ok := p.Meta[metaKeyClientCapabilities]; !ok {
		return requestMeta{}, &rpcError{
			Code:    codeInvalidParams,
			Message: "missing or invalid _meta field " + metaKeyClientCapabilities,
		}
	}
	return requestMeta{modern: true, version: version}, nil
}

// unsupportedProtocolVersionError builds the SEP-2575 -32022 error whose data
// advertises the supported revisions so a client can retry with one of them.
func unsupportedProtocolVersionError(requested string) *rpcError {
	return &rpcError{
		Code:    codeUnsupportedProtocolVersion,
		Message: "unsupported protocol version",
		Data: map[string]any{
			"supported": append([]string(nil), supportedProtocolVersions...),
			"requested": requested,
		},
	}
}

func methodNotFoundError(method string) *rpcError {
	return &rpcError{Code: codeMethodNotFound, Message: "method not found: " + method}
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

	meta, metaErr := parseRequestMeta(req.Params)
	if metaErr != nil {
		resp.Error = metaErr
		return resp, true
	}
	if meta.modern && !isSupportedProtocolVersion(meta.version) {
		resp.Error = unsupportedProtocolVersionError(meta.version)
		return resp, true
	}

	switch req.Method {
	case "initialize":
		// initialize is removed in 2026-07-28; stateless clients discover the
		// server instead. Legacy clients negotiate down to a legacy revision.
		if meta.modern {
			resp.Error = methodNotFoundError("initialize")
			break
		}
		var p initializeParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				resp.Error = &rpcError{Code: codeInvalidParams, Message: "invalid params: " + err.Error()}
				break
			}
		}
		result := map[string]any{
			"protocolVersion": negotiateLegacyProtocolVersion(p.ProtocolVersion),
			"capabilities":    s.capabilities(),
			"serverInfo":      s.info,
		}
		if s.Instructions != "" {
			result["instructions"] = s.Instructions
		}
		resp.Result = result
	case "server/discover":
		if !meta.modern {
			resp.Error = methodNotFoundError("server/discover")
			break
		}
		resp.Result = s.discoverResult()
	case "ping":
		// ping is removed in 2026-07-28.
		if meta.modern {
			resp.Error = methodNotFoundError("ping")
			break
		}
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = s.listToolsResult(meta.modern)
	case "tools/call":
		result, err := s.callTool(ctx, req.Params)
		if err != nil {
			resp.Error = &rpcError{Code: codeInvalidParams, Message: err.Error()}
		} else {
			resp.Result = result
		}
	default:
		resp.Error = methodNotFoundError(req.Method)
	}

	if meta.modern && resp.Error == nil {
		resp.Result = s.withStatelessMeta(resp.Result)
	}

	return resp, true
}

// capabilities is the server capability set advertised by initialize and
// server/discover.
func (s *Server) capabilities() map[string]any {
	return map[string]any{"tools": map[string]any{}}
}

// listToolsResult builds the tools/list result. Stateless clients also receive
// the cache hints every list result carries from 2026-07-28 on.
func (s *Server) listToolsResult(modern bool) map[string]any {
	result := map[string]any{"tools": s.listTools()}
	if modern {
		result["ttlMs"] = 0
		result["cacheScope"] = "public"
	}
	return result
}

// discoverResult implements the SEP-2575 server/discover RPC: without any
// prior handshake a client learns the supported protocol versions, the server's
// capabilities and identity, and how to use it.
func (s *Server) discoverResult() map[string]any {
	result := map[string]any{
		"supportedVersions": append([]string(nil), supportedProtocolVersions...),
		"capabilities":      s.capabilities(),
		"ttlMs":             0,
		"cacheScope":        "public",
	}
	if s.Instructions != "" {
		result["instructions"] = s.Instructions
	}
	return result
}

// withStatelessMeta adds the fields every 2026-07-28 result requires: a
// resultType and the server's identity in _meta. Legacy results are unchanged.
func (s *Server) withStatelessMeta(result any) any {
	meta := map[string]any{metaKeyServerInfo: s.info}
	switch r := result.(type) {
	case ToolResult:
		r.ResultType = resultTypeComplete
		r.Meta = meta
		return r
	case *ToolResult:
		r.ResultType = resultTypeComplete
		r.Meta = meta
		return r
	case map[string]any:
		if _, ok := r["resultType"]; !ok {
			r["resultType"] = resultTypeComplete
		}
		r["_meta"] = meta
		return r
	default:
		return result
	}
}

func (s *Server) listTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tools := make([]Tool, 0, len(s.tools))
	for _, t := range s.tools {
		tools = append(tools, Tool{
			Name:         t.Name,
			Title:        t.Title,
			Description:  t.Description,
			InputSchema:  t.InputSchema,
			OutputSchema: t.OutputSchema,
			Annotations:  t.Annotations,
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
