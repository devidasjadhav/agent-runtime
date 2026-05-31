package middleware

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

const (
	skillMaxFileSize   = 10 * 1024 * 1024 // 10 MB
	skillMaxDescLen    = 1024
	skillNameMaxLen    = 64
)

var skillNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// SkillMeta holds the parsed frontmatter of a SKILL.md file.
type SkillMeta struct {
	Name         string
	Description  string
	AllowedTools []string
	FilePath     string
	Label        string // source label, e.g. "Claude", "Project"
}

// SkillSource points to a single SKILL.md file and an optional human-readable label.
type SkillSource struct {
	Path  string // path to SKILL.md
	Label string // shown in the system prompt; defaults to file's parent dir name
}

// SkillsMiddleware loads SKILL.md files, parses their YAML frontmatter, and
// injects a skill directory into the system prompt (progressive disclosure).
// Full skill content is read on-demand by the agent via read_file.
type SkillsMiddleware struct {
	noopMiddleware
	loader  FileLoader
	sources []SkillSource

	once   sync.Once
	skills []SkillMeta
}

type SkillOption func(*SkillsMiddleware)

// NewSkillsMiddleware creates a SkillsMiddleware.
// Each SkillSource.Path should point to a SKILL.md file.
// Later sources override earlier ones when skill names collide (last-wins).
func NewSkillsMiddleware(loader FileLoader, sources []SkillSource, opts ...SkillOption) *SkillsMiddleware {
	m := &SkillsMiddleware{loader: loader, sources: sources}
	for _, o := range opts {
		o(m)
	}
	return m
}

func (m *SkillsMiddleware) BeforeModel(ctx context.Context, state *State) (*State, error) {
	m.once.Do(func() { m.load(ctx) })
	if len(m.skills) > 0 {
		state.SystemPromptExtensions = append(state.SystemPromptExtensions, m.buildPrompt())
	}
	return state, nil
}

func (m *SkillsMiddleware) load(ctx context.Context) {
	seen := make(map[string]int) // name → index in m.skills (last-wins)
	for _, src := range m.sources {
		data, err := m.loader.Load(ctx, src.Path)
		if err != nil {
			continue
		}
		if len(data) > skillMaxFileSize {
			continue
		}
		meta, ok := parseSkillFile(string(data), src.Path, src.Label)
		if !ok {
			continue
		}
		if idx, exists := seen[meta.Name]; exists {
			m.skills[idx] = meta // last-wins override
		} else {
			seen[meta.Name] = len(m.skills)
			m.skills = append(m.skills, meta)
		}
	}
}

func (m *SkillsMiddleware) buildPrompt() string {
	var b strings.Builder
	b.WriteString("## Available Skills\n\n")
	b.WriteString("The following skills are available. Use read_file to get full instructions.\n\n")
	for _, s := range m.skills {
		fmt.Fprintf(&b, "### %s", s.Name)
		if s.Label != "" {
			fmt.Fprintf(&b, " (source: %s)", s.Label)
		}
		b.WriteByte('\n')
		if s.Description != "" {
			fmt.Fprintf(&b, "Description: %s\n", s.Description)
		}
		fmt.Fprintf(&b, "Path: %s\n", s.FilePath)
		if len(s.AllowedTools) > 0 {
			fmt.Fprintf(&b, "Allowed tools: %s\n", strings.Join(s.AllowedTools, ", "))
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// parseSkillFile extracts SkillMeta from a SKILL.md file's YAML frontmatter.
func parseSkillFile(content, filePath, label string) (SkillMeta, bool) {
	fm, _ := parseFrontmatter(content)
	if fm == nil {
		return SkillMeta{}, false
	}

	name, _ := fm["name"].(string)
	name = strings.TrimSpace(name)
	if !isValidSkillName(name) {
		return SkillMeta{}, false
	}

	desc, _ := fm["description"].(string)
	desc = strings.TrimSpace(desc)
	if len(desc) > skillMaxDescLen {
		desc = desc[:skillMaxDescLen]
	}

	var allowedTools []string
	if tools, ok := fm["allowed_tools"].([]string); ok {
		allowedTools = tools
	}

	if label == "" {
		label = deriveLabel(filePath)
	}

	return SkillMeta{
		Name:         name,
		Description:  desc,
		AllowedTools: allowedTools,
		FilePath:     filePath,
		Label:        label,
	}, true
}

// parseFrontmatter extracts the YAML frontmatter block from a string.
// Handles string scalars and list values (- item). Returns nil if no frontmatter.
func parseFrontmatter(content string) (map[string]any, string) {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return nil, content
	}
	content = strings.TrimPrefix(content, "---\r\n")
	content = strings.TrimPrefix(content, "---\n")

	end := strings.Index(content, "\n---")
	if end == -1 {
		return nil, content
	}
	block := content[:end]
	body := content[end:]
	body = strings.TrimPrefix(body, "\n---\n")
	body = strings.TrimPrefix(body, "\n---\r\n")
	body = strings.TrimPrefix(body, "\n---")

	result := parseSimpleYAML(block)
	return result, body
}

// parseSimpleYAML parses a minimal subset of YAML:
// scalar strings (key: value) and string lists (key:\n  - item).
func parseSimpleYAML(block string) map[string]any {
	result := make(map[string]any)
	lines := strings.Split(block, "\n")
	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			i++
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			i++
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		i++

		if value == "" {
			// Possibly a list follows
			var items []string
			for i < len(lines) {
				item := lines[i]
				trimmed := strings.TrimSpace(item)
				if strings.HasPrefix(trimmed, "- ") {
					items = append(items, strings.TrimPrefix(trimmed, "- "))
					i++
				} else if trimmed == "-" {
					i++ // empty list item, skip
				} else {
					break
				}
			}
			if len(items) > 0 {
				result[key] = items
			}
		} else {
			// Strip inline YAML quotes
			if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
				(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
				value = value[1 : len(value)-1]
			}
			result[key] = value
		}
	}
	return result
}

func isValidSkillName(name string) bool {
	if name == "" || len(name) > skillNameMaxLen {
		return false
	}
	return skillNameRe.MatchString(name)
}

func deriveLabel(filePath string) string {
	// Use the parent directory name as the label.
	parts := strings.Split(strings.ReplaceAll(filePath, "\\", "/"), "/")
	for i := len(parts) - 2; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return ""
}

var _ Middleware = (*SkillsMiddleware)(nil)
