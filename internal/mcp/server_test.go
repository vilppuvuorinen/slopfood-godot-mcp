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
	s.RegisterTool(Tool{
		Name:        "echo",
		Description: "echo",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			return string(args), nil
		},
	})
	return s
}

func TestServeInitialize(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")
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
	if result["protocolVersion"] != protocolVersion {
		t.Errorf("protocolVersion = %v", result["protocolVersion"])
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
