package middleware

import (
	"context"
	"os"
	"regexp"
	"strings"
	"sync"
)

// FileLoader loads a file by path. Implement with a sandbox or local fs.
type FileLoader interface {
	Load(ctx context.Context, path string) ([]byte, error)
}

// MemoryMiddleware reads AGENTS.md files from configured sources and injects
// their content into the system prompt before every model call.
// Files are loaded lazily on the first BeforeModel call and cached for the run.
type MemoryMiddleware struct {
	noopMiddleware
	loader           FileLoader
	sources          []string
	stripHTMLComment bool

	once     sync.Once
	contents []string
}

type MemoryOption func(*MemoryMiddleware)

// WithHTMLCommentStripping controls whether <!-- ... --> blocks are removed (default true).
func WithHTMLCommentStripping(strip bool) MemoryOption {
	return func(m *MemoryMiddleware) { m.stripHTMLComment = strip }
}

// NewMemoryMiddleware creates a MemoryMiddleware that loads each path in sources
// via loader and appends the content to the system prompt.
func NewMemoryMiddleware(loader FileLoader, sources []string, opts ...MemoryOption) *MemoryMiddleware {
	m := &MemoryMiddleware{
		loader:           loader,
		sources:          sources,
		stripHTMLComment: true,
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

var htmlCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)

func (m *MemoryMiddleware) BeforeModel(ctx context.Context, state *State) (*State, error) {
	m.once.Do(func() { m.load(ctx) })
	state.SystemPromptExtensions = append(state.SystemPromptExtensions, m.contents...)
	return state, nil
}

func (m *MemoryMiddleware) load(ctx context.Context) {
	for _, src := range m.sources {
		data, err := m.loader.Load(ctx, src)
		if err != nil {
			continue
		}
		content := string(data)
		if m.stripHTMLComment {
			content = htmlCommentRe.ReplaceAllString(content, "")
		}
		content = strings.TrimSpace(content)
		if content != "" {
			m.contents = append(m.contents, content)
		}
	}
}

// AfterModel, BeforeTool, AfterTool are no-ops (inherited from noopMiddleware).
var _ Middleware = (*MemoryMiddleware)(nil)

// LocalFileLoader loads files from the local filesystem.
// For sandbox-backed loading, implement FileLoader inline:
//
//	FileLoaderFunc(func(ctx context.Context, path string) ([]byte, error) {
//	    r, err := sbx.ReadFile(ctx, path, 0, 0)
//	    if err != nil || r.Error != "" { return nil, errors.New(r.Error) }
//	    return []byte(r.Content), nil
//	})
type LocalFileLoader struct{}

func (LocalFileLoader) Load(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

// FileLoaderFunc is a convenience adapter for one-off FileLoader implementations.
type FileLoaderFunc func(ctx context.Context, path string) ([]byte, error)

func (f FileLoaderFunc) Load(ctx context.Context, path string) ([]byte, error) { return f(ctx, path) }
