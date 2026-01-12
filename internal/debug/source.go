package debug

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// SourceManager handles source file operations
type SourceManager struct {
	session    *Session
	cache      map[string]string // scriptID -> source
	fileCache  map[string]string // file path -> source
	mu         sync.RWMutex
	workingDir string
}

// NewSourceManager creates a new source manager
func NewSourceManager(session *Session, workingDir string) *SourceManager {
	return &SourceManager{
		session:    session,
		cache:      make(map[string]string),
		fileCache:  make(map[string]string),
		workingDir: workingDir,
	}
}

// GetSource gets the source code for a script
func (m *SourceManager) GetSource(scriptID string) (string, error) {
	m.mu.RLock()
	if source, ok := m.cache[scriptID]; ok {
		m.mu.RUnlock()
		return source, nil
	}
	m.mu.RUnlock()

	// Fetch from debugger
	params := map[string]any{
		"scriptId": scriptID,
	}

	result, err := m.session.Call("Debugger.getScriptSource", params)
	if err != nil {
		return "", fmt.Errorf("failed to get source: %w", err)
	}

	var response struct {
		ScriptSource string `json:"scriptSource"`
		Bytecode     string `json:"bytecode,omitempty"`
	}

	if err := json.Unmarshal(result, &response); err != nil {
		return "", fmt.Errorf("failed to parse source: %w", err)
	}

	m.mu.Lock()
	m.cache[scriptID] = response.ScriptSource
	m.mu.Unlock()

	return response.ScriptSource, nil
}

// GetSourceLines gets source lines for a script
func (m *SourceManager) GetSourceLines(scriptID string, startLine, endLine int) ([]string, error) {
	source, err := m.GetSource(scriptID)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(source, "\n")

	if startLine < 0 {
		startLine = 0
	}
	if endLine > len(lines) || endLine < 0 {
		endLine = len(lines)
	}
	if startLine >= len(lines) {
		return nil, nil
	}

	return lines[startLine:endLine], nil
}

// GetSourceLine gets a single source line
func (m *SourceManager) GetSourceLine(scriptID string, lineNumber int) (string, error) {
	lines, err := m.GetSourceLines(scriptID, lineNumber, lineNumber+1)
	if err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "", nil
	}
	return lines[0], nil
}

// GetSourceContext gets source lines around a specific line
func (m *SourceManager) GetSourceContext(scriptID string, lineNumber int, contextLines int) ([]SourceLine, error) {
	startLine := lineNumber - contextLines
	if startLine < 0 {
		startLine = 0
	}
	endLine := lineNumber + contextLines + 1

	source, err := m.GetSource(scriptID)
	if err != nil {
		return nil, err
	}

	allLines := strings.Split(source, "\n")
	if endLine > len(allLines) {
		endLine = len(allLines)
	}

	result := make([]SourceLine, 0, endLine-startLine)
	for i := startLine; i < endLine; i++ {
		result = append(result, SourceLine{
			Number:    i + 1, // 1-indexed for display
			Content:   allLines[i],
			IsCurrent: i == lineNumber,
		})
	}

	return result, nil
}

// SourceLine represents a line of source code
type SourceLine struct {
	Number       int
	Content      string
	IsCurrent    bool
	HasBreakpoint bool
}

// GetLocalFile reads a source file from the local filesystem
func (m *SourceManager) GetLocalFile(filePath string) (string, error) {
	m.mu.RLock()
	if source, ok := m.fileCache[filePath]; ok {
		m.mu.RUnlock()
		return source, nil
	}
	m.mu.RUnlock()

	// Resolve path
	path := filePath
	if !filepath.IsAbs(path) {
		path = filepath.Join(m.workingDir, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	source := string(data)

	m.mu.Lock()
	m.fileCache[filePath] = source
	m.mu.Unlock()

	return source, nil
}

// GetLocalFileLines reads lines from a local file
func (m *SourceManager) GetLocalFileLines(filePath string, startLine, endLine int) ([]SourceLine, error) {
	source, err := m.GetLocalFile(filePath)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(source, "\n")

	if startLine < 0 {
		startLine = 0
	}
	if endLine > len(lines) || endLine < 0 {
		endLine = len(lines)
	}
	if startLine >= len(lines) {
		return nil, nil
	}

	result := make([]SourceLine, 0, endLine-startLine)
	for i := startLine; i < endLine; i++ {
		result = append(result, SourceLine{
			Number:  i + 1,
			Content: lines[i],
		})
	}

	return result, nil
}

// ResolveScriptPath resolves a script URL to a local file path
func (m *SourceManager) ResolveScriptPath(scriptURL string) string {
	// Remove file:// prefix if present
	if strings.HasPrefix(scriptURL, "file://") {
		return scriptURL[7:]
	}

	// Try to resolve relative to working directory
	if !filepath.IsAbs(scriptURL) {
		return filepath.Join(m.workingDir, scriptURL)
	}

	return scriptURL
}

// FindScriptByPath finds a script by its file path
func (m *SourceManager) FindScriptByPath(path string) *Script {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	// Use thread-safe method to get scripts
	scripts := m.session.state.GetAllScripts()
	for _, script := range scripts {
		scriptPath := m.ResolveScriptPath(script.URL)
		if scriptPath == absPath || script.URL == path {
			return script
		}
	}

	return nil
}

// GetBreakpointsForFile gets breakpoints for a file with line info
func (m *SourceManager) GetBreakpointsForFile(filePath string) map[int]bool {
	breakpoints := m.session.state.GetAllBreakpoints()
	result := make(map[int]bool)

	absPath, _ := filepath.Abs(filePath)

	for _, bp := range breakpoints {
		bpPath := m.ResolveScriptPath(bp.ScriptURL)
		if bpPath == absPath || bp.ScriptURL == filePath {
			result[bp.LineNumber] = bp.Enabled
		}
	}

	return result
}

// CountLines counts the number of lines in a script
func (m *SourceManager) CountLines(scriptID string) (int, error) {
	source, err := m.GetSource(scriptID)
	if err != nil {
		return 0, err
	}

	scanner := bufio.NewScanner(strings.NewReader(source))
	count := 0
	for scanner.Scan() {
		count++
	}

	return count, nil
}

// ClearCache clears the source cache
func (m *SourceManager) ClearCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = make(map[string]string)
	m.fileCache = make(map[string]string)
}

// InvalidateScript removes a script from the cache
func (m *SourceManager) InvalidateScript(scriptID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cache, scriptID)
}
