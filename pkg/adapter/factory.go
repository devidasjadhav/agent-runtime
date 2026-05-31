package adapter

import (
	"context"
	"sync"

	"github.com/anomalyco/open-swe/agent-runtime/pkg/agent"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/middleware"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/model"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/sandbox"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool"
	"github.com/anomalyco/open-swe/agent-runtime/pkg/tool/builtin"
)

type AgentFactory struct {
	mu              sync.Mutex
	provider        model.Provider
	fallback        model.Provider
	registry        *tool.Registry
	sandboxFactory  sandbox.ProviderFactory
	reconnector     sandbox.Reconnector
	proxyConfigurer sandbox.GitHubProxyConfigurer
	resultStore     agent.ResultStore
	checkpoints     CheckpointStore
	queue           MessageQueue
	eventSink       EventSink
	teamDefaults    map[string]string
}

func NewAgentFactory(
	provider model.Provider,
	registry *tool.Registry,
	opts ...FactoryOption,
) *AgentFactory {
	f := &AgentFactory{
		provider:     provider,
		registry:     registry,
		teamDefaults: make(map[string]string),
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

type FactoryOption func(*AgentFactory)

func WithFallbackProvider(p model.Provider) FactoryOption {
	return func(f *AgentFactory) { f.fallback = p }
}

func WithSandboxFactory(sf sandbox.ProviderFactory) FactoryOption {
	return func(f *AgentFactory) { f.sandboxFactory = sf }
}

func WithReconnector(r sandbox.Reconnector) FactoryOption {
	return func(f *AgentFactory) { f.reconnector = r }
}

func WithGitHubProxyConfigurer(c sandbox.GitHubProxyConfigurer) FactoryOption {
	return func(f *AgentFactory) { f.proxyConfigurer = c }
}

func WithResultStore(s agent.ResultStore) FactoryOption {
	return func(f *AgentFactory) { f.resultStore = s }
}

func WithCheckpointStore(s CheckpointStore) FactoryOption {
	return func(f *AgentFactory) { f.checkpoints = s }
}

func WithMessageQueue(q MessageQueue) FactoryOption {
	return func(f *AgentFactory) { f.queue = q }
}

func WithEventSink(s EventSink) FactoryOption {
	return func(f *AgentFactory) { f.eventSink = s }
}

func WithTeamDefaults(defaults map[string]string) FactoryOption {
	return func(f *AgentFactory) {
		for k, v := range defaults {
			f.teamDefaults[k] = v
		}
	}
}

func applyMiddlewares(opts []agent.Option, chain middleware.Chain) []agent.Option {
	for _, m := range chain {
		opts = append(opts, agent.WithMiddleware(m))
	}
	return opts
}

func (f *AgentFactory) CreateCodingAgent(ctx context.Context, cfg AgentConfig, sb sandbox.Sandbox) (*AgentHandle, error) {
	registry := tool.NewRegistry()

	for _, t := range f.registry.All() {
		registry.Register(t)
	}

	todoState := &builtin.TodoState{}
	registry.Register(builtin.NewTodoTool(todoState))

	taskTool := builtin.NewTaskTool(
		f.provider,
		registry,
		cfg.SubagentModelID,
		cfg.MaxTokens,
	)
	registry.Register(taskTool)

	chain := middleware.Chain{
		middleware.SanitizeInputs{},
		middleware.NewCallLimit(cfg.MaxIterations),
		middleware.NewThinkingBlockSanitizer(true),
	}

	agentOpts := []agent.Option{
		agent.WithModelID(cfg.ModelID),
		agent.WithMaxTokens(cfg.MaxTokens),
		agent.WithMaxIterations(cfg.MaxIterations),
	}
	agentOpts = applyMiddlewares(agentOpts, chain)

	if f.fallback != nil && cfg.FallbackModelID != "" {
		agentOpts = append(agentOpts, agent.WithFallbackProvider(f.fallback, cfg.FallbackModelID))
	}

	if f.resultStore != nil {
		agentOpts = append(agentOpts, agent.WithResultOffload(f.resultStore, 80_000))
	}

	a := agent.New(f.provider, registry, agentOpts...)

	return &AgentHandle{
		Agent:       a,
		Sandbox:     sb,
		TodoState:   todoState,
		Checkpoints: f.checkpoints,
		EventSink:   f.eventSink,
	}, nil
}

func (f *AgentFactory) CreateReviewerAgent(ctx context.Context, cfg ReviewConfig, sb sandbox.Sandbox) (*AgentHandle, error) {
	registry := tool.NewRegistry()

	registry.Register(builtin.NewAddFindingTool())
	registry.Register(builtin.NewUpdateFindingTool())
	registry.Register(builtin.NewListFindingsTool())
	registry.Register(builtin.NewPublishReviewTool())
	registry.Register(builtin.NewResolveFindingThreadTool())
	registry.Register(builtin.NewReplyToFindingThreadTool())

	for _, t := range f.registry.All() {
		if t.Name() == "web_search" || t.Name() == "fetch_url" || t.Name() == "http_request" {
			registry.Register(t)
		}
	}

	chain := middleware.Chain{
		middleware.SanitizeInputs{},
		middleware.NewCallLimit(DefaultReviewerMaxIterations),
		middleware.NewThinkingBlockSanitizer(true),
	}

	agentOpts := []agent.Option{
		agent.WithModelID(cfg.ModelID),
		agent.WithMaxTokens(DefaultMaxTokens),
		agent.WithMaxIterations(DefaultReviewerMaxIterations),
	}
	agentOpts = applyMiddlewares(agentOpts, chain)

	a := agent.New(f.provider, registry, agentOpts...)

	return &AgentHandle{
		Agent:       a,
		Sandbox:     sb,
		Checkpoints: f.checkpoints,
		EventSink:   f.eventSink,
	}, nil
}

func (f *AgentFactory) CreateStyleAnalyzer(ctx context.Context, cfg StyleAnalyzerConfig, sb sandbox.Sandbox) (*AgentHandle, error) {
	registry := tool.NewRegistry()
	registry.Register(builtin.NewSaveReviewStylePromptTool())

	chain := middleware.Chain{
		middleware.SanitizeInputs{},
		middleware.NewCallLimit(StyleAnalyzerMaxCallLimit),
		middleware.NewThinkingBlockSanitizer(true),
	}

	agentOpts := []agent.Option{
		agent.WithModelID(DefaultModelID),
		agent.WithMaxTokens(DefaultMaxTokens),
		agent.WithMaxIterations(StyleAnalyzerMaxCallLimit),
	}
	agentOpts = applyMiddlewares(agentOpts, chain)

	a := agent.New(f.provider, registry, agentOpts...)

	return &AgentHandle{
		Agent:     a,
		Sandbox:   sb,
		EventSink: f.eventSink,
	}, nil
}

func (f *AgentFactory) TeamDefault(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.teamDefaults[key]
}

func (f *AgentFactory) SetTeamDefault(key, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.teamDefaults[key] = value
}
