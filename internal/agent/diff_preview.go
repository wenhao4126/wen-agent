// Package agent — diff_preview tool: previews what a SEARCH/REPLACE edit
// would produce before applying it. Uses the diff package to render a unified
// diff between the current file content and the edited version, so the model
// can verify the edit is correct before committing to write_file or edit_file.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wenhao/internal/diff"
)

// DiffPreviewTool previews an edit without applying it. It reads the current
// file, applies the SEARCH/REPLACE transformation in memory, and returns the
// unified diff. The model can use this to verify an edit before committing.
type DiffPreviewTool struct {
	workDir string // for resolving relative paths, empty = cwd
}

// NewDiffPreviewTool creates a diff_preview tool.
func NewDiffPreviewTool(workDir string) *DiffPreviewTool {
	return &DiffPreviewTool{workDir: workDir}
}

func (t *DiffPreviewTool) Name() string { return "diff_preview" }

func (t *DiffPreviewTool) Description() string {
	return "Preview what a SEARCH/REPLACE edit will do before applying it. " +
		"Shows the unified diff that would result from replacing the SEARCH text with the REPLACE text in the given file. " +
		"The file is NOT modified — this is purely a preview. " +
		"Use this when you're uncertain about an edit: preview the diff first, verify it looks correct, then use edit_file to apply it. " +
		"Only works for single SEARCH/REPLACE operations; for multi-edit use multi_edit directly."
}

func (t *DiffPreviewTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "file_path":{"type":"string","description":"Path to the file to preview the edit on."},
  "search":{"type":"string","description":"The exact text to find in the file (same as SEARCH in edit_file)."},
  "replace":{"type":"string","description":"The text to replace it with (same as REPLACE in edit_file)."}
},
"required":["file_path","search","replace"]
}`)
}

func (t *DiffPreviewTool) ReadOnly() bool { return true }

func (t *DiffPreviewTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		FilePath string `json:"file_path"`
		Search   string `json:"search"`
		Replace  string `json:"replace"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	path := p.FilePath
	if !filepath.IsAbs(path) && t.workDir != "" {
		path = filepath.Join(t.workDir, path)
	}

	// Path safety: resolve symlinks and reject paths that escape the workspace.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("resolve symlinks: %w", err)
	}
	if err == nil {
		absPath = resolved
	}
	// Reject paths with ".." segments that try to escape.
	if !filepath.IsLocal(p.FilePath) {
		return "", fmt.Errorf("path traversal not allowed: %q", p.FilePath)
	}

	// Read the current file.
	oldText, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	// Apply SEARCH/REPLACE in memory.
	oldStr := string(oldText)
	if !strings.Contains(oldStr, p.Search) {
		return "", fmt.Errorf("SEARCH text not found in file %q — nothing to preview. The SEARCH block must match the file content exactly (same whitespace, indentation, blank lines).", p.FilePath)
	}

	// Only replace the first occurrence (matching edit_file behavior for single edits).
	newStr := strings.Replace(oldStr, p.Search, p.Replace, 1)

	// Build the diff.
	kind := diff.Modify
	if len(oldText) == 0 {
		kind = diff.Create
	}

	change := diff.Build(p.FilePath, oldStr, newStr, kind)
	if change.Binary {
		return "", fmt.Errorf("file appears to be binary — cannot diff_preview")
	}

	var b strings.Builder
	b.WriteString("## Diff Preview\n\n")
	b.WriteString(fmt.Sprintf("**File:** `%s`\n", p.FilePath))
	b.WriteString(fmt.Sprintf("**Change:** +%d / -%d lines\n\n", change.Added, change.Removed))

	if change.Diff != "" {
		b.WriteString("```diff\n")
		b.WriteString(change.Diff)
		b.WriteString("```\n")
	} else if change.Added == 0 && change.Removed == 0 {
		b.WriteString("(no changes — SEARCH equals REPLACE)\n")
	}

	b.WriteString("\nTo apply this change, call edit_file with the same file_path, search, and replace arguments.")
	return b.String(), nil
}
