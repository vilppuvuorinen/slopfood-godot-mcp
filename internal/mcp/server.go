package mcp

import (
	"bufio"
	"bytes"
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

// Starting with 2026-07-28 the protocol is stateless: clients use server/discover instead of the initialize handshake.
const protocolVersion20260728 = "2026-07-28"

// Caps initialize negotiation so a legacy client is never handed a stateless revision.
const latestLegacyProtocolVersion = "2025-11-25"

// Newest first; 2024-11-05 is kept so older clients still connect.
var supportedProtocolVersions = []string{
	protocolVersion20260728,
	"2025-11-25",
	"2025-06-18",
	"2025-03-26",
	"2024-11-05",
}

const (
	metaKeyProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	metaKeyClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	metaKeyServerInfo         = "io.modelcontextprotocol/serverInfo"
)

// The stateless protocol requires a resultType on every result.
const resultTypeComplete = "complete"

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
	// queue, when set, serializes tool calls across all connections; nil runs handlers inline.
	queue   *Queue
	writeMu sync.Mutex
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

// All fields are hints and may be omitted; clients must not rely on them.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

func Bool(b bool) *bool { return &b }

type ToolHandler func(ctx context.Context, args json.RawMessage) (any, error)

// Only the fields relevant to Type are populated.
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

// Handlers may return one directly to control the content blocks and isError.
type ToolResult struct {
	Content           []Content      `json:"content"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
	ResultType        string         `json:"resultType,omitempty"`
	Meta              map[string]any `json:"_meta,omitempty"`
}

func TextResult(text string) ToolResult {
	return ToolResult{Content: []Content{{Type: "text", Text: text}}}
}

func StructuredResult(text string, value any) ToolResult {
	return ToolResult{
		Content:           []Content{{Type: "text", Text: text}},
		StructuredContent: value,
	}
}

func JSONResult(text string) ToolResult {
	result := TextResult(text)
	var value any
	if err := json.Unmarshal([]byte(text), &value); err == nil {
		result.StructuredContent = value
	}
	return result
}

func ImageContent(data, mimeType string) Content {
	return Content{Type: "image", Data: data, MimeType: mimeType}
}

func AudioContent(data, mimeType string) Content {
	return Content{Type: "audio", Data: data, MimeType: mimeType}
}

func ResourceLinkContent(uri, name string) Content {
	return Content{Type: "resource_link", URI: uri, Name: name}
}

// Mirrors the Coding-Solo response shape; the structured envelope keeps a
// declared mutation outputSchema valid even for an error.
func ErrorResult(message string, possibleSolutions ...string) ToolResult {
	content := []Content{{Type: "text", Text: message}}
	structured := map[string]any{"success": false, "message": message}
	if len(possibleSolutions) > 0 {
		var b strings.Builder
		b.WriteString("Possible solutions:\n- ")
		b.WriteString(strings.Join(possibleSolutions, "\n- "))
		content = append(content, Content{Type: "text", Text: b.String()})
		structured["solutions"] = possibleSolutions
	}
	return ToolResult{
		Content:           content,
		StructuredContent: structured,
		IsError:           true,
	}
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

// Echoes a supported legacy revision, otherwise offers the newest legacy one;
// stateless revisions are reached through server/discover.
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

func isSupportedProtocolVersion(version string) bool {
	for _, supported := range supportedProtocolVersions {
		if supported == version {
			return true
		}
	}
	return false
}

type requestMeta struct {
	modern bool
	// version is empty on the legacy path.
	version string
}

// A request is legacy unless its _meta declares protocolVersion >= 2026-07-28,
// and a stateless request must also declare clientCapabilities.
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

// With a queue installed, concurrent calls run one at a time; nil runs inline.
func (s *Server) SetQueue(q *Queue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = q
}

// The reader runs on its own goroutine and cancels the connection context on
// EOF or read error, so a disconnect kills any in-flight work. Each id-bearing
// request is handled on its own goroutine so a notification (notably
// notifications/cancelled) is still processed while one is running; the shared
// writer serializes the responses, which clients match by id.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	lines := make(chan []byte)
	readErr := make(chan error, 1)

	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := bytes.Clone(scanner.Bytes())
			select {
			case lines <- line:
			case <-ctx.Done():
				readErr <- ctx.Err()
				return
			}
		}
		cancel()
		readErr <- scanner.Err()
	}()

	inflight := newInflightRequests()
	var handlers sync.WaitGroup

	for line := range lines {
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			s.logger.Warn("malformed request", "error", err)
			continue
		}
		// A request without an id is a notification: process it but never respond.
		if len(req.ID) == 0 {
			s.handleNotification(req, inflight)
			continue
		}
		id := strings.TrimSpace(string(req.ID))
		reqCtx, reqCancel := context.WithCancel(ctx)
		inflight.add(id, reqCancel)
		handlers.Add(1)
		go func(reqCtx context.Context, req request, id string, reqCancel context.CancelFunc) {
			defer handlers.Done()
			defer inflight.remove(id)
			defer reqCancel()
			if err := s.write(w, s.handle(reqCtx, req)); err != nil {
				s.logger.Debug("write response", "error", err)
				cancel()
			}
		}(reqCtx, req, id, reqCancel)
	}

	handlers.Wait()
	if err := <-readErr; err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	return nil
}

// Notifications get no response; the only one that needs action is
// notifications/cancelled, which cancels the matching in-flight request.
func (s *Server) handleNotification(req request, inflight *inflightRequests) {
	if req.Method != "notifications/cancelled" || len(req.Params) == 0 {
		return
	}
	var params struct {
		RequestID json.RawMessage `json:"requestId"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Debug("invalid cancellation notification", "error", err)
		return
	}
	if len(params.RequestID) == 0 {
		return
	}
	inflight.cancel(strings.TrimSpace(string(params.RequestID)))
}

// inflightRequests tracks the requests being served on one connection so a
// cancellation notification can stop the matching one. Registration and
// cancellation run on the read loop, removal on the handler goroutine, so every
// access is mutex-guarded.
type inflightRequests struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func newInflightRequests() *inflightRequests {
	return &inflightRequests{cancels: make(map[string]context.CancelFunc)}
}

func (r *inflightRequests) add(id string, cancel context.CancelFunc) {
	r.mu.Lock()
	r.cancels[id] = cancel
	r.mu.Unlock()
}

func (r *inflightRequests) remove(id string) {
	r.mu.Lock()
	delete(r.cancels, id)
	r.mu.Unlock()
}

func (r *inflightRequests) cancel(id string) {
	r.mu.Lock()
	cancel := r.cancels[id]
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Server) handle(ctx context.Context, req request) response {
	resp := response{JSONRPC: "2.0", ID: req.ID}

	meta, metaErr := parseRequestMeta(req.Params)
	if metaErr != nil {
		resp.Error = metaErr
		return resp
	}
	if meta.modern && !isSupportedProtocolVersion(meta.version) {
		resp.Error = unsupportedProtocolVersionError(meta.version)
		return resp
	}

	switch req.Method {
	case "initialize":
		// initialize is removed in 2026-07-28; stateless clients use server/discover.
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

	return resp
}

func (s *Server) capabilities() map[string]any {
	return map[string]any{"tools": map[string]any{}}
}

func (s *Server) listToolsResult(modern bool) map[string]any {
	result := map[string]any{"tools": s.listTools()}
	if modern {
		result["ttlMs"] = 0
		result["cacheScope"] = "public"
	}
	return result
}

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
	queue := s.queue
	s.mu.RUnlock()
	if !ok {
		return nil, errors.New("unknown tool: " + p.Name)
	}
	out, err := s.callHandler(ctx, queue, tool.Handler, p.Arguments)
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

// Submits to queue when installed so engine calls serialize; a queue timeout
// cancels any engine work the handler started.
func (s *Server) callHandler(ctx context.Context, queue *Queue, handler ToolHandler, args json.RawMessage) (any, error) {
	if queue == nil {
		return handler(ctx, args)
	}
	return queue.Submit(ctx, func(ctx context.Context) (any, error) {
		return handler(ctx, args)
	})
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
