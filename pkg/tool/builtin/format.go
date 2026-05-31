package builtin

import (
	"strings"
)

const (
	maxToolOutputChars    = 80_000
	maxExecuteOutputChars = 100_000
)

const (
	listTruncationMessage = "... [results truncated, try being more specific with your parameters]"
	readTruncationMessage = "[Output was truncated due to size limits. The file content is very large. Consider reformatting the file to make it easier to navigate...]"
	execTruncationMessage = "[Output was truncated due to size limits]"
)

func formatPythonList(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, value := range values {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('\'')
		b.WriteString(strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "'", "\\'"))
		b.WriteByte('\'')
	}
	b.WriteByte(']')
	return b.String()
}

func trimTrailingNewline(b *strings.Builder) {
	s := b.String()
	if !strings.HasSuffix(s, "\n") {
		return
	}
	b.Reset()
	b.WriteString(strings.TrimSuffix(s, "\n"))
}

func truncateListOutput(content string) string {
	return truncateWithMessage(content, maxToolOutputChars, listTruncationMessage)
}

func truncateReadOutput(content string) string {
	return truncateWithMessage(content, maxToolOutputChars, readTruncationMessage)
}

func truncateExecuteOutput(content string) string {
	return truncateWithMessage(content, maxExecuteOutputChars, execTruncationMessage)
}

func truncateWithMessage(content string, maxChars int, message string) string {
	if maxChars <= 0 || len(content) <= maxChars {
		return content
	}
	budget := maxChars - len(message) - 1
	if budget < 0 {
		budget = 0
	}
	truncated := content[:budget]
	truncated = strings.TrimRight(truncated, "\n")
	if truncated != "" {
		truncated += "\n"
	}
	return truncated + message
}
