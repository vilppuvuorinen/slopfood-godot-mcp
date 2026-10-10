package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testServer() *Server { return testServerWrapped(nil) }

func testServerWrapped(wrap HandlerWrapper) *Server {
	s := NewServer(ServerInfo{Name: "test", Version: "0.0.0"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.Instructions = "test instructions"
	handler := ToolHandler(func(_ context.Context, args json.RawMessage) (any, error) {
		return string(args), nil
	})
	if wrap != nil {
		handler = wrap(handler)
	}
	s.RegisterTool(Tool{
		Name:         "echo",
		Title:        "Echo",
		Description:  "echo",
		InputSchema:  map[string]any{"type": "object"},
		OutputSchema: map[string]any{"type": "object"},
		Annotations:  &ToolAnnotations{ReadOnlyHint: Bool(true)},
		Handler:      handler,
	})
	return s
}

func TestServeInitialize(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}` + "\n")
	var out bytes.Buffer

	if err := testServer().Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type %T", resp.Result)
	}
	if result["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocolVersion = %v", result["protocolVersion"])
	}
	if result["instructions"] != "test instructions" {
		t.Errorf("instructions = %v", result["instructions"])
	}
}

func TestServeInitializeFallsBackToLatestLegacy(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}` + "\n")
	var out bytes.Buffer

	if err := testServer().Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type %T", resp.Result)
	}
	if result["protocolVersion"] != latestLegacyProtocolVersion {
		t.Errorf("protocolVersion = %v, want %v", result["protocolVersion"], latestLegacyProtocolVersion)
	}
}

func TestServeInitializeNeverStateless(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28"}}` + "\n")
	var out bytes.Buffer

	if err := testServer().Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type %T", resp.Result)
	}
	if result["protocolVersion"] != latestLegacyProtocolVersion {
		t.Errorf("protocolVersion = %v, want %v", result["protocolVersion"], latestLegacyProtocolVersion)
	}
}

func TestServeToolsListMetadata(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n")
	var out bytes.Buffer

	if err := testServer().Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	var resp struct {
		Result struct {
			Tools []Tool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Result.Tools) != 1 {
		t.Fatalf("tools = %d, want 1", len(resp.Result.Tools))
	}
	tool := resp.Result.Tools[0]
	if tool.Title != "Echo" {
		t.Errorf("title = %q", tool.Title)
	}
	if tool.OutputSchema == nil {
		t.Error("outputSchema missing")
	}
	if tool.Annotations == nil || tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint {
		t.Errorf("annotations = %+v", tool.Annotations)
	}
}

func TestServeStructuredContent(t *testing.T) {
	s := NewServer(ServerInfo{Name: "test", Version: "0.0.0"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.RegisterTool(Tool{
		Name:        "structured",
		Description: "structured",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
			return StructuredResult(`{"count":1}`, map[string]any{"count": 1}), nil
		},
	})
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"structured"}}` + "\n")
	var out bytes.Buffer

	if err := s.Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if !strings.Contains(out.String(), `"structuredContent":{"count":1}`) {
		t.Errorf("output = %s", out.String())
	}
}

// The structured envelope keeps a mutation outputSchema valid even for an error.
func TestErrorResultStructuredContent(t *testing.T) {
	result := ErrorResult("boom", "try again")
	if !result.IsError {
		t.Fatal("IsError = false")
	}
	if len(result.Content) != 2 || result.Content[0].Text != "boom" {
		t.Fatalf("content = %+v", result.Content)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %T, want object", result.StructuredContent)
	}
	if structured["success"] != false {
		t.Errorf("success = %v, want false", structured["success"])
	}
	if structured["message"] != "boom" {
		t.Errorf("message = %v, want boom", structured["message"])
	}
	if solutions, ok := structured["solutions"].([]string); !ok || len(solutions) != 1 || solutions[0] != "try again" {
		t.Errorf("solutions = %#v, want [try again]", structured["solutions"])
	}
}

func TestServeToolsCall(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"message":"hi"}}}` + "\n")
	var out bytes.Buffer

	if err := testServer().Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if !strings.Contains(out.String(), "hi") {
		t.Errorf("output = %s", out.String())
	}
}

// The input stream is held open until the reply arrives, since a disconnect
// cancels an in-flight queued call.
func TestServeToolsCallThroughQueue(t *testing.T) {
	q := NewQueue(4, testQueueTimeout)
	defer q.Close()
	s := testServerWrapped(q.Wrap)

	inR, inW := io.Pipe()
	var out lockedBuffer
	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background(), inR, &out) }()

	writeLine(t, inW, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"message":"hi"}}}`)
	awaitCondition(t, func() bool { return strings.Contains(out.String(), "hi") }, "the queued echo reply")
	_ = inW.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not return after the stream closed")
	}
}

// A "block" tool signals started then blocks until canceled, then signals canceled.
func blockingServer() (*Server, <-chan struct{}, <-chan struct{}) {
	s := NewServer(ServerInfo{Name: "test", Version: "0.0.0"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	started := make(chan struct{})
	canceled := make(chan struct{})
	s.RegisterTool(Tool{
		Name:        "block",
		Description: "block",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(ctx context.Context, _ json.RawMessage) (any, error) {
			close(started)
			<-ctx.Done()
			close(canceled)
			return nil, ctx.Err()
		},
	})
	return s, started, canceled
}

// The reader hits EOF, cancels the connection context, and the handler observes it.
func TestServeCancelsHandlerOnDisconnect(t *testing.T) {
	s, started, canceled := blockingServer()

	inR, inW := io.Pipe()
	var out lockedBuffer
	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background(), inR, &out) }()

	writeLine(t, inW, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"block"}}`)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}

	_ = inW.Close()

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("handler context was not canceled on disconnect")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after disconnect")
	}
}

// The reader is parked in Read, so the stream is closed afterwards to let Serve
// return; the cancellation itself came from the parent context.
func TestServeParentContextCancelCancelsHandler(t *testing.T) {
	s, started, canceled := blockingServer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inR, inW := io.Pipe()
	var out lockedBuffer
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, inR, &out) }()

	writeLine(t, inW, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"block"}}`)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}

	cancel()

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("handler context was not canceled by the parent context")
	}

	_ = inW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return")
	}
}

func TestCallToolSerializesThroughQueue(t *testing.T) {
	s := NewServer(ServerInfo{Name: "test", Version: "0.0.0"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	q := NewQueue(64, testQueueTimeout)
	defer q.Close()

	var active, maxActive int64
	s.RegisterTool(Tool{
		Name:        "engine",
		Description: "engine",
		InputSchema: map[string]any{"type": "object"},
		Handler: q.Wrap(func(context.Context, json.RawMessage) (any, error) {
			n := atomic.AddInt64(&active, 1)
			for {
				highest := atomic.LoadInt64(&maxActive)
				if n <= highest || atomic.CompareAndSwapInt64(&maxActive, highest, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt64(&active, -1)
			return nil, nil
		}),
	})

	const calls = 20
	var wg sync.WaitGroup
	for range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.callTool(context.Background(), json.RawMessage(`{"name":"engine"}`)); err != nil {
				t.Errorf("callTool: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt64(&maxActive); got != 1 {
		t.Errorf("max concurrent handlers = %d, want 1", got)
	}
}

func TestCallToolQueueTimeoutIsToolError(t *testing.T) {
	s := NewServer(ServerInfo{Name: "test", Version: "0.0.0"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	q := NewQueue(4, 20*time.Millisecond)
	defer q.Close()

	s.RegisterTool(Tool{
		Name:        "hang",
		Description: "hang",
		InputSchema: map[string]any{"type": "object"},
		Handler: q.Wrap(func(ctx context.Context, _ json.RawMessage) (any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}),
	})

	result, err := s.callTool(context.Background(), json.RawMessage(`{"name":"hang"}`))
	if err != nil {
		t.Fatalf("callTool: %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want a tool error")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, ErrQueueTimeout.Error()) {
		t.Errorf("content = %+v, want queue timeout message", result.Content)
	}
}

func TestServeUnknownMethod(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"nope"}` + "\n")
	var out bytes.Buffer

	if err := testServer().Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("expected method-not-found, got %+v", resp.Error)
	}
}

func TestServeNotificationsGetNoResponse(t *testing.T) {
	in := strings.NewReader(
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}` + "\n" +
			`{"jsonrpc":"2.0","method":"nope"}` + "\n",
	)
	var out bytes.Buffer

	if err := testServer().Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("expected no response for notifications, got %q", out.String())
	}
}

// A notifications/cancelled for an in-flight request cancels it without a disconnect.
func TestServeCancellationNotificationStopsHandler(t *testing.T) {
	s, started, canceled := blockingServer()

	inR, inW := io.Pipe()
	var out lockedBuffer
	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background(), inR, &out) }()

	writeLine(t, inW, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"block"}}`)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}

	writeLine(t, inW, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":7}}`)
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation notification did not cancel the handler")
	}

	awaitCondition(t, func() bool { return strings.Contains(out.String(), "context canceled") }, "the cancellation reply")
	_ = inW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after the stream closed")
	}
}

const modernMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`

func serveOne(t *testing.T, req string) map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := testServer().Serve(context.Background(), strings.NewReader(req+"\n"), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resp struct {
		Result map[string]any `json:"result"`
		Error  *rpcError      `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal %q: %v", out.String(), err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	return resp.Result
}

func TestServeDiscover(t *testing.T) {
	result := serveOne(t, `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{`+modernMeta+`}}`)

	versions, ok := result["supportedVersions"].([]any)
	if !ok {
		t.Fatalf("supportedVersions type %T", result["supportedVersions"])
	}
	found := false
	for _, v := range versions {
		if v == protocolVersion20260728 {
			found = true
		}
	}
	if !found {
		t.Errorf("supportedVersions = %v, want %s", versions, protocolVersion20260728)
	}
	if result["resultType"] != resultTypeComplete {
		t.Errorf("resultType = %v", result["resultType"])
	}
	if result["cacheScope"] != "public" {
		t.Errorf("cacheScope = %v", result["cacheScope"])
	}
	if _, hasTTL := result["ttlMs"]; !hasTTL {
		t.Error("ttlMs missing")
	}
	if result["instructions"] != "test instructions" {
		t.Errorf("instructions = %v", result["instructions"])
	}
	caps, ok := result["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("capabilities type %T", result["capabilities"])
	}
	if _, hasTools := caps["tools"]; !hasTools {
		t.Errorf("capabilities = %v, want tools", caps)
	}
	meta, ok := result["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("_meta type %T", result["_meta"])
	}
	if _, hasServerInfo := meta[metaKeyServerInfo]; !hasServerInfo {
		t.Errorf("_meta = %v, want serverInfo", meta)
	}
}

func TestServeDiscoverRequiresStateless(t *testing.T) {
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}` + "\n")
	if err := testServer().Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("error = %+v, want method not found", resp.Error)
	}
}

func TestServeStatelessToolsList(t *testing.T) {
	result := serveOne(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{`+modernMeta+`}}`)

	if _, ok := result["tools"].([]any); !ok {
		t.Fatalf("tools type %T", result["tools"])
	}
	if result["resultType"] != resultTypeComplete {
		t.Errorf("resultType = %v", result["resultType"])
	}
	if result["cacheScope"] != "public" {
		t.Errorf("cacheScope = %v", result["cacheScope"])
	}
	if _, ok := result["_meta"].(map[string]any); !ok {
		t.Errorf("_meta missing: %v", result["_meta"])
	}
}

func TestServeLegacyToolsListUnchanged(t *testing.T) {
	result := serveOne(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)

	if _, ok := result["resultType"]; ok {
		t.Errorf("unexpected resultType on legacy result: %v", result)
	}
	if _, ok := result["_meta"]; ok {
		t.Errorf("unexpected _meta on legacy result: %v", result)
	}
	if _, ok := result["ttlMs"]; ok {
		t.Errorf("unexpected ttlMs on legacy result: %v", result)
	}
}

func TestServeStatelessToolsCall(t *testing.T) {
	result := serveOne(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"message":"hi"},`+modernMeta+`}}`)

	if result["resultType"] != resultTypeComplete {
		t.Errorf("resultType = %v", result["resultType"])
	}
	if _, ok := result["_meta"].(map[string]any); !ok {
		t.Errorf("_meta missing: %v", result["_meta"])
	}
}

func TestServeStatelessPingRemoved(t *testing.T) {
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{` + modernMeta + `}}` + "\n")
	if err := testServer().Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("error = %+v, want method not found", resp.Error)
	}
}

func TestServeStatelessUnsupportedVersion(t *testing.T) {
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2099-01-01","io.modelcontextprotocol/clientCapabilities":{}}}}` + "\n")
	if err := testServer().Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != codeUnsupportedProtocolVersion {
		t.Fatalf("error = %+v, want unsupported protocol version", resp.Error)
	}
}

func TestServeStatelessRequiresCapabilities(t *testing.T) {
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}` + "\n")
	if err := testServer().Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resp response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != codeInvalidParams {
		t.Fatalf("error = %+v, want invalid params", resp.Error)
	}
}

// Concurrency-safe io.Writer: the server's reply copy writes while the test reads.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p) //nolint:wrapcheck // a test buffer passes the writer's errors through unchanged.
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func awaitCondition(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func writeLine(t *testing.T, w io.Writer, line string) {
	t.Helper()
	if _, err := io.WriteString(w, line+"\n"); err != nil {
		t.Fatalf("write %q: %v", line, err)
	}
}
