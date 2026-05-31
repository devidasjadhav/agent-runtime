package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
)

type TodoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

type TodoState struct {
	mu    sync.Mutex
	Items []TodoItem `json:"items"`
}

func (s *TodoState) Update(items []TodoItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Items = items
}

func (s *TodoState) Get() []TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TodoItem, len(s.Items))
	copy(out, s.Items)
	return out
}

type TodoTool struct {
	state *TodoState
}

func NewTodoState() *TodoState { return &TodoState{} }

func NewTodoTool(state *TodoState) *TodoTool {
	return &TodoTool{state: state}
}

func (t *TodoTool) Name() string { return "write_todos" }

func (t *TodoTool) Description() string {
	return "Update the todo list. Provide the full list of todos with their statuses. Use this to track progress on complex multi-step tasks."
}

func (t *TodoTool) Parameters() tool.ToolSchema {
	return tool.ObjectSchema(
		[]string{"todos"},
		map[string]tool.ToolPropertySchema{
			"todos": {
				Type:        "array",
				Description: "The full list of todo items. Each item has 'content' (string) and 'status' (one of 'pending', 'in_progress', 'completed').",
				Items: &tool.ToolPropertySchema{
					Type: "object",
				},
			},
		},
	)
}

func (t *TodoTool) Execute(_ context.Context, args json.RawMessage) (tool.Result, error) {
	var input struct {
		Todos []TodoItem `json:"todos"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tool.Result{Content: fmt.Sprintf("Error: invalid arguments: %v", err), Error: true}, nil
	}

	validStatuses := map[string]bool{"pending": true, "in_progress": true, "completed": true}
	for i, item := range input.Todos {
		if strings.TrimSpace(item.Content) == "" {
			return tool.Result{Content: fmt.Sprintf("Error: todo item %d has empty content", i), Error: true}, nil
		}
		if !validStatuses[item.Status] {
			return tool.Result{Content: fmt.Sprintf("Error: todo item %d has invalid status %q (must be pending, in_progress, or completed)", i, item.Status), Error: true}, nil
		}
	}

	t.state.Update(input.Todos)

	var lines []string
	for _, item := range input.Todos {
		icon := "○"
		switch item.Status {
		case "in_progress":
			icon = "◉"
		case "completed":
			icon = "●"
		}
		lines = append(lines, fmt.Sprintf("%s %s", icon, item.Content))
	}

	summary := "Todo list updated."
	if len(input.Todos) > 0 {
		pending := 0
		inProgress := 0
		completed := 0
		for _, item := range input.Todos {
			switch item.Status {
			case "pending":
				pending++
			case "in_progress":
				inProgress++
			case "completed":
				completed++
			}
		}
		summary = fmt.Sprintf("Todo list updated (%d pending, %d in progress, %d completed):\n%s",
			pending, inProgress, completed, strings.Join(lines, "\n"))
	}

	return tool.Result{Content: summary}, nil
}

func FormatTodos(items []TodoItem) string {
	if len(items) == 0 {
		return "No todos."
	}
	statusOrder := map[string]int{"in_progress": 0, "pending": 1, "completed": 2}
	sorted := make([]TodoItem, len(items))
	copy(sorted, items)
	sort.SliceStable(sorted, func(i, j int) bool {
		return statusOrder[sorted[i].Status] < statusOrder[sorted[j].Status]
	})
	var lines []string
	for _, item := range sorted {
		icon := "○"
		switch item.Status {
		case "in_progress":
			icon = "◉"
		case "completed":
			icon = "●"
		}
		lines = append(lines, fmt.Sprintf("%s %s", icon, item.Content))
	}
	return strings.Join(lines, "\n")
}
