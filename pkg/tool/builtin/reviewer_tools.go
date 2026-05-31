package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type Finding struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	FilePath    string `json:"file_path"`
	LineStart   int    `json:"line_start"`
	LineEnd     int    `json:"line_end"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
	ThreadID    string `json:"thread_id,omitempty"`
	ResolvedBy  string `json:"resolved_by,omitempty"`
}

type FindingStore struct {
	mu       sync.Mutex
	findings map[string]*Finding
	nextID   int
}

func NewFindingStore() *FindingStore {
	return &FindingStore{
		findings: make(map[string]*Finding),
		nextID:   1,
	}
}

func (s *FindingStore) Add(f *Finding) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := fmt.Sprintf("finding_%d", s.nextID)
	s.nextID++
	f.ID = id
	s.findings[id] = f
	return id
}

func (s *FindingStore) Update(id string, f *Finding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.findings[id]
	if !ok {
		return fmt.Errorf("finding %s not found", id)
	}
	if f.Title != "" {
		existing.Title = f.Title
	}
	if f.Body != "" {
		existing.Body = f.Body
	}
	if f.FilePath != "" {
		existing.FilePath = f.FilePath
	}
	if f.LineStart > 0 {
		existing.LineStart = f.LineStart
	}
	if f.LineEnd > 0 {
		existing.LineEnd = f.LineEnd
	}
	if f.Severity != "" {
		existing.Severity = f.Severity
	}
	if f.Status != "" {
		existing.Status = f.Status
	}
	return nil
}

func (s *FindingStore) List() []*Finding {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Finding
	for _, f := range s.findings {
		out = append(out, f)
	}
	return out
}

func (s *FindingStore) Get(id string) (*Finding, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.findings[id]
	return f, ok
}

func (s *FindingStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.findings = make(map[string]*Finding)
}

type AddFindingTool struct {
	store *FindingStore
}

func NewAddFindingTool() *AddFindingTool {
	return &AddFindingTool{store: nil}
}

func (t *AddFindingTool) WithStore(s *FindingStore) *AddFindingTool {
	t.store = s
	return t
}

func (t *AddFindingTool) Name() string { return "add_finding" }

func (t *AddFindingTool) Description() string {
	return "Add a new code review finding. Each finding represents an issue, suggestion, or observation about the code being reviewed."
}

func (t *AddFindingTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema(
		[]string{"title", "body"},
		map[string]tool.ToolPropertySchema{
			"title":      tool.StringProperty("Short title summarizing the finding"),
			"body":       tool.StringProperty("Detailed description of the finding"),
			"file_path":  tool.StringProperty("File path where the finding is located"),
			"line_start": tool.IntegerProperty("Start line number", 0),
			"line_end":   tool.IntegerProperty("End line number", 0),
			"severity":   tool.StringEnumProperty("Severity level", []string{"info", "warning", "error"}, "info"),
		},
	)
}

func (t *AddFindingTool) Execute(_ context.Context, args json.RawMessage) (tool.Result, error) {
	var input Finding
	if err := json.Unmarshal(args, &input); err != nil {
		return tool.Result{Content: fmt.Sprintf("Error: invalid arguments: %v", err), Error: true}, nil
	}

	if input.Title == "" {
		return tool.Result{Content: "Error: title is required", Error: true}, nil
	}
	if input.Severity == "" {
		input.Severity = "info"
	}
	input.Status = "open"

	if t.store != nil {
		id := t.store.Add(&input)
		return tool.Result{Content: fmt.Sprintf("Finding added with ID: %s", id)}, nil
	}

	return tool.Result{Content: "Finding added (no store configured)"}, nil
}

type UpdateFindingTool struct {
	store *FindingStore
}

func NewUpdateFindingTool() *UpdateFindingTool { return &UpdateFindingTool{} }

func (t *UpdateFindingTool) WithStore(s *FindingStore) *UpdateFindingTool {
	t.store = s
	return t
}

func (t *UpdateFindingTool) Name() string { return "update_finding" }

func (t *UpdateFindingTool) Description() string {
	return "Update an existing review finding. You can modify the title, body, severity, line range, or status."
}

func (t *UpdateFindingTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema(
		[]string{"finding_id"},
		map[string]tool.ToolPropertySchema{
			"finding_id": tool.StringProperty("The ID of the finding to update"),
			"title":      tool.StringProperty("Updated title"),
			"body":       tool.StringProperty("Updated description"),
			"severity":   tool.StringEnumProperty("Updated severity", []string{"info", "warning", "error"}, ""),
			"status":     tool.StringEnumProperty("Updated status", []string{"open", "resolved", "dismissed"}, ""),
			"line_start": tool.IntegerProperty("Updated start line", 0),
			"line_end":   tool.IntegerProperty("Updated end line", 0),
		},
	)
}

func (t *UpdateFindingTool) Execute(_ context.Context, args json.RawMessage) (tool.Result, error) {
	var input struct {
		FindingID string `json:"finding_id"`
		Finding
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tool.Result{Content: fmt.Sprintf("Error: invalid arguments: %v", err), Error: true}, nil
	}
	if input.FindingID == "" {
		return tool.Result{Content: "Error: finding_id is required", Error: true}, nil
	}

	if t.store != nil {
		if err := t.store.Update(input.FindingID, &input.Finding); err != nil {
			return tool.Result{Content: fmt.Sprintf("Error: %v", err), Error: true}, nil
		}
	}

	return tool.Result{Content: fmt.Sprintf("Finding %s updated", input.FindingID)}, nil
}

type ListFindingsTool struct {
	store *FindingStore
}

func NewListFindingsTool() *ListFindingsTool { return &ListFindingsTool{} }

func (t *ListFindingsTool) WithStore(s *FindingStore) *ListFindingsTool {
	t.store = s
	return t
}

func (t *ListFindingsTool) Name() string { return "list_findings" }

func (t *ListFindingsTool) Description() string {
	return "List all current review findings. Returns a summary of each finding including ID, title, severity, and status."
}

func (t *ListFindingsTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema(nil, nil)
}

func (t *ListFindingsTool) Execute(_ context.Context, _ json.RawMessage) (tool.Result, error) {
	if t.store == nil {
		return tool.Result{Content: "No findings store configured"}, nil
	}

	findings := t.store.List()
	if len(findings) == 0 {
		return tool.Result{Content: "No findings yet."}, nil
	}

	var out string
	for _, f := range findings {
		out += fmt.Sprintf("- [%s] %s (severity: %s, status: %s", f.ID, f.Title, f.Severity, f.Status)
		if f.FilePath != "" {
			out += fmt.Sprintf(", file: %s:%d-%d", f.FilePath, f.LineStart, f.LineEnd)
		}
		out += ")\n"
	}

	return tool.Result{Content: out}, nil
}

type PublishReviewTool struct {
	store       *FindingStore
	PublishFunc func(findings []*Finding) error
}

func NewPublishReviewTool() *PublishReviewTool { return &PublishReviewTool{} }

func (t *PublishReviewTool) WithStore(s *FindingStore) *PublishReviewTool {
	t.store = s
	return t
}

func (t *PublishReviewTool) Name() string { return "publish_review" }

func (t *PublishReviewTool) Description() string {
	return "Publish the review by posting all findings to the GitHub PR. This is the final step of the review process."
}

func (t *PublishReviewTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema(nil, nil)
}

func (t *PublishReviewTool) Execute(_ context.Context, _ json.RawMessage) (tool.Result, error) {
	if t.store == nil {
		return tool.Result{Content: "Error: no findings store", Error: true}, nil
	}

	findings := t.store.List()
	if len(findings) == 0 {
		return tool.Result{Content: "No findings to publish."}, nil
	}

	if t.PublishFunc != nil {
		if err := t.PublishFunc(findings); err != nil {
			return tool.Result{Content: fmt.Sprintf("Error publishing review: %v", err), Error: true}, nil
		}
	}

	return tool.Result{Content: fmt.Sprintf("Review published with %d findings.", len(findings))}, nil
}

type ResolveFindingThreadTool struct {
	store *FindingStore
}

func NewResolveFindingThreadTool() *ResolveFindingThreadTool {
	return &ResolveFindingThreadTool{}
}

func (t *ResolveFindingThreadTool) WithStore(s *FindingStore) *ResolveFindingThreadTool {
	t.store = s
	return t
}

func (t *ResolveFindingThreadTool) Name() string { return "resolve_finding_thread" }

func (t *ResolveFindingThreadTool) Description() string {
	return "Resolve or dismiss a finding thread. Marks the finding as resolved."
}

func (t *ResolveFindingThreadTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema(
		[]string{"finding_id"},
		map[string]tool.ToolPropertySchema{
			"finding_id":   tool.StringProperty("The ID of the finding to resolve"),
			"resolved_by":  tool.StringProperty("Who resolved the finding"),
		},
	)
}

func (t *ResolveFindingThreadTool) Execute(_ context.Context, args json.RawMessage) (tool.Result, error) {
	var input struct {
		FindingID  string `json:"finding_id"`
		ResolvedBy string `json:"resolved_by"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tool.Result{Content: fmt.Sprintf("Error: invalid arguments: %v", err), Error: true}, nil
	}

	if t.store != nil {
		err := t.store.Update(input.FindingID, &Finding{Status: "resolved", ResolvedBy: input.ResolvedBy})
		if err != nil {
			return tool.Result{Content: fmt.Sprintf("Error: %v", err), Error: true}, nil
		}
	}

	return tool.Result{Content: fmt.Sprintf("Finding %s resolved", input.FindingID)}, nil
}

type ReplyToFindingThreadTool struct {
	store *FindingStore
}

func NewReplyToFindingThreadTool() *ReplyToFindingThreadTool {
	return &ReplyToFindingThreadTool{}
}

func (t *ReplyToFindingThreadTool) WithStore(s *FindingStore) *ReplyToFindingThreadTool {
	t.store = s
	return t
}

func (t *ReplyToFindingThreadTool) Name() string { return "reply_to_finding_thread" }

func (t *ReplyToFindingThreadTool) Description() string {
	return "Reply to a finding thread. Use this to respond to author feedback or questions about a finding."
}

func (t *ReplyToFindingThreadTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema(
		[]string{"finding_id", "body"},
		map[string]tool.ToolPropertySchema{
			"finding_id": tool.StringProperty("The ID of the finding to reply to"),
			"body":       tool.StringProperty("The reply content"),
		},
	)
}

func (t *ReplyToFindingThreadTool) Execute(_ context.Context, args json.RawMessage) (tool.Result, error) {
	var input struct {
		FindingID string `json:"finding_id"`
		Body      string `json:"body"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tool.Result{Content: fmt.Sprintf("Error: invalid arguments: %v", err), Error: true}, nil
	}
	if input.FindingID == "" || input.Body == "" {
		return tool.Result{Content: "Error: finding_id and body are required", Error: true}, nil
	}

	return tool.Result{Content: fmt.Sprintf("Reply posted to finding %s", input.FindingID)}, nil
}

type SaveReviewStylePromptTool struct{}

func NewSaveReviewStylePromptTool() *SaveReviewStylePromptTool {
	return &SaveReviewStylePromptTool{}
}

func (t *SaveReviewStylePromptTool) Name() string { return "save_review_style_prompt" }

func (t *SaveReviewStylePromptTool) Description() string {
	return "Save the analyzed review style as a reusable prompt. Only call this after you have gathered enough evidence from historical reviews to form a comprehensive style description."
}

func (t *SaveReviewStylePromptTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema(
		[]string{"prompt"},
		map[string]tool.ToolPropertySchema{
			"prompt":     tool.StringProperty("The review style prompt to save"),
			"full_name":  tool.StringProperty("The repository full name (owner/repo) this style applies to"),
		},
	)
}

func (t *SaveReviewStylePromptTool) Execute(_ context.Context, args json.RawMessage) (tool.Result, error) {
	var input struct {
		Prompt   string `json:"prompt"`
		FullName string `json:"full_name"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tool.Result{Content: fmt.Sprintf("Error: invalid arguments: %v", err), Error: true}, nil
	}
	if input.Prompt == "" {
		return tool.Result{Content: "Error: prompt is required", Error: true}, nil
	}

	return tool.Result{Content: fmt.Sprintf("Review style prompt saved for %s", input.FullName)}, nil
}
