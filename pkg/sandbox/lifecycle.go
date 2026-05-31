package sandbox

import "context"

type Reconnector interface {
	Reconnect(ctx context.Context, externalID string) (Sandbox, error)
}

type Recreator interface {
	Recreate(ctx context.Context, previous Sandbox) (Sandbox, error)
}

type GitHubProxyConfigurer interface {
	ConfigureGitHubProxy(ctx context.Context, sandbox Sandbox, token string) error
}

type ProviderFactory interface {
	Create(ctx context.Context) (Sandbox, error)
}
