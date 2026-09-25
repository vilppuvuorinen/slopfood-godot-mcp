package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func testServer() *Server {
	s := NewServer(ServerInfo{Name: "test", Version: "0.0.0"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.Instructions = "test instructions"
	s.RegisterTool(Tool{
		Name:         "echo",
		Title:        "Echo",
		Description:  "echo",
		InputSchema:  map[string]any{"type": "object"},
		OutputSchema: map[string]any{"type": "object"},
		Annotations:  &ToolAnnotations{ReadOnlyHint: Bool(true)},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			return string(args), nil
		},
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

// TestServeInitializeNeverStateless asserts a legacy initialize request is
// never handed a 2026-07-28 revision: that path is only reachable via discover.
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

// TestErrorResultStructuredContent asserts an error result carries the
// success/message envelope as structured content alongside isError, so a client
// that eagerly validates structuredContent against a mutation outputSchema sees
// the required fields rather than no body. The text block stays the primary
// message, with any solutions appended after it.
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

// modernMeta is the per-request _meta triple a 2026-07-28 client sends.
const modernMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`

// serveOne runs a single request line through a fresh test server and returns
// the decoded result map.
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
		if v == latestProtocolVersion {
			found = true
		}
	}
	if !found {
		t.Errorf("supportedVersions = %v, want %s", versions, latestProtocolVersion)
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

// TestServeDiscoverRequiresStateless asserts the legacy path cannot call
// server/discover; a legacy client must use initialize.
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

// TestServeStatelessToolsList asserts a stateless tools/list carries resultType,
// serverInfo metadata, and the cache hints.
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

// TestServeLegacyToolsListUnchanged asserts the legacy path gets no stateless
// decoration.
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

// TestServeStatelessToolsCall asserts a stateless tools/call result carries a
// resultType and serverInfo metadata.
func TestServeStatelessToolsCall(t *testing.T) {
	result := serveOne(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"message":"hi"},`+modernMeta+`}}`)

	if result["resultType"] != resultTypeComplete {
		t.Errorf("resultType = %v", result["resultType"])
	}
	if _, ok := result["_meta"].(map[string]any); !ok {
		t.Errorf("_meta missing: %v", result["_meta"])
	}
}

// TestServeStatelessPingRemoved asserts ping is not served on 2026-07-28.
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

// TestServeStatelessUnsupportedVersion asserts an unknown modern revision gets
// the -32022 error advertising the supported list.
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

// TestServeStatelessRequiresCapabilities asserts the required _meta
// clientCapabilities key is enforced.
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
