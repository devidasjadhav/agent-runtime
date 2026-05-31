package model

import (
	"errors"
	"fmt"
	"strings"
)

type ErrorCategory string

const (
	ErrorCategoryValidation  ErrorCategory = "validation"
	ErrorCategoryAuth        ErrorCategory = "auth"
	ErrorCategoryRateLimit   ErrorCategory = "rate_limit"
	ErrorCategoryTimeout     ErrorCategory = "timeout"
	ErrorCategoryTransient   ErrorCategory = "transient_provider_failure"
	ErrorCategoryTerminal    ErrorCategory = "terminal_provider_failure"
	ErrorCategoryUnsupported ErrorCategory = "unsupported"
)

type ProviderError struct {
	Provider string
	Category ErrorCategory
	Status   int
	Message  string
	Err      error
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	base := e.Message
	if base == "" && e.Err != nil {
		base = e.Err.Error()
	}
	if e.Provider != "" {
		return fmt.Sprintf("%s provider error (%s): %s", e.Provider, e.Category, base)
	}
	return fmt.Sprintf("provider error (%s): %s", e.Category, base)
}

func (e *ProviderError) Unwrap() error { return e.Err }

func ErrorCategoryOf(err error) ErrorCategory {
	if err == nil {
		return ""
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) && providerErr.Category != "" {
		return providerErr.Category
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "rate limit") || strings.Contains(msg, "429"):
		return ErrorCategoryRateLimit
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded"):
		return ErrorCategoryTimeout
	case strings.Contains(msg, "unauthorized") || strings.Contains(msg, "forbidden") || strings.Contains(msg, "401") || strings.Contains(msg, "403"):
		return ErrorCategoryAuth
	case strings.Contains(msg, "bad request") || strings.Contains(msg, "invalid") || strings.Contains(msg, "400"):
		return ErrorCategoryValidation
	case strings.Contains(msg, "500") || strings.Contains(msg, "502") || strings.Contains(msg, "503") || strings.Contains(msg, "504"):
		return ErrorCategoryTransient
	default:
		return ErrorCategoryTerminal
	}
}

func IsFallbackEligible(err error) bool {
	switch ErrorCategoryOf(err) {
	case ErrorCategoryRateLimit, ErrorCategoryTimeout, ErrorCategoryTransient:
		return true
	default:
		return false
	}
}
