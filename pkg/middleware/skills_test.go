package middleware_test

import (
	"context"
	"strings"
	"testing"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/middleware"
)

const validSkill = `---
name: my-skill
description: Does something useful
allowed_tools:
  - read_file
  - execute
---

Full instructions here.
`

const minimalSkill = `---
name: minimal
description: bare minimum
---
`

func skillLoader(files map[string]string) middleware.FileLoaderFunc {
	return func(_ context.Context, path string) ([]byte, error) {
		if c, ok := files[path]; ok {
			return []byte(c), nil
		}
		return nil, context.DeadlineExceeded
	}
}

func TestSkills_InjectsPrompt(t *testing.T) {
	m := middleware.NewSkillsMiddleware(skillLoader(map[string]string{
		"/skills/my-skill/SKILL.md": validSkill,
	}), []middleware.SkillSource{{Path: "/skills/my-skill/SKILL.md", Label: "Test"}})

	state, err := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
	if err != nil {
		t.Fatalf("BeforeModel: %v", err)
	}
	if len(state.SystemPromptExtensions) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(state.SystemPromptExtensions))
	}
	prompt := state.SystemPromptExtensions[0]
	if !strings.Contains(prompt, "my-skill") {
		t.Errorf("prompt missing skill name: %q", prompt)
	}
	if !strings.Contains(prompt, "Does something useful") {
		t.Errorf("prompt missing description: %q", prompt)
	}
	if !strings.Contains(prompt, "/skills/my-skill/SKILL.md") {
		t.Errorf("prompt missing file path: %q", prompt)
	}
	if !strings.Contains(prompt, "source: Test") {
		t.Errorf("prompt missing label: %q", prompt)
	}
}

func TestSkills_AllowedToolsListed(t *testing.T) {
	m := middleware.NewSkillsMiddleware(skillLoader(map[string]string{
		"/s/SKILL.md": validSkill,
	}), []middleware.SkillSource{{Path: "/s/SKILL.md"}})

	state, _ := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
	prompt := state.SystemPromptExtensions[0]
	if !strings.Contains(prompt, "read_file") || !strings.Contains(prompt, "execute") {
		t.Errorf("allowed tools missing from prompt: %q", prompt)
	}
}

func TestSkills_LastWinsOnNameCollision(t *testing.T) {
	first := `---
name: same-skill
description: first version
---
`
	second := `---
name: same-skill
description: second version
---
`
	m := middleware.NewSkillsMiddleware(skillLoader(map[string]string{
		"/a/SKILL.md": first,
		"/b/SKILL.md": second,
	}), []middleware.SkillSource{
		{Path: "/a/SKILL.md"},
		{Path: "/b/SKILL.md"},
	})

	state, _ := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
	prompt := state.SystemPromptExtensions[0]
	if strings.Contains(prompt, "first version") {
		t.Error("first version should be overridden by second")
	}
	if !strings.Contains(prompt, "second version") {
		t.Error("second version should win")
	}
}

func TestSkills_InvalidNameSkipped(t *testing.T) {
	bad := `---
name: My Invalid Skill!
description: bad name
---
`
	m := middleware.NewSkillsMiddleware(skillLoader(map[string]string{
		"/s/SKILL.md": bad,
	}), []middleware.SkillSource{{Path: "/s/SKILL.md"}})

	state, _ := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
	if len(state.SystemPromptExtensions) != 0 {
		t.Errorf("invalid skill name should be skipped, got: %v", state.SystemPromptExtensions)
	}
}

func TestSkills_NoFrontmatterSkipped(t *testing.T) {
	m := middleware.NewSkillsMiddleware(skillLoader(map[string]string{
		"/s/SKILL.md": "just plain text, no frontmatter",
	}), []middleware.SkillSource{{Path: "/s/SKILL.md"}})

	state, _ := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
	if len(state.SystemPromptExtensions) != 0 {
		t.Error("file without frontmatter should be skipped")
	}
}

func TestSkills_DescriptionTruncated(t *testing.T) {
	longDesc := strings.Repeat("x", 2000)
	content := "---\nname: long-desc\ndescription: " + longDesc + "\n---\n"
	m := middleware.NewSkillsMiddleware(skillLoader(map[string]string{
		"/s/SKILL.md": content,
	}), []middleware.SkillSource{{Path: "/s/SKILL.md"}})

	state, _ := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
	if len(state.SystemPromptExtensions) == 0 {
		t.Fatal("expected skill to be loaded")
	}
	// Description should be truncated to 1024 chars
	if strings.Contains(state.SystemPromptExtensions[0], strings.Repeat("x", 1025)) {
		t.Error("description not truncated to 1024 chars")
	}
}

func TestSkills_LabelDerivedFromPath(t *testing.T) {
	m := middleware.NewSkillsMiddleware(skillLoader(map[string]string{
		"/myproject/SKILL.md": minimalSkill,
	}), []middleware.SkillSource{{Path: "/myproject/SKILL.md"}}) // no Label set

	state, _ := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
	if len(state.SystemPromptExtensions) == 0 {
		t.Fatal("expected skill loaded")
	}
	if !strings.Contains(state.SystemPromptExtensions[0], "myproject") {
		t.Errorf("label not derived from parent dir: %q", state.SystemPromptExtensions[0])
	}
}

func TestSkills_LoadsOnce(t *testing.T) {
	calls := 0
	loader := middleware.FileLoaderFunc(func(_ context.Context, _ string) ([]byte, error) {
		calls++
		return []byte(validSkill), nil
	})
	m := middleware.NewSkillsMiddleware(loader, []middleware.SkillSource{{Path: "/s/SKILL.md"}})
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		m.BeforeModel(ctx, &middleware.State{Metadata: map[string]any{}})
	}
	if calls != 1 {
		t.Fatalf("expected loader called once, got %d", calls)
	}
}

func TestSkills_MissingSourceSkipped(t *testing.T) {
	m := middleware.NewSkillsMiddleware(skillLoader(map[string]string{
		"/exists/SKILL.md": minimalSkill,
	}), []middleware.SkillSource{
		{Path: "/missing/SKILL.md"},
		{Path: "/exists/SKILL.md"},
	})

	state, _ := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
	if len(state.SystemPromptExtensions) != 1 {
		t.Fatalf("expected 1 skill (missing skipped), got %d extensions", len(state.SystemPromptExtensions))
	}
}

func TestParseFrontmatter(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantName string
		wantDesc string
		wantBody string
	}{
		{
			name:     "full",
			input:    "---\nname: test-skill\ndescription: A test\n---\nbody here",
			wantName: "test-skill",
			wantDesc: "A test",
			wantBody: "body here",
		},
		{
			name:     "quoted values",
			input:    "---\nname: q-skill\ndescription: \"quoted\"\n---\n",
			wantName: "q-skill",
			wantDesc: "quoted",
		},
		{
			name:     "no frontmatter",
			input:    "plain text",
			wantName: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loader := skillLoader(map[string]string{"/s.md": tc.input})
			m := middleware.NewSkillsMiddleware(loader, []middleware.SkillSource{{Path: "/s.md", Label: "x"}})
			state, _ := m.BeforeModel(context.Background(), &middleware.State{Metadata: map[string]any{}})
			if tc.wantName == "" {
				if len(state.SystemPromptExtensions) != 0 {
					t.Errorf("expected no skill, got: %v", state.SystemPromptExtensions)
				}
				return
			}
			if len(state.SystemPromptExtensions) == 0 {
				t.Fatal("expected skill in prompt")
			}
			if !strings.Contains(state.SystemPromptExtensions[0], tc.wantName) {
				t.Errorf("name %q not in prompt", tc.wantName)
			}
			if tc.wantDesc != "" && !strings.Contains(state.SystemPromptExtensions[0], tc.wantDesc) {
				t.Errorf("desc %q not in prompt", tc.wantDesc)
			}
		})
	}
}
