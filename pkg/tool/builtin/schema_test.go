package builtin_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox/local"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool/builtin"
)

func TestBuiltinToolSchemas(t *testing.T) {
	dir := t.TempDir()
	sbx, err := local.New(dir)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	defer sbx.Close(context.Background())

	tools := []tool.Tool{
		// sandbox-backed tools
		builtin.NewExecuteTool(sbx),
		builtin.NewReadFileTool(sbx),
		builtin.NewWriteFileTool(sbx),
		builtin.NewEditFileTool(sbx),
		builtin.NewLsTool(sbx),
		builtin.NewGlobTool(sbx),
		builtin.NewGrepTool(sbx),
		// stateless network tools
		builtin.NewHTTPRequestTool(),
		builtin.NewFetchURLTool(),
		builtin.NewWebSearchTool("", ""),
		// stateful tools (schema methods don't use state)
		builtin.NewTodoTool(builtin.NewTodoState()),
		builtin.NewTaskTool(nil, tool.NewRegistry(), "", 0),
		// reviewer tools
		builtin.NewAddFindingTool(),
		builtin.NewUpdateFindingTool(),
		builtin.NewListFindingsTool(),
		builtin.NewPublishReviewTool(),
		builtin.NewResolveFindingThreadTool(),
		builtin.NewReplyToFindingThreadTool(),
		builtin.NewSaveReviewStylePromptTool(),
	}

	for _, current := range tools {
		t.Run(current.Name(), func(t *testing.T) {
			if current.Description() == "" {
				t.Fatal("description cannot be empty")
			}

			schema := current.Parameters()
			if schema.Type != "object" {
				t.Fatalf("expected object schema, got %q", schema.Type)
			}
			for _, required := range schema.Required {
				if _, ok := schema.Properties[required]; !ok {
					t.Fatalf("required field %q missing from properties", required)
				}
			}
			for name, prop := range schema.Properties {
				if prop.Type == "" {
					t.Fatalf("property %q missing type", name)
				}
				if prop.Description == "" {
					t.Fatalf("property %q missing description", name)
				}
				if len(prop.Enum) > 0 && prop.Type != "string" {
					t.Fatalf("property %q has enum but is type %q", name, prop.Type)
				}
			}

			encoded, err := json.Marshal(schema)
			if err != nil {
				t.Fatalf("schema must marshal to JSON: %v", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("schema JSON must unmarshal: %v", err)
			}
			if decoded["type"] != "object" {
				t.Fatalf("marshaled schema type mismatch: %s", string(encoded))
			}
		})
	}
}
