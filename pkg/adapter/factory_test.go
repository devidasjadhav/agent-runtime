package adapter

import (
	"context"
	"testing"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/agent"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/agent/testutil"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/middleware"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

func TestFactoryCreateCodingAgent(t *testing.T) {
	fakeProvider := &testutil.FakeProvider{
		Responses: []testutil.FakeResponse{
			{Content: "done", Stop: true},
		},
	}

	webRegistry := tool.NewRegistry()
	f := NewAgentFactory(fakeProvider, webRegistry)

	handle, err := f.CreateCodingAgent(context.Background(), AgentConfig{
		ModelID:       "test-model",
		MaxTokens:     4096,
		MaxIterations: 10,
	}, nil)
	if err != nil {
		t.Fatalf("CreateCodingAgent error: %v", err)
	}
	if handle == nil {
		t.Fatal("handle is nil")
	}
	if handle.TodoState == nil {
		t.Error("expected TodoState to be initialized")
	}
}

func TestFactoryCreateReviewerAgent(t *testing.T) {
	fakeProvider := &testutil.FakeProvider{
		Responses: []testutil.FakeResponse{
			{Content: "review done", Stop: true},
		},
	}

	webRegistry := tool.NewRegistry()
	f := NewAgentFactory(fakeProvider, webRegistry)

	handle, err := f.CreateReviewerAgent(context.Background(), ReviewConfig{
		ModelID: "test-reviewer",
	}, nil)
	if err != nil {
		t.Fatalf("CreateReviewerAgent error: %v", err)
	}
	if handle == nil {
		t.Fatal("handle is nil")
	}
}

func TestFactoryCreateStyleAnalyzer(t *testing.T) {
	fakeProvider := &testutil.FakeProvider{
		Responses: []testutil.FakeResponse{
			{Content: "style analyzed", Stop: true},
		},
	}

	webRegistry := tool.NewRegistry()
	f := NewAgentFactory(fakeProvider, webRegistry)

	handle, err := f.CreateStyleAnalyzer(context.Background(), StyleAnalyzerConfig{
		FullName: "owner/repo",
	}, nil)
	if err != nil {
		t.Fatalf("CreateStyleAnalyzer error: %v", err)
	}
	if handle == nil {
		t.Fatal("handle is nil")
	}
}

func TestFactoryTeamDefaults(t *testing.T) {
	fakeProvider := &testutil.FakeProvider{}
	webRegistry := tool.NewRegistry()

	f := NewAgentFactory(fakeProvider, webRegistry, WithTeamDefaults(map[string]string{
		"agent":    "openai:gpt-5.5",
		"reviewer": "anthropic:claude-4-sonnet",
	}))

	if f.TeamDefault("agent") != "openai:gpt-5.5" {
		t.Errorf("agent default = %q", f.TeamDefault("agent"))
	}
	if f.TeamDefault("reviewer") != "anthropic:claude-4-sonnet" {
		t.Errorf("reviewer default = %q", f.TeamDefault("reviewer"))
	}
	if f.TeamDefault("nonexistent") != "" {
		t.Error("expected empty for nonexistent key")
	}

	f.SetTeamDefault("agent", "openai:gpt-4o")
	if f.TeamDefault("agent") != "openai:gpt-4o" {
		t.Errorf("updated agent default = %q", f.TeamDefault("agent"))
	}
}

func TestApplyMiddlewares(t *testing.T) {
	opts := []agent.Option{}
	chain := middleware.Chain{
		middleware.SanitizeInputs{},
		middleware.NewCallLimit(10),
	}
	opts = applyMiddlewares(opts, chain)
	if len(opts) != 2 {
		t.Errorf("expected 2 options, got %d", len(opts))
	}
}

func TestSSEStreamWriter(t *testing.T) {
	sink := SSEStreamWriter(&mockWriter{})
	err := sink.Send(context.Background(), "test_event", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}
}

type mockWriter struct{}

func (m *mockWriter) Write(p []byte) (int, error) { return len(p), nil }
