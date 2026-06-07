package memory

import (
	"os"
	"path/filepath"
	"testing"

	"wenhao/internal/provider"
)

func TestSessionMemory_EnsureFile(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir, DefaultSessionMemoryConfig)

	if err := sm.EnsureFile(); err != nil {
		t.Fatalf("EnsureFile failed: %v", err)
	}

	// Verify the file exists and has content.
	content := sm.Content()
	if content == "" {
		t.Fatal("expected non-empty content after EnsureFile")
	}

	// Second call should be idempotent.
	if err := sm.EnsureFile(); err != nil {
		t.Fatalf("second EnsureFile failed: %v", err)
	}
}

func TestSessionMemory_Content_Empty(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir, DefaultSessionMemoryConfig)

	if content := sm.Content(); content != "" {
		t.Fatalf("expected empty content for non-existent file, got %q", content)
	}
}

func TestSessionMemory_Path(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir, DefaultSessionMemoryConfig)

	if sm.Path() != filepath.Join(dir, "memory.md") {
		t.Fatalf("expected path %s, got %s", filepath.Join(dir, "memory.md"), sm.Path())
	}
}

func TestSessionMemory_ShouldExtract_InitThreshold(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir, SessionMemoryConfig{
		InitThreshold:           100,
		UpdateThreshold:         100,
		ToolCallsBetweenUpdates: 1,
	})

	// Empty messages: should not extract.
	if sm.ShouldExtract(nil) {
		t.Fatal("expected false for nil messages")
	}

	// Not enough tokens: should not extract.
	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "system prompt"},
		{Role: provider.RoleUser, Content: "hello"},
		{Role: provider.RoleAssistant, Content: "hi"},
	}
	if sm.ShouldExtract(msgs) {
		t.Fatal("expected false for small message set")
	}

	// Build up enough tokens to pass the init threshold.
	var manyMsgs []provider.Message
	manyMsgs = append(manyMsgs, provider.Message{Role: provider.RoleSystem, Content: "system prompt"})
	for i := 0; i < 50; i++ {
		manyMsgs = append(manyMsgs, provider.Message{Role: provider.RoleUser, Content: "this is a message with enough tokens to push past the init threshold for testing purposes"})
		manyMsgs = append(manyMsgs, provider.Message{Role: provider.RoleAssistant, Content: "response with some content that adds up over multiple turns of conversation"})
	}

	// Last assistant turn has no tool calls (natural break), so should extract.
	if !sm.ShouldExtract(manyMsgs) {
		t.Fatal("expected true after init threshold met with natural break")
	}
}

func TestSessionMemory_ShouldExtract_UpdateThreshold(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir, SessionMemoryConfig{
		InitThreshold:           100,
		UpdateThreshold:         200,
		ToolCallsBetweenUpdates: 2,
	})

	// First extraction initializes.
	var msgs []provider.Message
	msgs = append(msgs, provider.Message{Role: provider.RoleSystem, Content: "system"})
	for i := 0; i < 20; i++ {
		msgs = append(msgs, provider.Message{Role: provider.RoleUser, Content: "message content that adds up to meaningful token count"})
		msgs = append(msgs, provider.Message{Role: provider.RoleAssistant, Content: "response content", ToolCalls: []provider.ToolCall{{ID: "1", Name: "read_file", Arguments: "{}"}}})
	}
	if !sm.ShouldExtract(msgs) {
		t.Fatal("expected true for init threshold")
	}
	sm.MarkExtractionStarting(msgs)
	sm.MarkExtractionDone()

	// Add small amount: should NOT trigger update (below update threshold).
	msgs = append(msgs, provider.Message{Role: provider.RoleUser, Content: "short"})
	msgs = append(msgs, provider.Message{Role: provider.RoleAssistant, Content: "ok"})
	if sm.ShouldExtract(msgs) {
		t.Fatal("expected false: not enough new tokens for update threshold")
	}
}

func TestSessionMemory_MarkExtraction_Concurrency(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir, SessionMemoryConfig{
		InitThreshold:   10,
		UpdateThreshold: 10,
	})

	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "system"},
		{Role: provider.RoleUser, Content: "a long enough message to pass the minimum token threshold for triggering session memory extraction"},
		{Role: provider.RoleAssistant, Content: "response"},
	}

	// First extraction starts.
	if !sm.MarkExtractionStarting(msgs) {
		t.Fatal("expected true for first extraction start")
	}

	// Second extraction while one is in progress should stash.
	if sm.MarkExtractionStarting(msgs) {
		t.Fatal("expected false: extraction already in progress")
	}

	// Done: should return trailing snapshot.
	trailing := sm.MarkExtractionDone()
	if trailing == nil {
		t.Fatal("expected trailing snapshot after stashed extraction")
	}
}

func TestSessionMemory_BuildPrompt(t *testing.T) {
	current := "## Goal\nImplement login"

	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "please add a login page"},
		{Role: provider.RoleAssistant, Content: "I'll add auth/login.tsx with the login form."},
		{Role: provider.RoleTool, Name: "write_file", Content: "wrote file successfully"},
	}

	transcript := renderTranscript(msgs)
	prompt := BuildSessionMemoryPrompt(current, transcript)

	if len(prompt) == 0 {
		t.Fatal("expected non-empty prompt")
	}

	// Should contain the current memory.
	if !contains(prompt, "Implement login") {
		t.Error("prompt should contain current memory content")
	}
	// Should contain the transcript.
	if !contains(prompt, "please add a login page") {
		t.Error("prompt should contain transcript content")
	}
	// Should contain instructions.
	if !contains(prompt, "memory.md") {
		t.Error("prompt should mention memory.md")
	}
}

func TestSessionMemory_RecentTranscript(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "system prompt"},
		{Role: provider.RoleUser, Content: "first message"},
		{Role: provider.RoleAssistant, Content: "first response"},
		{Role: provider.RoleUser, Content: "second message"},
		{Role: provider.RoleAssistant, Content: "second response"},
	}

	// Get last 2 messages (excluding system).
	transcript := RecentTranscript(msgs, 2)
	if transcript == "" {
		t.Fatal("expected non-empty transcript")
	}
	// Should contain second message, not first.
	if !contains(transcript, "second message") {
		t.Error("transcript should contain second message")
	}
	if contains(transcript, "first message") {
		t.Error("transcript should NOT contain first message when count=2")
	}
	if contains(transcript, "system prompt") {
		t.Error("transcript should NOT contain system prompt")
	}
}

func TestSessionMemory_Injection(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir, DefaultSessionMemoryConfig)
	sm.EnsureFile()

	// Fresh template should produce a non-empty injection.
	inj := sm.Injection()
	if inj == "" {
		t.Fatal("expected non-empty injection from template")
	}
	if !contains(inj, "Session Memory") {
		t.Error("injection should contain section header")
	}
}

func TestSessionMemory_TimestampedPath(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir, DefaultSessionMemoryConfig)
	sm.EnsureFile()

	tsPath := sm.TimestampedPath()
	if !contains(tsPath, "memory.md") {
		t.Errorf("expected path to contain memory.md, got %s", tsPath)
	}
	if !contains(tsPath, "updated") {
		t.Errorf("expected path to contain 'updated', got %s", tsPath)
	}
}

func TestExtractMemories_Disabled(t *testing.T) {
	// Zero store = disabled.
	em := NewExtractMemories(Store{}, DefaultExtractMemoriesConfig)
	if em.Enabled() {
		t.Fatal("expected disabled with zero store")
	}
	if em.ShouldExtract(nil) {
		t.Fatal("expected false for disabled extractor")
	}
}

func TestExtractMemories_ShouldExtract(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	store := Store{Dir: memDir}

	em := NewExtractMemories(store, ExtractMemoriesConfig{
		MinMessagesBetweenExtractions: 3,
		MaxTurns:                      5,
	})

	if !em.Enabled() {
		t.Fatal("expected enabled with valid store")
	}

	// Not enough messages.
	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "system"},
		{Role: provider.RoleUser, Content: "hello"},
	}
	if em.ShouldExtract(msgs) {
		t.Fatal("expected false: not enough visible messages")
	}

	// Enough visible messages.
	msgs = append(msgs,
		provider.Message{Role: provider.RoleAssistant, Content: "hi"},
		provider.Message{Role: provider.RoleUser, Content: "how are you"},
		provider.Message{Role: provider.RoleAssistant, Content: "good"},
	)
	if !em.ShouldExtract(msgs) {
		t.Fatal("expected true: enough visible messages")
	}
}

func TestExtractMemories_IsAutoMemPath(t *testing.T) {
	em := NewExtractMemories(Store{Dir: "/home/user/.wenhao/projects/test/memory"}, DefaultExtractMemoriesConfig)

	if !em.IsAutoMemPath("/home/user/.wenhao/projects/test/memory/auth-pattern.md") {
		t.Fatal("expected true for path inside memory dir")
	}
	if em.IsAutoMemPath("/home/user/.wenhao/projects/test/memory/../escape.md") {
		t.Fatal("expected false for .. escape")
	}
	if em.IsAutoMemPath("/etc/passwd") {
		t.Fatal("expected false for unrelated path")
	}
	if em.IsAutoMemPath("/home/user/.wenhao/projects/test/outside.md") {
		t.Fatal("expected false for sibling dir")
	}
}

func TestExtractMemories_MarkExtraction(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	store := Store{Dir: memDir}
	em := NewExtractMemories(store, DefaultExtractMemoriesConfig)

	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "hello"},
		{Role: provider.RoleAssistant, Content: "hi"},
	}

	// Start extraction.
	if !em.MarkExtractionStarting(msgs) {
		t.Fatal("expected true for first extraction")
	}

	// Second start while in progress.
	if em.MarkExtractionStarting(msgs) {
		t.Fatal("expected false: already in progress")
	}

	// Done with nil (trailing run shouldn't advance cursor).
	trailing := em.MarkExtractionDone(nil)
	if trailing == nil {
		t.Fatal("expected trailing snapshot")
	}
}

func TestFormatMemoryManifest(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	os.MkdirAll(memDir, 0o755)
	store := Store{Dir: memDir}

	// Save a memory.
	_, err := store.Save(Memory{
		Name:        "test-fact",
		Title:       "Test Fact",
		Description: "A test memory for manifest testing",
		Type:        TypeProject,
		Body:        "This is a test fact.",
	})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	manifest := FormatMemoryManifest(store)
	if manifest == "" {
		t.Fatal("expected non-empty manifest")
	}
	if !contains(manifest, "Test Fact") {
		t.Error("manifest should contain memory title")
	}
	if !contains(manifest, "test-fact") {
		t.Error("manifest should contain memory name")
	}
}

func TestExtractMemories_BuildPrompt(t *testing.T) {
	manifest := "- [Test Fact](test-fact.md) — A test memory"
	prompt := BuildExtractMemoriesPrompt(10, manifest)

	if len(prompt) == 0 {
		t.Fatal("expected non-empty prompt")
	}
	if !contains(prompt, "10 messages") {
		t.Error("prompt should mention message count")
	}
	if !contains(prompt, "Test Fact") {
		t.Error("prompt should contain existing memories")
	}
}

func TestMemory_RecursiveIntegration(t *testing.T) {
	dir := t.TempDir()

	// Set up session memory.
	sm := NewSessionMemory(dir, SessionMemoryConfig{
		InitThreshold:   50,
		UpdateThreshold: 50,
	})
	if err := sm.EnsureFile(); err != nil {
		t.Fatalf("EnsureFile: %v", err)
	}

	// Build messages that cross the init threshold.
	var msgs []provider.Message
	msgs = append(msgs, provider.Message{Role: provider.RoleSystem, Content: "system prompt"})
	for i := 0; i < 10; i++ {
		msgs = append(msgs,
			provider.Message{Role: provider.RoleUser, Content: "user message with enough text to accumulate tokens for threshold testing"},
			provider.Message{Role: provider.RoleAssistant, Content: "assistant response with sufficient text content for extraction testing"},
		)
	}

	// Should trigger extraction.
	if !sm.ShouldExtract(msgs) {
		t.Fatal("expected ShouldExtract=true after crossing init threshold")
	}

	// Mark extraction.
	if !sm.MarkExtractionStarting(msgs) {
		t.Fatal("expected extraction to start")
	}

	// Write some fake updated content (simulating what the extraction agent would do).
	updated := "## Goal\nImplement user authentication\n\n## Decisions\n- Use JWT tokens\n\n## Files\n- src/auth/login.ts created"
	if err := os.WriteFile(sm.Path(), []byte(updated), 0o644); err != nil {
		t.Fatalf("write updated memory: %v", err)
	}

	sm.MarkExtractionDone()

	// Verify content was updated.
	content := sm.Content()
	if !contains(content, "user authentication") {
		t.Errorf("expected updated content, got: %s", content)
	}

	// Verify injection works.
	inj := sm.Injection()
	if !contains(inj, "Session Memory") {
		t.Error("injection should contain header")
	}
	if !contains(inj, "user authentication") {
		t.Error("injection should contain memory content")
	}
}

func contains(s, sub string) bool {
	return len(s) > 0 && len(sub) > 0 && len(s) >= len(sub) && searchSubstring(s, sub)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
