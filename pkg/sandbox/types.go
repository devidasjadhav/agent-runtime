package sandbox

type ExecResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
}

type ReadResult struct {
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

type WriteResult struct {
	Path  string `json:"path"`
	Error string `json:"error,omitempty"`
}

type EditResult struct {
	Path        string `json:"path"`
	Error       string `json:"error,omitempty"`
	Occurrences int    `json:"occurrences,omitempty"`
}

type LsResult struct {
	Entries []FileInfo `json:"entries,omitempty"`
	Error   string     `json:"error,omitempty"`
}

type GlobResult struct {
	Matches []FileInfo `json:"matches,omitempty"`
	Error   string     `json:"error,omitempty"`
}

type GrepResult struct {
	Matches []GrepMatch `json:"matches,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type FileUpload struct {
	Path    string
	Content []byte
}

type FileUploadResult struct {
	Path  string `json:"path"`
	Error string `json:"error,omitempty"`
}

type FileDownloadResult struct {
	Path    string `json:"path"`
	Content []byte `json:"content"`
	Error   string `json:"error,omitempty"`
}

type FileInfo struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir,omitempty"`
	Size  int64  `json:"size,omitempty"`
}

type GrepMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}
