package openai

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"

	apimodel "github.com/anomalyco/open-swe/agent-runtime/pkg/model"
)

type Provider struct {
	client *openai.Client
}

func NewProvider(apiKey string, opts ...option.RequestOption) *Provider {
	clientOpts := []option.RequestOption{option.WithAPIKey(apiKey)}
	clientOpts = append(clientOpts, opts...)
	client := openai.NewClient(clientOpts...)
	return &Provider{client: &client}
}

func (p *Provider) Complete(ctx context.Context, req apimodel.ModelRequest) (*apimodel.ModelResponse, error) {
	params := p.buildParams(req)
	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, wrapProviderError(err)
	}
	return p.parseResponse(resp), nil
}

func (p *Provider) Stream(ctx context.Context, req apimodel.ModelRequest) (<-chan apimodel.ModelChunk, error) {
	params := p.buildParams(req)
	stream := p.client.Chat.Completions.NewStreaming(ctx, params)

	ch := make(chan apimodel.ModelChunk, 64)
	go func() {
		defer close(ch)

		for stream.Next() {
			chunk := stream.Current()

			if len(chunk.Choices) == 0 {
				continue
			}
			delta := chunk.Choices[0].Delta

			if delta.Content != "" {
				ch <- apimodel.ModelChunk{Type: "content", Content: delta.Content}
			}

			for _, tc := range delta.ToolCalls {
				if tc.Function.Name != "" {
					ch <- apimodel.ModelChunk{
						Type:       "tool_call_start",
						ToolIndex:  int(tc.Index),
						ToolCallID: tc.ID,
						ToolName:   tc.Function.Name,
					}
				}
				if tc.Function.Arguments != "" {
					ch <- apimodel.ModelChunk{
						Type:       "tool_call_args",
						ToolIndex:  int(tc.Index),
						ToolCallID: tc.ID,
						ToolArgs:   tc.Function.Arguments,
					}
				}
			}
		}

		if err := stream.Err(); err != nil {
			ch <- apimodel.ModelChunk{Type: "error", Content: wrapProviderError(err).Error()}
			return
		}

		ch <- apimodel.ModelChunk{Type: "done", Done: true}
	}()

	return ch, nil
}

func wrapProviderError(err error) error {
	return &apimodel.ProviderError{
		Provider: "openai-compatible",
		Category: apimodel.ErrorCategoryOf(err),
		Message:  err.Error(),
		Err:      err,
	}
}

func (p *Provider) buildParams(req apimodel.ModelRequest) openai.ChatCompletionNewParams {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)

	if req.SystemPrompt != "" {
		messages = append(messages, openai.SystemMessage(req.SystemPrompt))
	}

	for _, m := range req.Messages {
		switch m.Role {
		case apimodel.RoleUser:
			messages = append(messages, openai.UserMessage(m.Content))
		case apimodel.RoleAssistant:
			if len(m.ToolCalls) > 0 {
				assistantMsg := openai.ChatCompletionAssistantMessageParam{}
				if m.Content != "" {
					parts := []openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{
						{OfText: &openai.ChatCompletionContentPartTextParam{Text: m.Content}},
					}
					assistantMsg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
						OfArrayOfContentParts: parts,
					}
				}
				assistantMsg.ToolCalls = make([]openai.ChatCompletionMessageToolCallParam, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, openai.ChatCompletionMessageToolCallParam{
						ID: tc.ID,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					})
				}
				messages = append(messages, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &assistantMsg,
				})
			} else {
				messages = append(messages, openai.AssistantMessage(m.Content))
			}
		case apimodel.RoleTool:
			messages = append(messages, openai.ToolMessage(m.Content, m.ToolCallID))
		}
	}

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(req.Model),
		Messages: messages,
	}

	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = param.Opt[int64]{Value: int64(req.MaxTokens)}
	}

	if len(req.Tools) > 0 {
		params.Tools = make([]openai.ChatCompletionToolParam, 0, len(req.Tools))
		for _, t := range req.Tools {
			params.Tools = append(params.Tools, openai.ChatCompletionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        t.Name,
					Description: param.Opt[string]{Value: t.Description},
					Parameters:  toFunctionParameters(t.Parameters),
				},
			})
		}
	}

	return params
}

func (p *Provider) parseResponse(resp *openai.ChatCompletion) *apimodel.ModelResponse {
	if len(resp.Choices) == 0 {
		return &apimodel.ModelResponse{
			Message:    apimodel.Message{Role: apimodel.RoleAssistant, Content: ""},
			StopReason: "empty",
		}
	}
	choice := resp.Choices[0]

	msg := apimodel.Message{
		Role:    apimodel.RoleAssistant,
		Content: choice.Message.Content,
	}

	for _, tc := range choice.Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, apimodel.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	reason := string(choice.FinishReason)
	if strings.Contains(strings.ToLower(reason), "tool") {
		reason = "tool_call"
	}

	return &apimodel.ModelResponse{
		Message:    msg,
		StopReason: reason,
		Usage: apimodel.Usage{
			InputTokens:  int(resp.Usage.PromptTokens),
			OutputTokens: int(resp.Usage.CompletionTokens),
		},
	}
}

func toFunctionParameters(v interface{}) openai.FunctionParameters {
	switch p := v.(type) {
	case map[string]interface{}:
		return openai.FunctionParameters(p)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return openai.FunctionParameters{}
		}
		var out map[string]interface{}
		if err := json.Unmarshal(data, &out); err != nil {
			return openai.FunctionParameters{}
		}
		return openai.FunctionParameters(out)
	}
}
