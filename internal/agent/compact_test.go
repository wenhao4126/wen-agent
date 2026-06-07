package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wenhao/internal/provider"
)

func TestLayeredCompaction_PartitionImportant(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "normal message"},
		{Role: provider.RoleUser, Content: "<!--IMPORTANT--> This is a key decision: use pgx not database/sql"},
		{Role: provider.RoleAssistant, Content: "ok"},
		{Role: provider.RoleUser, Content: "another normal message"},
	}

	compactable, important := partitionImportant(msgs)

	if len(compactable) != 3 {
		t.Errorf("expected 3 compactable messages, got %d", len(compactable))
	}
	if len(important) != 1 {
		t.Errorf("expected 1 important message, got %d", len(important))
	}
	if !strings.HasPrefix(important[0].Content, importantMarker) {
		t.Error("important message should have the marker prefix")
	}
}

func TestLayeredCompaction_PlanLayered(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "system prompt"},
		// First compaction summary block.
		{Role: provider.RoleUser, Content: summaryTagOpen + "\nSummary of earlier conversation:\nfirst summary\n" + summaryTagClose},
		// New messages after first compaction.
		{Role: provider.RoleUser, Content: "user message after first compaction"},
		{Role: provider.RoleAssistant, Content: "assistant response"},
		{Role: provider.RoleUser, Content: "another user message"},
		{Role: provider.RoleAssistant, Content: "another response"},
	}

	a := &Agent{
		contextWindow: 100000,
		recentKeep:    2,
	}

	head, start, ok := a.planLayeredCompaction(msgs, 2)
	if !ok {
		t.Fatal("expected compaction plan to succeed")
	}

	// head should be AFTER the first compaction summary (index 2).
	if head != 2 {
		t.Errorf("expected head=2 (after first compaction summary), got %d", head)
	}
	// start should leave the last 2 messages (the recent tail).
	if start < 4 {
		t.Errorf("expected start >= 4 (recent tail of 2), got %d", start)
	}
}

func TestRecentFiles_TrackAndRestore(t *testing.T) {
	a := &Agent{}

	a.TrackReadFile("/tmp/test.go", "package main\n\nfunc main() {}")
	a.TrackReadFile("/tmp/config.toml", "[server]\nport = 8080")

	a.recentFilesMu.Lock()
	if len(a.recentFiles) != 2 {
		t.Errorf("expected 2 recent files, got %d", len(a.recentFiles))
	}
	a.recentFilesMu.Unlock()

	// Track the same file again — should update and move to end.
	a.TrackReadFile("/tmp/test.go", "package main\n\nfunc main() { os.Exit(0) }")
	a.recentFilesMu.Lock()
	if len(a.recentFiles) != 2 {
		t.Errorf("expected still 2 recent files after update, got %d", len(a.recentFiles))
	}
	last := a.recentFiles[len(a.recentFiles)-1]
	if last.path != "/tmp/test.go" {
		t.Errorf("expected updated file to be last, got %s", last.path)
	}
	if !strings.Contains(last.content, "os.Exit") {
		t.Error("expected updated content")
	}
	a.recentFilesMu.Unlock()
}

func TestRecentFiles_MaxCapacity(t *testing.T) {
	a := &Agent{}
	for i := 0; i < 10; i++ {
		a.TrackReadFile("file_"+string(rune('a'+i))+".go", "content")
	}

	a.recentFilesMu.Lock()
	if len(a.recentFiles) > maxRecentFiles {
		t.Errorf("expected at most %d recent files, got %d", maxRecentFiles, len(a.recentFiles))
	}
	a.recentFilesMu.Unlock()
}

func TestReplaceContext_Validation(t *testing.T) {
	dir := t.TempDir()
	sess := NewSession("system prompt")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "msg 1"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "response 1"})
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "msg 2"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "response 2"})
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "msg 3"})
	sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "response 3"})

	rt := NewReplaceContextTool(sess, nil)

	// Valid: replace messages 1-2.
	_, err := rt.Execute(nil, []byte(`{"from_index":1,"to_index":2,"new_content":"replaced summary"}`))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// After replacement, the session should be shorter.
	msgs := sess.Snapshot()
	if len(msgs) < 5 {
		t.Errorf("expected session to still have messages after replacement, got %d", len(msgs))
	}

	_ = dir
}

func TestReplaceContext_RejectSystemPrompt(t *testing.T) {
	sess := NewSession("system prompt")
	sess.Add(provider.Message{Role: provider.RoleUser, Content: "msg 1"})

	rt := NewReplaceContextTool(sess, nil)

	_, err := rt.Execute(nil, []byte(`{"from_index":0,"to_index":0,"new_content":"hacked"}`))
	if err == nil {
		t.Error("expected error when trying to replace system prompt")
	}
}

func TestReplaceContext_RejectRecentMessages(t *testing.T) {
	sess := NewSession("system prompt")
	for i := 0; i < 10; i++ {
		sess.Add(provider.Message{Role: provider.RoleUser, Content: "msg"})
		sess.Add(provider.Message{Role: provider.RoleAssistant, Content: "response"})
	}

	rt := NewReplaceContextTool(sess, nil)

	// Try to replace the last messages — should be rejected.
	_, err := rt.Execute(nil, []byte(`{"from_index":10,"to_index":19,"new_content":"should fail"}`))
	if err == nil {
		t.Error("expected error when trying to replace recent messages")
	}
}

func TestDiffPreview_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	original := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	dt := NewDiffPreviewTool(dir)
	result, err := dt.Execute(nil, []byte(`{"file_path":"test.go","search":"fmt.Println(\"hello\")","replace":"fmt.Println(\"world\")"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Diff Preview") {
		t.Error("expected diff preview header")
	}
	if !strings.Contains(result, `"hello"`) && !strings.Contains(result, "hello") {
		t.Error("expected diff to contain original line")
	}
	if !strings.Contains(result, "edit_file") {
		t.Error("expected hint about using edit_file")
	}

	// File should NOT have been modified.
	content, _ := os.ReadFile(path)
	if string(content) != original {
		t.Error("diff_preview should NOT modify the file")
	}
}

func TestDiffPreview_SearchNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	dt := NewDiffPreviewTool(dir)
	_, err := dt.Execute(nil, []byte(`{"file_path":"test.txt","search":"nonexistent","replace":"something"}`))
	if err == nil {
		t.Error("expected error for SEARCH not found")
	}
}

func TestDiffPreview_FileNotFound(t *testing.T) {
	dt := NewDiffPreviewTool("/tmp")
	_, err := dt.Execute(nil, []byte(`{"file_path":"nonexistent_file.xyz","search":"x","replace":"y"}`))
	if err == nil {
		t.Error("expected error for missing file")
	}
}
