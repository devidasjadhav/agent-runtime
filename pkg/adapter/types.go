package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type ThreadID struct {
	Source string
	Key    string
}

func ThreadIDFromLinearIssue(issueID int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("linear-issue:%d", issueID)))
	return hex.EncodeToString(h[:])
}

func ThreadIDFromGitHubIssue(issueID int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("github-issue:%d", issueID)))
	return hex.EncodeToString(h[:])
}

func ThreadIDFromSlackThread(channelID, threadTS string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", channelID, threadTS)))
	var u uuid.UUID
	copy(u[:], h[:16])
	return u.String()
}

func ThreadIDFromReviewerThread(owner, repo string, prNumber int) string {
	name := uuid.NameSpaceURL
	val := fmt.Sprintf("%s/%s/pr/%d/reviewer", owner, repo, prNumber)
	return uuid.NewSHA1(name, []byte(val)).String()
}

type RunConfig struct {
	ThreadID     string
	GraphName    string
	Configurable map[string]any
	Metadata     map[string]any
}

type AgentConfig struct {
	ModelID           string
	SubagentModelID   string
	MaxTokens         int
	ReasoningEffort   string
	ProviderProfile   string
	APIKey            string
	BaseURL           string
	FallbackModelID   string
	AlwaysCreatePRs   bool
	MaxIterations     int
}

type RepoInfo struct {
	Owner string
	Name  string
}

type UserInfo struct {
	Email       string
	GitHubLogin string
	GitHubUID   int64
}

type SandboxConfig struct {
	Type         string
	SnapshotID   string
	ThreadID     string
	GitHubToken  string
}

type Source string

const (
	SourceLinear      Source = "linear"
	SourceSlack       Source = "slack"
	SourceGitHub      Source = "github"
	SourceGitHubPush  Source = "github_push"
	SourceDashboard   Source = "dashboard"
)

type ReviewerEvent string

const (
	ReviewerEventFirstReview   ReviewerEvent = "first_review"
	ReviewerEventReReview      ReviewerEvent = "re_review"
	ReviewerEventFindingReply  ReviewerEvent = "finding_reply"
)

type ReviewConfig struct {
	Repo              RepoInfo
	PRNumber          int
	PRURL             string
	BaseSHA           string
	HeadSHA           string
	LastReviewedSHA   string
	IsReReview        bool
	ReviewerEvent     ReviewerEvent
	FindingReplyID    string
	FindingReplyAuthor string
	FindingReplyBody  string
	SlackChannel      string
	SlackThreadTS     string
	ModelID           string
	SubagentModelID   string
	ReasoningEffort   string
}

type StyleAnalyzerConfig struct {
	FullName   string
	SamplesText string
	GitHubToken string
}

type AgentRunInput struct {
	Messages []AgentMessage
}

type AgentMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type EventSink interface {
	Send(ctx context.Context, eventType string, data any) error
	Close() error
}

type SSEEvent struct {
	ID    string
	Event string
	Data  string
}

type CheckpointState struct {
	ThreadID    string
	RunID       string
	Step        int
	Messages    []any
	ToolCalls   int
	Metadata    map[string]any
}

type CheckpointStore interface {
	Save(ctx context.Context, state CheckpointState) error
	Load(ctx context.Context, threadID string) (*CheckpointState, error)
	List(ctx context.Context, threadID string) ([]CheckpointState, error)
}

type MessageQueue interface {
	Enqueue(ctx context.Context, threadID string, content string) error
	Dequeue(ctx context.Context, threadID string) ([]string, error)
	Peek(ctx context.Context, threadID string) ([]string, error)
	Len(ctx context.Context, threadID string) (int, error)
}

func AgentMessageToInput(msgs []AgentMessage) AgentRunInput {
	return AgentRunInput{Messages: msgs}
}

func FormatSourcePrefix(source Source, user *UserInfo) string {
	parts := []string{fmt.Sprintf("[source: %s]", source)}
	if user != nil {
		if user.GitHubLogin != "" {
			parts = append(parts, fmt.Sprintf("[user: %s]", user.GitHubLogin))
		}
	}
	return strings.Join(parts, " ")
}

const DefaultModelID = "openai:gpt-5.5"
const DefaultMaxTokens = 64_000
const DefaultMaxIterations = 50
const DefaultReviewerMaxIterations = 50
const StyleAnalyzerMaxCallLimit = 80

func ResolveModelID(configurable map[string]any, teamDefault, key string) string {
	if v, ok := configurable[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	if teamDefault != "" {
		return teamDefault
	}
	return DefaultModelID
}

func SplitModelID(modelID string) (provider, model string) {
	parts := strings.SplitN(modelID, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "openai", parts[0]
}
