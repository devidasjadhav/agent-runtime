package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type WebSearchTool struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

func NewWebSearchTool(apiKey, baseURL string) *WebSearchTool {
	if baseURL == "" {
		baseURL = "https://api.tavily.com"
	}
	return &WebSearchTool{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *WebSearchTool) Name() string { return "web_search" }

func (t *WebSearchTool) Description() string {
	return "Search the web for information. Returns relevant search results with titles, URLs, and content snippets."
}

func (t *WebSearchTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema(
		[]string{"query"},
		map[string]tool.ToolPropertySchema{
			"query":            tool.StringProperty("The search query"),
			"max_results":      tool.IntegerProperty("Maximum number of results (1-10)", 5),
			"search_depth":     tool.StringEnumProperty("Search depth", []string{"basic", "advanced"}, "basic"),
			"include_domains":  tool.StringProperty("Comma-separated list of domains to include"),
			"exclude_domains":  tool.StringProperty("Comma-separated list of domains to exclude"),
		},
	)
}

func (t *WebSearchTool) Execute(_ context.Context, args json.RawMessage) (tool.Result, error) {
	if t.APIKey == "" {
		return tool.Result{Content: "Error: web search API key not configured", Error: true}, nil
	}

	var input struct {
		Query           string `json:"query"`
		MaxResults      int    `json:"max_results"`
		SearchDepth     string `json:"search_depth"`
		IncludeDomains  string `json:"include_domains"`
		ExcludeDomains  string `json:"exclude_domains"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tool.Result{Content: fmt.Sprintf("Error: invalid arguments: %v", err), Error: true}, nil
	}

	if input.MaxResults <= 0 || input.MaxResults > 10 {
		input.MaxResults = 5
	}
	if input.SearchDepth == "" {
		input.SearchDepth = "basic"
	}

	payload := map[string]any{
		"api_key":       t.APIKey,
		"query":         input.Query,
		"max_results":   input.MaxResults,
		"search_depth":  input.SearchDepth,
		"include_answer": true,
	}

	if input.IncludeDomains != "" {
		payload["include_domains"] = stringsToSlice(input.IncludeDomains)
	}
	if input.ExcludeDomains != "" {
		payload["exclude_domains"] = stringsToSlice(input.ExcludeDomains)
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", t.BaseURL+"/search", bytes.NewReader(body))
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("Error creating request: %v", err), Error: true}, nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.Client.Do(req)
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("Error: search failed: %v", err), Error: true}, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 500_000))
	if err != nil {
		return tool.Result{Content: fmt.Sprintf("Error reading response: %v", err), Error: true}, nil
	}

	var result struct {
		Answer  string `json:"answer"`
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return tool.Result{Content: string(respBody)}, nil
	}

	var out string
	if result.Answer != "" {
		out = fmt.Sprintf("Answer: %s\n\n", result.Answer)
	}
	for i, r := range result.Results {
		out += fmt.Sprintf("## Result %d: %s\nURL: %s\n%s\n\n", i+1, r.Title, r.URL, r.Content)
	}

	if len(out) > 80_000 {
		out = out[:80_000] + "\n\n... [truncated]"
	}
	return tool.Result{Content: out}, nil
}

func stringsToSlice(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
