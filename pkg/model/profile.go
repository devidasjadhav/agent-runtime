package model

import (
	"fmt"
	"os"
)

type ProviderProfile struct {
	Name         string
	APIKey       string
	BaseURL      string
	DefaultModel string
}

func ResolveProviderProfileFromEnv() (ProviderProfile, error) {
	if key := os.Getenv("DEEPSEEK_API_KEY"); key != "" {
		return DeepSeekProfile(key), nil
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return OpenAIProfile(key), nil
	}
	return ProviderProfile{}, fmt.Errorf("DEEPSEEK_API_KEY or OPENAI_API_KEY environment variable is required")
}

func DeepSeekProfile(apiKey string) ProviderProfile {
	return ProviderProfile{
		Name:         "deepseek",
		APIKey:       apiKey,
		BaseURL:      "https://api.deepseek.com",
		DefaultModel: "deepseek-v4-flash",
	}
}

func OpenAIProfile(apiKey string) ProviderProfile {
	return ProviderProfile{
		Name:         "openai",
		APIKey:       apiKey,
		DefaultModel: "gpt-4o",
	}
}

func CustomOpenAICompatibleProfile(name, apiKey, baseURL, defaultModel string) ProviderProfile {
	return ProviderProfile{
		Name:         name,
		APIKey:       apiKey,
		BaseURL:      baseURL,
		DefaultModel: defaultModel,
	}
}
