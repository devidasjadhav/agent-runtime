package langsmith

// Package langsmith is reserved for the production LangSmith sandbox adapter.
//
// Phase 3 intentionally adds this package as a provider boundary without
// pulling a concrete SDK dependency yet. The future adapter should implement
// sandbox.Sandbox, and should keep GitHub proxy/token configuration as a
// control-plane callback rather than embedding Open SWE auth policy here.
