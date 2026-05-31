package model_test

import (
	"testing"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/model"
)

func TestProviderProfiles(t *testing.T) {
	deepseek := model.DeepSeekProfile("key")
	if deepseek.Name != "deepseek" || deepseek.APIKey != "key" || deepseek.BaseURL != "https://api.deepseek.com" || deepseek.DefaultModel != "deepseek-v4-flash" {
		t.Fatalf("unexpected deepseek profile: %#v", deepseek)
	}

	openai := model.OpenAIProfile("key")
	if openai.Name != "openai" || openai.APIKey != "key" || openai.BaseURL != "" || openai.DefaultModel != "gpt-4o" {
		t.Fatalf("unexpected openai profile: %#v", openai)
	}
}

func TestProviderErrorCategoryAndFallbackEligibility(t *testing.T) {
	err := &model.ProviderError{Provider: "test", Category: model.ErrorCategoryRateLimit, Message: "rate limited"}
	if model.ErrorCategoryOf(err) != model.ErrorCategoryRateLimit {
		t.Fatalf("expected rate limit category")
	}
	if !model.IsFallbackEligible(err) {
		t.Fatal("expected rate limit to be fallback eligible")
	}

	validation := &model.ProviderError{Provider: "test", Category: model.ErrorCategoryValidation, Message: "bad request"}
	if model.IsFallbackEligible(validation) {
		t.Fatal("validation errors should not fallback")
	}
}
