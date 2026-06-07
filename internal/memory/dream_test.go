package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDream_Disabled(t *testing.T) {
	d := NewDream(DreamConfig{}, Store{}, nil, "", nil)
	if d.Enabled() {
		t.Fatal("expected disabled with zero store")
	}

	// MaybeDream should not panic on disabled dream.
	d.MaybeDream(nil)
}

func TestDream_Lock(t *testing.T) {
	dir := t.TempDir()
	d := &Dream{
		cfg: DreamConfig{StateDir: dir},
	}

	// First lock should succeed.
	if !d.tryAcquireLock() {
		t.Fatal("expected lock to succeed")
	}

	// Second lock should fail (already held).
	if d.tryAcquireLock() {
		t.Fatal("expected second lock to fail")
	}

	// Release and re-acquire.
	d.releaseLock()
	if !d.tryAcquireLock() {
		t.Fatal("expected lock after release")
	}
	d.releaseLock()
}

func TestDream_StateReadWrite(t *testing.T) {
	dir := t.TempDir()
	d := &Dream{
		cfg: DreamConfig{StateDir: dir},
	}

	// Zero time when no state.
	if !d.readLastConsolidatedAt().IsZero() {
		t.Error("expected zero time when no state file")
	}

	// Write and read back.
	now := time.Now().Truncate(time.Second)
	d.writeLastConsolidatedAt(now)
	got := d.readLastConsolidatedAt()
	if !got.Equal(now) {
		t.Errorf("expected %v, got %v", now, got)
	}
}

func TestDream_ListSessionsSince(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions")

	// Create some session memory files with different mtimes.
	oldSession := filepath.Join(sessionsDir, "old-session")
	newSession := filepath.Join(sessionsDir, "new-session")
	os.MkdirAll(oldSession, 0o755)
	os.MkdirAll(newSession, 0o755)

	// Write memory files.
	os.WriteFile(filepath.Join(oldSession, "memory.md"), []byte("old memory"), 0o644)
	os.WriteFile(filepath.Join(newSession, "memory.md"), []byte("new memory"), 0o644)

	// Set mtimes: old is 48h ago, new is 1h ago.
	oldTime := time.Now().Add(-48 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)
	os.Chtimes(filepath.Join(oldSession, "memory.md"), oldTime, oldTime)
	os.Chtimes(filepath.Join(newSession, "memory.md"), newTime, newTime)

	d := &Dream{
		cfg: DreamConfig{SessionMemoryDir: sessionsDir},
	}

	// Since 6h ago should only include the new session.
	since := time.Now().Add(-6 * time.Hour)
	ids, err := d.listSessionsSince(since)
	if err != nil {
		t.Fatalf("listSessionsSince: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("expected 1 session since 6h ago, got %d: %v", len(ids), ids)
	}
	if len(ids) > 0 && ids[0] != "new-session" {
		t.Errorf("expected new-session, got %s", ids[0])
	}

	// Since 72h ago should include both.
	since = time.Now().Add(-72 * time.Hour)
	ids, err = d.listSessionsSince(since)
	if err != nil {
		t.Fatalf("listSessionsSince: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 sessions since 72h ago, got %d", len(ids))
	}
}

func TestDream_ApplyOutput_Merge(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	os.MkdirAll(memDir, 0o755)
	store := Store{Dir: memDir}

	// Pre-populate two memories.
	store.Save(Memory{Name: "auth-pattern", Description: "Auth pattern", Type: TypeProject, Body: "Use JWT for auth."})
	store.Save(Memory{Name: "db-pattern", Description: "DB pattern", Type: TypeProject, Body: "Use pgx for postgres."})

	d := &Dream{store: store}

	output := `MERGE: auth-pattern + db-pattern → backend-stack
  title: Backend Stack
  description: Consolidated backend technology choices
  body: |
    Use JWT for authentication and pgx for PostgreSQL access.
    All new services should follow this stack.`

	changes := d.applyDreamOutput(output)
	if !strings.Contains(changes, "merged") {
		t.Errorf("expected merge in changes, got %q", changes)
	}

	// Verify the merged memory exists.
	memories := store.List()
	found := false
	for _, m := range memories {
		if m.Name == "backend-stack" {
			found = true
			if !strings.Contains(m.Body, "JWT") {
				t.Error("merged memory should contain JWT")
			}
			if !strings.Contains(m.Body, "pgx") {
				t.Error("merged memory should contain pgx")
			}
		}
	}
	if !found {
		t.Error("expected backend-stack memory after merge")
	}
}

func TestDream_ApplyOutput_Delete(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	os.MkdirAll(memDir, 0o755)
	store := Store{Dir: memDir}

	store.Save(Memory{Name: "outdated-fact", Description: "Outdated", Type: TypeProject, Body: "Use Python 2.7."})

	d := &Dream{store: store}

	output := "DELETE: outdated-fact\n"
	changes := d.applyDreamOutput(output)
	if !strings.Contains(changes, "removed") {
		t.Errorf("expected removed in changes, got %q", changes)
	}

	memories := store.List()
	for _, m := range memories {
		if m.Name == "outdated-fact" {
			t.Error("outdated-fact should have been deleted")
		}
	}
}

func TestDream_ApplyOutput_Update(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	os.MkdirAll(memDir, 0o755)
	store := Store{Dir: memDir}

	store.Save(Memory{Name: "api-version", Description: "API version", Type: TypeProject, Body: "Using v1 API."})

	d := &Dream{store: store}

	output := `UPDATE: api-version
  description: Updated API version info
  body: |
    Migrated to v2 API. All new endpoints use v2.`

	changes := d.applyDreamOutput(output)
	if !strings.Contains(changes, "updated") {
		t.Errorf("expected updated in changes, got %q", changes)
	}

	memories := store.List()
	for _, m := range memories {
		if m.Name == "api-version" {
			if !strings.Contains(m.Body, "v2") {
				t.Error("updated memory should contain v2")
			}
		}
	}
}

func TestDream_ApplyOutput_NoChanges(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	os.MkdirAll(memDir, 0o755)
	store := Store{Dir: memDir}

	d := &Dream{store: store}
	changes := d.applyDreamOutput("")
	if changes != "no changes needed" {
		t.Errorf("expected 'no changes needed', got %q", changes)
	}
}

func TestBuildDreamPrompt(t *testing.T) {
	memories := []Memory{
		{Name: "test-1", Description: "Test memory 1", Body: "Content 1"},
	}
	sessionContexts := []string{"### Session abc\n\nGoal: build login page"}

	prompt := buildDreamPrompt(1, "- [test-1](test-1.md)", memories, sessionContexts)
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !strings.Contains(prompt, "test-1") {
		t.Error("prompt should contain existing memory")
	}
	if !strings.Contains(prompt, "build login page") {
		t.Error("prompt should contain session context")
	}
	if !strings.Contains(prompt, "MERGE") {
		t.Error("prompt should instruct about MERGE directives")
	}
}

func TestDream_MinHours_Default(t *testing.T) {
	cfg := DefaultDreamConfig("/tmp/sessions", "/tmp/state")
	if cfg.MinHours != 24 {
		t.Errorf("expected default MinHours=24, got %f", cfg.MinHours)
	}
	if cfg.MinSessions != 5 {
		t.Errorf("expected default MinSessions=5, got %d", cfg.MinSessions)
	}
}
