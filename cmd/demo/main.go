package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/openai/openai-go/option"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/agent"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/middleware"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/model"
	modelopenai "github.com/anomalyco/open-swe/agent-runtime/pkg/model/openai"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox/local"
	sshsandbox "github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox/ssh"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool/builtin"
)

func main() {
	task        := flag.String("task", "", "Task for the agent to perform")
	sandboxDir  := flag.String("dir", "", "Working directory for the sandbox (default: temp dir, ignored for ssh)")
	sandboxType := flag.String("sandbox", "local", "Sandbox type: local or ssh (ssh reads SSH_HOST/SSH_USER/SSH_PASSWORD/SSH_KEY_PATH/SSH_DIR from env)")
	modelID     := flag.String("model", "", "Model ID to use (default: deepseek-chat with DEEPSEEK_API_KEY, otherwise gpt-4o)")
	stream      := flag.Bool("stream", true, "Use streaming mode")
	maxIter     := flag.Int("max-iter", 20, "Maximum agent loop iterations")
	memoryFile  := flag.String("memory-file", "", "Local AGENTS.md path to inject into the system prompt (MemoryMiddleware)")
	aclDeny     := flag.String("acl-deny", "", "Glob pattern to deny all file operations, e.g. /etc/** (FilesystemACLMiddleware)")
	skillsFile  := flag.String("skills-file", "", "Local SKILL.md path to load skills from (SkillsMiddleware)")
	hitl        := flag.Bool("hitl", false, "Enable human-in-the-loop approval for write_file/edit_file ops via stdin")
	flag.Parse()

	if *task == "" {
		log.Fatal("Usage: demo -task \"...\" [-sandbox local|ssh] [-dir /path] [-model id] [-stream true]")
	}

	profile, err := model.ResolveProviderProfileFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if *modelID == "" {
		*modelID = profile.DefaultModel
	}

	var sbx sandbox.Sandbox
	var displayDir string

	switch *sandboxType {
	case "ssh":
		s, err := sshsandbox.NewFromEnv()
		if err != nil {
			log.Fatalf("create ssh sandbox: %v", err)
		}
		sbx = s
		displayDir = os.Getenv("SSH_DIR")
		if displayDir == "" {
			displayDir = "(remote home)"
		}
	default:
		dir := *sandboxDir
		if dir == "" {
			tmp, err := os.MkdirTemp("", "agent-demo-*")
			if err != nil {
				log.Fatalf("create temp dir: %v", err)
			}
			dir = tmp
			defer os.RemoveAll(tmp)
		}
		s, err := local.New(dir)
		if err != nil {
			log.Fatalf("create sandbox: %v", err)
		}
		absDir, _ := filepath.Abs(dir)
		displayDir = absDir
		sbx = s
	}
	defer sbx.Close(context.Background())

	registry := tool.NewRegistry()
	registry.Register(builtin.NewExecuteTool(sbx))
	registry.Register(builtin.NewLsTool(sbx))
	registry.Register(builtin.NewReadFileTool(sbx))
	registry.Register(builtin.NewWriteFileTool(sbx))
	registry.Register(builtin.NewEditFileTool(sbx))
	registry.Register(builtin.NewGlobTool(sbx))
	registry.Register(builtin.NewGrepTool(sbx))

	systemPrompt := fmt.Sprintf(`You are an AI coding agent. You can execute shell commands, list directories, search files, read files, write files, and edit files.

Working directory: %s

Complete the user's task using the available tools. After completing the task, provide a brief summary of what you did.`, displayDir)

	providerOpts := []option.RequestOption{}
	if profile.BaseURL != "" {
		providerOpts = append(providerOpts, option.WithBaseURL(profile.BaseURL))
	}
	provider := modelopenai.NewProvider(profile.APIKey, providerOpts...)

	retentionStore := agent.NewSandboxRetentionStore(sbx, sbx, 20)

	agentOpts := []agent.Option{
		agent.WithModelID(*modelID),
		agent.WithSystemPrompt(systemPrompt),
		agent.WithMaxIterations(*maxIter),
		agent.WithMaxTokens(4096),
		agent.WithMiddleware(middleware.NewCallLimit(50)),
		agent.WithMiddleware(middleware.NewErrorHandler()),
		agent.WithResultOffload(retentionStore, 80_000),
	}

	if *memoryFile != "" {
		agentOpts = append(agentOpts, agent.WithMiddleware(
			middleware.NewMemoryMiddleware(middleware.LocalFileLoader{}, []string{*memoryFile}),
		))
	}
	if *aclDeny != "" {
		agentOpts = append(agentOpts, agent.WithMiddleware(
			middleware.NewFilesystemACL([]middleware.Permission{
				{Pattern: *aclDeny, Operations: middleware.OpAll, Allow: false},
			}),
		))
	}
	if *skillsFile != "" {
		agentOpts = append(agentOpts, agent.WithMiddleware(
			middleware.NewSkillsMiddleware(middleware.LocalFileLoader{}, []middleware.SkillSource{
				{Path: *skillsFile},
			}),
		))
	}
	if *hitl {
		agentOpts = append(agentOpts, agent.WithMiddleware(
			middleware.NewHumanInTheLoop(StdinApprovalGate{}, middleware.TriggerOnWriteOps()),
		))
	}

	ag := agent.New(provider, registry, agentOpts...)

	fmt.Printf("=== Agent Demo ===\n")
	fmt.Printf("Task:      %s\n", *task)
	fmt.Printf("Sandbox:   %s (%s)\n", *sandboxType, displayDir)
	fmt.Printf("Provider:  %s\n", profile.Name)
	fmt.Printf("Model:     %s\n", *modelID)
	fmt.Printf("Mode:      %s\n", map[bool]string{true: "streaming", false: "complete"}[*stream])
	fmt.Printf("Middleware:")
	if *memoryFile != "" {
		fmt.Printf(" memory(%s)", *memoryFile)
	}
	if *aclDeny != "" {
		fmt.Printf(" acl-deny(%s)", *aclDeny)
	}
	if *skillsFile != "" {
		fmt.Printf(" skills(%s)", *skillsFile)
	}
	if *hitl {
		fmt.Printf(" hitl(write-ops)")
	}
	fmt.Printf("\n\n")

	input := agent.Input{
		Messages: []model.Message{
			{Role: model.RoleUser, Content: *task},
		},
	}

	ctx := context.Background()

	if *stream {
		runStreaming(ctx, ag, input)
	} else {
		runComplete(ctx, ag, input)
	}

	if *sandboxType == "local" {
		fmt.Printf("\n=== Sandbox contents ===\n")
		printDir(displayDir, "")
	}
}

func runStreaming(ctx context.Context, ag *agent.Agent, input agent.Input) {
	events, err := ag.RunStreaming(ctx, input)
	if err != nil {
		log.Fatalf("start agent: %v", err)
	}

	for event := range events {
		switch event.Type {
		case "text_delta":
			fmt.Print(event.Content)
		case "text":
			fmt.Printf("\n[assistant] %s\n", event.Content)
		case "tool_call":
			if event.ToolCall != nil {
				fmt.Printf("\n[tool call] %s(%s)\n", event.ToolCall.Name, truncate(event.ToolCall.Args, 120))
			}
		case "tool_executing":
			if event.ToolCall != nil {
				fmt.Printf("[executing] %s...\n", event.ToolCall.Name)
			}
		case "tool_result":
			if event.ToolResult != nil {
				output := event.ToolResult.Output
				if len(output) > 200 {
					output = output[:200] + "..."
				}
				fmt.Printf("[result] %s: %s\n", event.ToolResult.Name, output)
			}
		case "completed":
			fmt.Printf("\n=== Completed ===\n%s\n", event.Content)
		case "error":
			fmt.Printf("\n=== Error ===\n%s\n", event.Content)
		}
	}
}

func runComplete(ctx context.Context, ag *agent.Agent, input agent.Input) {
	events, err := ag.Run(ctx, input)
	if err != nil {
		log.Fatalf("start agent: %v", err)
	}

	for event := range events {
		switch event.Type {
		case "model_call_start":
			fmt.Print("[thinking...")
		case "model_call_end":
			fmt.Print("]\n")
			if event.Usage != nil {
				fmt.Printf("[tokens: in=%d out=%d]\n", event.Usage.InputTokens, event.Usage.OutputTokens)
			}
		case "text":
			fmt.Printf("[assistant] %s\n", event.Content)
		case "tool_call":
			if event.ToolCall != nil {
				fmt.Printf("[tool call] %s(%s)\n", event.ToolCall.Name, truncate(event.ToolCall.Args, 120))
			}
		case "tool_result":
			if event.ToolResult != nil {
				output := event.ToolResult.Output
				if len(output) > 200 {
					output = output[:200] + "..."
				}
				fmt.Printf("[result] %s: %s\n", event.ToolResult.Name, output)
			}
		case "completed":
			fmt.Printf("\n=== Completed ===\n%s\n", event.Content)
		case "error":
			fmt.Printf("\n=== Error ===\n%s\n", event.Content)
		}
	}
}

func printDir(dir, prefix string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			fmt.Printf("%s%s/\n", prefix, e.Name())
			printDir(path, prefix+"  ")
		} else {
			info, _ := e.Info()
			data, _ := os.ReadFile(path)
			content := string(data)
			if len(content) > 100 {
				content = content[:100] + "..."
			}
			fmt.Printf("%s%s (%d bytes): %s\n", prefix, e.Name(), info.Size(), strings.TrimSpace(content))
		}
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func mustMarshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// StdinApprovalGate implements middleware.ApprovalGate via stdin.
// When HITL triggers, prints the tool name + args and waits for "y" to approve.
type StdinApprovalGate struct{}

func (StdinApprovalGate) RequestApproval(ctx context.Context, toolName string, args json.RawMessage) (bool, string, error) {
	fmt.Printf("\n[HITL] %s(%s)\nApprove? [y/N]: ", toolName, args)
	ch := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			ch <- strings.TrimSpace(scanner.Text())
		} else {
			ch <- ""
		}
	}()
	select {
	case <-ctx.Done():
		return false, "", ctx.Err()
	case line := <-ch:
		if strings.ToLower(line) == "y" {
			return true, "", nil
		}
		return false, "operator rejected", nil
	}
}
