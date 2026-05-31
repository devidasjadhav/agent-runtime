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

type FetchURLTool struct {
	Client *http.Client
}

func NewFetchURLTool() *FetchURLTool {
	return &FetchURLTool{
		Client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *FetchURLTool) Name() string { return "fetch_url" }

func (t *FetchURLTool) Description() string {
	return "Fetch a URL and return its content. Automatically converts HTML to a readable text format. Use this to read web pages."
}

func (t *FetchURLTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema(
		[]string{"url"},
		map[string]tool.ToolPropertySchema{
			"url": tool.StringProperty("The URL to fetch"),
		},
	)
}

func (t *FetchURLTool) Execute(_ context.Context, args json.RawMessage) (tool.Result, error) {
	var input struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tool.Result{Content: fmt.Sprintf("Error: invalid arguments: %v", err), Error: true}, nil
	}

	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Get(input.URL)
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("Error fetching URL: %v", err), Error: true}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return tool.Result{Content: fmt.Sprintf("Error: HTTP %d %s", resp.StatusCode, resp.Status), Error: true}, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("Error reading response: %v", err), Error: true}, nil
	}

	content := string(body)
	if len(content) > 80_000 {
		content = truncateHTTP(content, 80_000)
	}

	return tool.Result{Content: content}, nil
}

func stripHTMLTags(s string) string {
	var out strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			out.WriteByte('\n')
			continue
		}
		if !inTag {
			out.WriteRune(r)
		}
	}
	return out.String()
}
