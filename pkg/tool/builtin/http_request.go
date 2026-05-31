package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type HTTPRequestTool struct {
	Client *http.Client
}

func NewHTTPRequestTool() *HTTPRequestTool {
	return &HTTPRequestTool{
		Client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *HTTPRequestTool) Name() string { return "http_request" }

func (t *HTTPRequestTool) Description() string {
	return "Make an HTTP request to a specified URL. Supports GET, POST, PUT, PATCH, DELETE methods. Use this to interact with APIs."
}

func (t *HTTPRequestTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema(
		[]string{"method", "url"},
		map[string]tool.ToolPropertySchema{
			"method":  tool.StringEnumProperty("HTTP method", []string{"GET", "POST", "PUT", "PATCH", "DELETE"}, "GET"),
			"url":     tool.StringProperty("The URL to request"),
			"headers": tool.StringProperty("JSON object of request headers"),
			"body":    tool.StringProperty("Request body (for POST/PUT/PATCH)"),
		},
	)
}

func (t *HTTPRequestTool) Execute(_ context.Context, args json.RawMessage) (tool.Result, error) {
	var input struct {
		Method  string `json:"method"`
		URL     string `json:"url"`
		Headers string `json:"headers"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tool.Result{Content: fmt.Sprintf("Error: invalid arguments: %v", err), Error: true}, nil
	}

	method := input.Method
	if method == "" {
		method = "GET"
	}

	var body io.Reader
	if input.Body != "" && (method == "POST" || method == "PUT" || method == "PATCH") {
		body = strings.NewReader(input.Body)
	}

	req, err := http.NewRequest(method, input.URL, body)
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("Error creating request: %v", err), Error: true}, nil
	}

	if input.Headers != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(input.Headers), &headers); err == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
	}

	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("Error: request failed: %v", err), Error: true}, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1_000_000))
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("Error reading response: %v", err), Error: true}, nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Status: %d %s\n", resp.StatusCode, resp.Status))
	b.WriteString(fmt.Sprintf("Content-Length: %d\n", len(respBody)))
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		b.WriteString(fmt.Sprintf("Content-Type: %s\n", ct))
	}
	b.WriteString("\n")
	b.Write(respBody)

	content := b.String()
	if len(content) > 80_000 {
		content = truncateHTTP(content, 80_000)
	}
	return tool.Result{Content: content}, nil
}

func truncateHTTP(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	head := limit / 2
	tail := limit / 2
	return s[:head] + "\n\n... [truncated] ...\n\n" + s[len(s)-tail:]
}
