// Package memory — Dream: cross-session memory consolidation. Periodically
// (default: daily, after ≥5 sessions) a forked sub-agent reviews recent
// session-memory files and the durable memory store, then consolidates:
//   - Merges related memories into more concise, higher-signal versions
//   - Removes outdated or contradicted memories
//   - Updates MEMORY.md index
//   - Produces a summary of what changed
//
// Design mirrors Claude Code's autoDream: time-gate → session-count-gate →
// lock acquisition → forked agent run. The lock file prevents concurrent dream
// runs across multiple sessions.

package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"wenhao/internal/event"
	"wenhao/internal/provider"
)

// DreamConfig controls the dream consolidation trigger.
type DreamConfig struct {
	// MinHours is the minimum hours since the last consolidation before the
	// time gate passes.
	MinHours float64
	// MinSessions is the minimum number of session memory files that must
	// exist (with mtime > lastConsolidatedAt) before the session gate passes.
	MinSessions int
	// SessionMemoryDir is the directory containing per-session memory.md files
	// (typically ~/.wenhao/sessions/).
	SessionMemoryDir string
	// StateDir is the directory where dream state (last_consolidated_at, lock)
	// is stored. Typically ~/.wenhao/.
	StateDir string
}

// DefaultDreamConfig returns the built-in dream trigger thresholds.
func DefaultDreamConfig(sessionMemoryDir, stateDir string) DreamConfig {
	return DreamConfig{
		MinHours:         24,
		MinSessions:      5,
		SessionMemoryDir: sessionMemoryDir,
		StateDir:         stateDir,
	}
}

const (
	dreamStateFile = "dream_state.json"
	dreamLockFile  = "dream.lock"
)

// dreamState is the persistent record of the last consolidation.
type dreamState struct {
	LastConsolidatedAt time.Time `json:"last_consolidated_at"`
	SessionCount       int       `json:"session_count"` // sessions reviewed in last run
}

// Dream manages cross-session consolidation. Created once at boot; the
// controller calls MaybeDream after session startup.
type Dream struct {
	cfg   DreamConfig
	store Store // durable memory store to consolidate into
	sink  event.Sink
	prov  provider.Provider // for the dream agent's summarization
	sys   string            // system prompt for the dream agent

	mu            sync.Mutex
	lastSessionScan time.Time
	lastScanCount   int
}

// NewDream creates a Dream manager. store is the durable memory store to
// consolidate into. prov and sys are used to run the dream agent. A zero
// store disables dream entirely.
func NewDream(cfg DreamConfig, store Store, prov provider.Provider, sys string, sink event.Sink) *Dream {
	if cfg.MinHours <= 0 {
		cfg.MinHours = 24
	}
	if cfg.MinSessions <= 0 {
		cfg.MinSessions = 5
	}
	return &Dream{
		cfg:   cfg,
		store: store,
		prov:  prov,
		sys:   sys,
		sink:  sink,
	}
}

// Enabled reports whether dream can run (valid store and provider).
func (d *Dream) Enabled() bool {
	return d != nil && d.store.Dir != "" && d.prov != nil
}

// MaybeDream checks the gates and, if they pass, runs consolidation in a
// background goroutine. Safe to call at session startup and periodically.
// The dream agent runs with a separate context so it isn't tied to the
// session lifecycle.
func (d *Dream) MaybeDream(ctx context.Context) {
	if !d.Enabled() {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Time gate: check hours since last consolidation.
	lastAt := d.readLastConsolidatedAt()
	hoursSince := time.Since(lastAt).Hours()
	if hoursSince < d.cfg.MinHours {
		return
	}

	// Session scan throttle: don't re-scan the filesystem on every call.
	if time.Since(d.lastSessionScan) < 10*time.Minute && d.lastScanCount < d.cfg.MinSessions {
		return
	}

	// Session gate: count session memory files touched since last consolidation.
	sessionIDs, err := d.listSessionsSince(lastAt)
	if err != nil {
		d.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
			Text: "dream: session scan failed: " + err.Error()})
		return
	}
	d.lastSessionScan = time.Now()
	d.lastScanCount = len(sessionIDs)

	if len(sessionIDs) < d.cfg.MinSessions {
		return
	}

	// Lock gate: prevent concurrent dream runs.
	if !d.tryAcquireLock() {
		return
	}

	d.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
		Text: fmt.Sprintf("dream: consolidating memories from %d recent sessions (%.1fh since last run)…",
			len(sessionIDs), hoursSince)})

	// Run dream in background goroutine.
	go d.runDream(context.WithoutCancel(ctx), sessionIDs, lastAt)
}

// runDream executes the consolidation agent and updates state on completion.
func (d *Dream) runDream(ctx context.Context, sessionIDs []string, priorMtime time.Time) {
	defer d.releaseLock()

	startTime := time.Now()
	notice, err := d.consolidate(ctx, sessionIDs)
	if err != nil {
		d.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
			Text: "dream: consolidation failed: " + err.Error()})
		// Roll back the mtime so the time gate passes again soon.
		d.writeLastConsolidatedAt(priorMtime)
		return
	}

	// Update the timestamp to now so the time gate resets.
	d.writeLastConsolidatedAt(time.Now())

	elapsed := time.Since(startTime)
	msg := fmt.Sprintf("dream: consolidation complete in %v", elapsed.Round(time.Second))
	if notice != "" {
		msg += " — " + notice
	}
	d.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Text: msg})
}

// consolidate runs the actual dream agent. It reads session memory files and
// the current durable memory store, then produces consolidation instructions.
func (d *Dream) consolidate(ctx context.Context, sessionIDs []string) (string, error) {
	// 1. Collect session memory contents.
	var sessionContexts []string
	for _, id := range sessionIDs {
		memPath := d.sessionMemoryPath(id)
		content, err := os.ReadFile(memPath)
		if err != nil {
			continue
		}
		trimmed := strings.TrimSpace(string(content))
		if trimmed == "" {
			continue
		}
		// Keep each session summary bounded.
		if len(trimmed) > 3000 {
			trimmed = trimmed[:3000] + "\n... (truncated)"
		}
		sessionContexts = append(sessionContexts,
			fmt.Sprintf("### Session %s\n\n%s", id, trimmed))
	}

	if len(sessionContexts) == 0 {
		return "", fmt.Errorf("no session memory files found")
	}

	// 2. Read current durable memories.
	manifest := FormatMemoryManifest(d.store)
	memories := d.store.List()

	// 3. Build the dream prompt.
	prompt := buildDreamPrompt(len(sessionIDs), manifest, memories, sessionContexts)

	// 4. Run the summarization.
	ch, err := d.prov.Stream(ctx, provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: dreamSystemPrompt},
			{Role: provider.RoleUser, Content: prompt},
		},
		Temperature: 0,
	})
	if err != nil {
		return "", fmt.Errorf("dream stream: %w", err)
	}

	var result strings.Builder
	for chunk := range ch {
		switch chunk.Type {
		case provider.ChunkText:
			result.WriteString(chunk.Text)
		case provider.ChunkError:
			return "", chunk.Err
		}
	}

	// 5. Parse the agent's output and apply changes to the memory store.
	output := strings.TrimSpace(result.String())
	if output == "" {
		return "", fmt.Errorf("dream agent returned empty output")
	}

	changes := d.applyDreamOutput(output)

	return changes, nil
}

// applyDreamOutput parses the dream agent's instructions and applies them to
// the memory store. The agent output format is plain text with sections:
//
//	MERGE: <name> + <name> → <new_name>
//	  title: <title>
//	  description: <desc>
//	  body: ...
//	DELETE: <name>
//	UPDATE: <name>
//	  description: <new_desc>
//	  body: ...
func (d *Dream) applyDreamOutput(output string) string {
	var merged, deleted, updated int

	// Simple line-based parser for dream output directives.
	lines := strings.Split(output, "\n")
	var currentAction string
	var currentName string
	var currentBody strings.Builder
	inBody := false

	flush := func() {
		if currentAction == "" || currentName == "" {
			currentAction = ""
			currentName = ""
			currentBody.Reset()
			inBody = false
			return
		}
		body := strings.TrimSpace(currentBody.String())
		switch currentAction {
		case "MERGE":
			// Parse "name1 + name2 → newName"
			// Extract source names before → and delete them after creating the merge.
			var sourceNames []string
			parts := strings.SplitN(currentName, "→", 2)
			newName := strings.TrimSpace(currentName)
			if len(parts) == 2 {
				newName = strings.TrimSpace(parts[1])
				// Parse source names: "name1 + name2 + name3"
				srcPart := strings.TrimSpace(parts[0])
				for _, s := range strings.Split(srcPart, "+") {
					s = slug(strings.TrimSpace(s))
					if s != "" {
						sourceNames = append(sourceNames, s)
					}
				}
			}
			if newName != "" && body != "" {
				d.store.Save(Memory{
					Name:        newName,
					Title:       newName,
					Description: firstLine(body),
					Type:        TypeProject,
					Body:        body,
				})
				merged++
				// Delete the source memories so they don't persist as duplicates.
				for _, src := range sourceNames {
					if src != newName {
						d.store.Delete(src)
					}
				}
			}
		case "DELETE":
			d.store.Delete(currentName)
			deleted++
		case "UPDATE":
			if body != "" {
				d.store.Save(Memory{
					Name:        currentName,
					Title:       currentName,
					Description: firstLine(body),
					Type:        TypeProject,
					Body:        body,
				})
				updated++
			}
		}
		currentAction = ""
		currentName = ""
		currentBody.Reset()
		inBody = false
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if inBody {
				currentBody.WriteString("\n")
			}
			continue
		}

		// Detect action directives.
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "MERGE:") || strings.HasPrefix(upper, "DELETE:") || strings.HasPrefix(upper, "UPDATE:") {
			flush()
			parts := strings.SplitN(trimmed, ":", 2)
			currentAction = strings.ToUpper(strings.TrimSpace(parts[0]))
			if len(parts) > 1 {
				currentName = strings.TrimSpace(parts[1])
			}
			inBody = true
			continue
		}

		if inBody {
			currentBody.WriteString(trimmed)
			currentBody.WriteString("\n")
		}
	}
	flush()

	parts := make([]string, 0, 3)
	if merged > 0 {
		parts = append(parts, fmt.Sprintf("%d merged", merged))
	}
	if updated > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", updated))
	}
	if deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d removed", deleted))
	}
	if len(parts) == 0 {
		return "no changes needed"
	}
	return strings.Join(parts, ", ")
}

// --- Lock management ---

func (d *Dream) lockPath() string {
	return filepath.Join(d.cfg.StateDir, dreamLockFile)
}

func (d *Dream) statePath() string {
	return filepath.Join(d.cfg.StateDir, dreamStateFile)
}

func (d *Dream) tryAcquireLock() bool {
	path := d.lockPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}

	// Try to create the lock file exclusively.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		// Lock already held — another dream is running.
		return false
	}
	// Write the current time into the lock so a stuck lock can be detected.
	fmt.Fprintf(f, "%d\n", time.Now().Unix())
	f.Close()
	return true
}

func (d *Dream) releaseLock() {
	os.Remove(d.lockPath())
}

// readLastConsolidatedAt returns the timestamp of the last consolidation, or
// the zero time if no state exists.
func (d *Dream) readLastConsolidatedAt() time.Time {
	b, err := os.ReadFile(d.statePath())
	if err != nil {
		return time.Time{}
	}
	var state dreamState
	if err := json.Unmarshal(b, &state); err != nil {
		return time.Time{}
	}
	return state.LastConsolidatedAt
}

func (d *Dream) writeLastConsolidatedAt(t time.Time) {
	dir := filepath.Dir(d.statePath())
	os.MkdirAll(dir, 0o755)

	state := dreamState{
		LastConsolidatedAt: t,
	}
	b, _ := json.Marshal(state)
	os.WriteFile(d.statePath(), b, 0o644)
}

// listSessionsSince returns session IDs (directory names) whose memory.md
// has an mtime after the given timestamp.
func (d *Dream) listSessionsSince(since time.Time) ([]string, error) {
	dir := d.cfg.SessionMemoryDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		memPath := d.sessionMemoryPath(e.Name())
		info, err := os.Stat(memPath)
		if err != nil {
			continue
		}
		if info.ModTime().After(since) {
			ids = append(ids, e.Name())
		}
	}

	sort.Strings(ids)
	return ids, nil
}

func (d *Dream) sessionMemoryPath(sessionID string) string {
	return filepath.Join(d.cfg.SessionMemoryDir, sessionID, "memory.md")
}

// --- Dream prompt ---

const dreamSystemPrompt = `You are a memory consolidation agent. Your job is to review recent coding session summaries and the existing durable memory store, then produce consolidation instructions.

Analyze the session summaries and existing memories. Look for:
1. **Merge opportunities**: two or more memories that cover the same topic — produce one concise, combined memory
2. **Outdated memories**: facts that are no longer true or have been superseded
3. **Gaps**: important project knowledge that appears in multiple sessions but isn't captured as a durable memory

Output your consolidation plan using these exact directives, one per line:

MERGE: old-name-1 + old-name-2 → new-consolidated-name
  title: Consolidated Title
  description: One-line summary
  body: |
    The full markdown body of the consolidated memory.
    Include concrete details: file paths, commands, preferences, patterns.

DELETE: outdated-memory-name

UPDATE: existing-memory-name
  description: Updated description
  body: |
    The updated markdown body.

Rules:
- Be conservative — don't delete a memory unless you're confident it's wrong or fully subsumed
- Merged memories should be MORE useful than the sum of their parts
- Preserve specific details (file paths, command names, version numbers) when merging
- If there's nothing worth consolidating, output "NO_CHANGES"
- Focus on project and feedback-type memories most; user preferences rarely need merging`

func buildDreamPrompt(sessionCount int, existingManifest string, memories []Memory, sessionContexts []string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Reviewing %d recent coding sessions.\n\n", sessionCount))

	b.WriteString("## Existing Durable Memories\n\n")
	if existingManifest != "" {
		b.WriteString(existingManifest)
	} else {
		b.WriteString("(no existing memories)\n")
	}

	// Include full body of existing memories for context.
	if len(memories) > 0 {
		b.WriteString("\n### Memory Details\n\n")
		for _, m := range memories {
			b.WriteString(fmt.Sprintf("#### %s\n%s\n\n", m.Name, m.Body))
		}
	}

	b.WriteString("## Recent Session Summaries\n\n")
	for _, sc := range sessionContexts {
		b.WriteString(sc)
		b.WriteString("\n\n")
	}

	b.WriteString("## Instructions\n\n")
	b.WriteString("Review the session summaries and existing memories. Output consolidation directives (MERGE, DELETE, UPDATE) or NO_CHANGES.\n")

	return b.String()
}

func firstLine(s string) string {
	lines := strings.SplitN(s, "\n", 2)
	return strings.TrimSpace(lines[0])
}
