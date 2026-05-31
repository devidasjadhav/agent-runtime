package builtin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPRequestTool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	tool := &HTTPRequestTool{Client: server.Client()}

	args, _ := json.Marshal(map[string]any{
		"method": "GET",
		"url":    server.URL + "/test",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Error {
		t.Errorf("unexpected error: %s", result.Content)
	}
}

func TestHTTPRequestToolPOST(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(201)
	}))
	defer server.Close()

	tool := &HTTPRequestTool{Client: server.Client()}
	args, _ := json.Marshal(map[string]any{
		"method": "POST",
		"url":    server.URL,
		"body":   `{"data": "test"}`,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Error {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if receivedBody != `{"data": "test"}` {
		t.Errorf("body = %q", receivedBody)
	}
}

func TestFetchRequestTool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`<html><body><h1>Hello World</h1><p>Content here</p></body></html>`))
	}))
	defer server.Close()

	tool := &FetchURLTool{Client: server.Client()}
	args, _ := json.Marshal(map[string]any{
		"url": server.URL,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.Error {
		t.Errorf("unexpected error: %s", result.Content)
	}
}

func TestFetchURLTool404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer server.Close()

	tool := &FetchURLTool{Client: server.Client()}
	args, _ := json.Marshal(map[string]any{
		"url": server.URL,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Error {
		t.Error("expected error for 404")
	}
}

func TestWebSearchToolNoAPIKey(t *testing.T) {
	tool := NewWebSearchTool("", "")
	args, _ := json.Marshal(map[string]any{
		"query": "test",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !result.Error {
		t.Error("expected error for missing API key")
	}
}
