package adapter

import (
	"testing"
)

func TestThreadIDFromLinearIssue(t *testing.T) {
	id := ThreadIDFromLinearIssue(12345)
	if id == "" {
		t.Error("expected non-empty thread ID")
	}
	if len(id) != 64 {
		t.Errorf("expected 64-char hex string, got %d", len(id))
	}
	same := ThreadIDFromLinearIssue(12345)
	if id != same {
		t.Error("deterministic ID changed for same input")
	}
	diff := ThreadIDFromLinearIssue(12346)
	if id == diff {
		t.Error("different inputs produced same ID")
	}
}

func TestThreadIDFromGitHubIssue(t *testing.T) {
	id := ThreadIDFromGitHubIssue(99)
	if id == "" {
		t.Error("expected non-empty thread ID")
	}
	if len(id) != 64 {
		t.Errorf("expected 64-char hex string, got %d", len(id))
	}
}

func TestThreadIDFromSlackThread(t *testing.T) {
	id := ThreadIDFromSlackThread("C123", "1234567890.123456")
	if id == "" {
		t.Error("expected non-empty thread ID")
	}
	same := ThreadIDFromSlackThread("C123", "1234567890.123456")
	if id != same {
		t.Error("deterministic ID changed")
	}
}

func TestThreadIDFromReviewerThread(t *testing.T) {
	id := ThreadIDFromReviewerThread("owner", "repo", 42)
	if id == "" {
		t.Error("expected non-empty thread ID")
	}
	same := ThreadIDFromReviewerThread("owner", "repo", 42)
	if id != same {
		t.Error("deterministic ID changed")
	}
	diff := ThreadIDFromReviewerThread("owner", "repo", 43)
	if id == diff {
		t.Error("different PR numbers produced same ID")
	}
}

func TestSplitModelID(t *testing.T) {
	provider, model := SplitModelID("openai:gpt-4o")
	if provider != "openai" || model != "gpt-4o" {
		t.Errorf("got %q/%q, want openai/gpt-4o", provider, model)
	}

	provider, model = SplitModelID("anthropic:claude-4-sonnet")
	if provider != "anthropic" || model != "claude-4-sonnet" {
		t.Errorf("got %q/%q, want anthropic/claude-4-sonnet", provider, model)
	}

	provider, model = SplitModelID("gpt-4o")
	if provider != "openai" || model != "gpt-4o" {
		t.Errorf("got %q/%q for no-prefix, want openai/gpt-4o", provider, model)
	}
}

func TestResolveModelID(t *testing.T) {
	configurable := map[string]any{
		"agent_model_id": "openai:gpt-4o-mini",
	}

	got := ResolveModelID(configurable, "openai:gpt-5.5", "agent_model_id")
	if got != "openai:gpt-4o-mini" {
		t.Errorf("configurable override: got %q, want %q", got, "openai:gpt-4o-mini")
	}

	got = ResolveModelID(map[string]any{}, "openai:gpt-5.5", "agent_model_id")
	if got != "openai:gpt-5.5" {
		t.Errorf("team default: got %q, want %q", got, "openai:gpt-5.5")
	}

	got = ResolveModelID(map[string]any{}, "", "agent_model_id")
	if got != DefaultModelID {
		t.Errorf("fallback: got %q, want %q", got, DefaultModelID)
	}

	got = ResolveModelID(map[string]any{"agent_model_id": ""}, "openai:gpt-5.5", "agent_model_id")
	if got != "openai:gpt-5.5" {
		t.Errorf("empty configurable: got %q, want %q", got, "openai:gpt-5.5")
	}
}

func TestFormatSourcePrefix(t *testing.T) {
	prefix := FormatSourcePrefix(SourceLinear, &UserInfo{GitHubLogin: "testuser"})
	if prefix == "" {
		t.Error("expected non-empty prefix")
	}

	prefix2 := FormatSourcePrefix(SourceGitHub, nil)
	if prefix2 == "" {
		t.Error("expected non-empty prefix for nil user")
	}
}

func TestDefaultConstants(t *testing.T) {
	if DefaultModelID != "openai:gpt-5.5" {
		t.Errorf("DefaultModelID = %q", DefaultModelID)
	}
	if DefaultMaxTokens != 64_000 {
		t.Errorf("DefaultMaxTokens = %d", DefaultMaxTokens)
	}
	if StyleAnalyzerMaxCallLimit != 80 {
		t.Errorf("StyleAnalyzerMaxCallLimit = %d", StyleAnalyzerMaxCallLimit)
	}
}
