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

func TestServeInitializeFallsBackToLatest(t *testing.T) {
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
	if result["protocolVersion"] != latestProtocolVersion {
		t.Errorf("protocolVersion = %v, want %v", result["protocolVersion"], latestProtocolVersion)
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
