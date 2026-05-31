package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/model"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type Summarizer interface {
	Summarize(ctx context.Context, messages []model.Message) (string, error)
}

type ContextSummarizer struct {
	Summarizer      Summarizer
	MaxMessages     int
	KeepRecent      int
	SummaryInserted bool
}

func NewContextSummarizer(summarizer Summarizer, maxMessages, keepRecent int) *ContextSummarizer {
	return &ContextSummarizer{
		Summarizer:  summarizer,
		MaxMessages: maxMessages,
		KeepRecent:  keepRecent,
	}
}

func (cs *ContextSummarizer) BeforeModel(ctx context.Context, state *State) (*State, error) {
	if cs.Summarizer == nil || cs.MaxMessages <= 0 || cs.KeepRecent <= 0 {
		return state, nil
	}

	if len(state.Messages) <= cs.MaxMessages {
		return state, nil
	}

	userAndAssistant := cs.extractSummarizable(state.Messages)
	if len(userAndAssistant) <= cs.KeepRecent {
		return state, nil
	}

	oldMessages := userAndAssistant[:len(userAndAssistant)-cs.KeepRecent]
	summary, err := cs.Summarizer.Summarize(ctx, oldMessages)
	if err != nil {
		log.Printf("context summarization failed: %v", err)
		return state, nil
	}

	summarized := model.Message{
		Role:    "user",
		Content: fmt.Sprintf("[Conversation summary]\n%s\n[End of summary. The conversation continues below with recent messages.]", summary),
	}

	summarizedSet := make(map[int]bool)
	for i, msg := range state.Messages {
		if m, ok := msg.(model.Message); ok {
			for _, old := range oldMessages {
				if m.Role == old.Role && m.Content == old.Content {
					summarizedSet[i] = true
				}
			}
		}
	}

	var newMessages []any
	newMessages = append(newMessages, summarized)
	for i, msg := range state.Messages {
		if !summarizedSet[i] {
			newMessages = append(newMessages, msg)
		}
	}

	state.Messages = newMessages
	cs.SummaryInserted = true

	if state.Metadata == nil {
		state.Metadata = make(map[string]any)
	}
	state.Metadata["summarized_message_count"] = len(oldMessages)
	state.Metadata["summary_length"] = len(summary)

	return state, nil
}

func (cs *ContextSummarizer) AfterModel(_ context.Context, state *State, _ *ModelResult) (*State, error) {
	return state, nil
}

func (cs *ContextSummarizer) BeforeTool(_ context.Context, call *ToolCall) (*ToolCall, error) {
	return call, nil
}

func (cs *ContextSummarizer) AfterTool(_ context.Context, _ *ToolCall, result tool.Result) (tool.Result, error) {
	return result, nil
}

func (cs *ContextSummarizer) extractSummarizable(messages []any) []model.Message {
	var result []model.Message
	for _, msg := range messages {
		if m, ok := msg.(model.Message); ok {
			if m.Role == "user" || m.Role == "assistant" {
				result = append(result, m)
			}
		}
	}
	return result
}

type ProviderSummarizer struct {
	Provider model.Provider
	ModelID  string
	MaxTokens int
}

func NewProviderSummarizer(provider model.Provider, modelID string) *ProviderSummarizer {
	return &ProviderSummarizer{
		Provider:  provider,
		ModelID:   modelID,
		MaxTokens: 1024,
	}
}

func (ps *ProviderSummarizer) Summarize(ctx context.Context, messages []model.Message) (string, error) {
	var parts []string
	for _, msg := range messages {
		role := msg.Role
		content := msg.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s: %s", role, content))
	}

	conversationText := strings.Join(parts, "\n")

	req := &model.ModelRequest{
		Model: ps.ModelID,
		SystemPrompt: "You are a conversation summarizer. Summarize the conversation below into a concise but complete summary that preserves all important facts, decisions, and actions taken. Focus on what was done and what the current state is.",
		Messages: []model.Message{
			{Role: "user", Content: fmt.Sprintf("Please summarize this conversation:\n\n%s", conversationText)},
		},
		MaxTokens: ps.MaxTokens,
	}

	resp, err := ps.Provider.Complete(ctx, *req)
	if err != nil {
		return "", fmt.Errorf("summarization model call failed: %w", err)
	}

	return resp.Message.Content, nil
}

func EstimateTokenCount(text string) int {
	return len(text) / 4
}

func EstimateMessageTokens(messages []model.Message) int {
	total := 0
	for _, msg := range messages {
		total += EstimateTokenCount(msg.Content)
		for _, tc := range msg.ToolCalls {
			total += EstimateTokenCount(tc.Arguments)
		}
	}
	return total
}

func MarshalMessagesForSummary(messages []model.Message) string {
	var parts []string
	for _, msg := range messages {
		data, err := json.Marshal(map[string]string{
			"role":    string(msg.Role),
			"content": msg.Content,
		})
		if err != nil {
			continue
		}
		parts = append(parts, string(data))
	}
	return strings.Join(parts, "\n")
}
