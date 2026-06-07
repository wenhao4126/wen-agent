// Package memory — Durable Memory extraction: automatic, background extraction of
// reusable knowledge from conversations into the project's auto-memory store.
//
// After each turn, when enough new content has accumulated, the controller may
// fire a forked sub-agent that:
//   1. Reads the current memory index (MEMORY.md) plus existing memory files.
//   2. Scans the recent conversation for reusable facts, patterns, and lessons.
//   3. Writes new or updated memory files to ~/.wenhao/projects/<slug>/memory/.
//   4. Updates MEMORY.md with the new entries.
//
// The sub-agent is constrained: read-only access everywhere, write access only
// within the memory directory. It shares the parent's cache prefix for efficiency.
//
// Design mirrors Claude Code's extractMemories: closure-scoped state, coalesced
// firing, trailing extraction for stashed contexts.

package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"wenhao/internal/event"
	"wenhao/internal/provider"
)

// ExtractMemoriesConfig controls the durable-memory extraction trigger.
type ExtractMemoriesConfig struct {
	// MinMessagesBetweenExtractions is the floor on visible messages between
	// extraction runs. Prevents firing on every turn.
	MinMessagesBetweenExtractions int
	// MaxTurns is the cap on API round-trips the sub-agent may use. Guards
	// against verification rabbit-holes.
	MaxTurns int
}

// DefaultExtractMemoriesConfig is the built-in trigger threshold.
var DefaultExtractMemoriesConfig = ExtractMemoriesConfig{
	MinMessagesBetweenExtractions: 6,
	MaxTurns:                      5,
}

// ExtractMemories manages durable-memory extraction for one session. Created
// once by the controller; closure-scoped state avoids module-level globals.
type ExtractMemories struct {
	cfg    ExtractMemoriesConfig
	store  Store
	dir    string // resolved memory directory

	mu sync.Mutex
	// lastMessageIdx is the index (into the session message slice) of the last
	// message processed by an extraction.
	lastMessageIdx int
	// inProgress guards against concurrent extractions.
	inProgress bool
	// pending carries stashed context for a trailing run.
	pending        bool
	pendingContext context.Context
	pendingMessages []provider.Message
}

// NewExtractMemories creates a durable-memory extraction manager. store is the
// project's auto-memory Store; a zero Store (no user config dir) disables
// extraction entirely.
func NewExtractMemories(store Store, cfg ExtractMemoriesConfig) *ExtractMemories {
	if cfg.MinMessagesBetweenExtractions <= 0 {
		cfg.MinMessagesBetweenExtractions = DefaultExtractMemoriesConfig.MinMessagesBetweenExtractions
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = DefaultExtractMemoriesConfig.MaxTurns
	}
	return &ExtractMemories{
		cfg:            cfg,
		store:          store,
		dir:            store.Dir,
		lastMessageIdx: -1,
	}
}

// Enabled reports whether durable-memory extraction can run (a valid store
// directory exists).
func (em *ExtractMemories) Enabled() bool {
	return em != nil && em.dir != ""
}

// Dir returns the memory store directory.
func (em *ExtractMemories) Dir() string { return em.dir }

// ShouldExtract reports whether extraction should fire given the current
// session state. msgs is the full session message slice.
func (em *ExtractMemories) ShouldExtract(msgs []provider.Message) bool {
	if !em.Enabled() {
		return false
	}
	em.mu.Lock()
	defer em.mu.Unlock()

	if em.inProgress {
		return false
	}

	// Count visible (non-system) messages since the last extraction.
	visibleSince := 0
	for i := em.lastMessageIdx + 1; i < len(msgs); i++ {
		if msgs[i].Role != provider.RoleSystem {
			visibleSince++
		}
	}
	return visibleSince >= em.cfg.MinMessagesBetweenExtractions
}

// MarkExtractionStarting records that an extraction is beginning for this
// message snapshot. Returns false when one is already in progress.
func (em *ExtractMemories) MarkExtractionStarting(msgs []provider.Message) bool {
	em.mu.Lock()
	defer em.mu.Unlock()
	if em.inProgress {
		em.pending = true
		em.pendingMessages = msgs
		return false
	}
	em.inProgress = true
	return true
}

// MarkExtractionDone signals that the extraction finished and advances the
// cursor given the current message snapshot. It returns a trailing message
// snapshot if one was stashed. msgs may be nil when the caller doesn't have
// a snapshot (trailing run).
func (em *ExtractMemories) MarkExtractionDone(msgs []provider.Message) []provider.Message {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.inProgress = false
	if msgs != nil {
		em.lastMessageIdx = len(msgs) - 1
	}
	if em.pending {
		em.pending = false
		trailing := em.pendingMessages
		em.pendingMessages = nil
		return trailing
	}
	return nil
}

// BuildExtractMemoriesPrompt constructs the extraction prompt for the forked
// sub-agent. existingManifest is the output of formatMemoryManifest.
func BuildExtractMemoriesPrompt(newMessageCount int, existingManifest string) string {
	var b strings.Builder
	b.WriteString("You are extracting durable, reusable knowledge from a coding agent's conversation.\n\n")
	b.WriteString("## Existing Memories\n\n")
	if existingManifest != "" {
		b.WriteString(existingManifest)
	} else {
		b.WriteString("(no existing memories yet)\n")
	}
	b.WriteString("\n## Task\n\n")
	b.WriteString("Analyze the conversation above. Identify facts, patterns, decisions, or lessons that are:\n")
	b.WriteString("- **Reusable** across sessions (not just relevant to this one conversation)\n")
	b.WriteString("- **Not already captured** in the existing memories above\n")
	b.WriteString("- **Not derivable** from reading the codebase alone\n\n")
	b.WriteString("For each extractable fact, create or update a memory file under the memory directory.\n")
	b.WriteString("Each file should have frontmatter (name, description, type) and a Markdown body.\n")
	b.WriteString("Then update MEMORY.md with one line per memory file.\n\n")
	b.WriteString("**Memory types:**\n")
	b.WriteString("- `user`: who the user is — role, preferences, expertise\n")
	b.WriteString("- `feedback`: guidance on how to work — with why and how-to-apply\n")
	b.WriteString("- `project`: ongoing work, goals, constraints not in the code\n")
	b.WriteString("- `reference`: pointers to external resources\n\n")
	b.WriteString(fmt.Sprintf("Review the last %d messages. Only save genuinely durable knowledge — when in doubt, skip it.\n", newMessageCount))
	b.WriteString("Use read_file to check existing memories before updating them. Use write_file only within the memory directory.\n")
	return b.String()
}

// FormatMemoryManifest returns a human-readable listing of the current memory
// files (name + description) suitable for the extraction prompt.
func FormatMemoryManifest(store Store) string {
	memories := store.List()
	if len(memories) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Current memories:\n")
	for _, m := range memories {
		fmt.Fprintf(&b, "- [%s](%s.md) — %s\n", m.Title, m.Name, m.Description)
	}
	return b.String()
}

// IsAutoMemPath reports whether the given absolute path falls inside the memory
// directory. Used by the permission layer to constrain the extraction sub-agent.
func (em *ExtractMemories) IsAutoMemPath(absPath string) bool {
	if em.dir == "" {
		return false
	}
	rel, err := filepath.Rel(em.dir, absPath)
	if err != nil {
		return false
	}
	// Must be a descendant of the memory dir (not ".." or an absolute escape).
	return !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

// ExtractMemoriesSink is a per-session handle that knows how to fork a
// sub-agent to extract durable memories. The controller wires it in.
type ExtractMemoriesSink interface {
	// ExtractMemories runs one extraction pass. ctx is for cancellation; msgs
	// is the full session message slice. It writes new memories to the store
	// and returns a notice string (or "" when nothing was saved).
	ExtractMemories(ctx context.Context, msgs []provider.Message) (notice string, err error)
}

// memoryExtractionNotice builds the user-visible notice for saved memories.
func memoryExtractionNotice(filesWritten []string) string {
	if len(filesWritten) == 0 {
		return ""
	}
	names := make([]string, len(filesWritten))
	for i, p := range filesWritten {
		names[i] = filepath.Base(p)
	}
	return fmt.Sprintf("Saved %d memories: %s", len(names), strings.Join(names, ", "))
}

// ensureMemoryDir creates the memory directory if needed.
func ensureMemoryDir(dir string) error {
	if dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

// memoryDirModTime returns the most recent modification time of any file in the
// memory directory, or the zero time.
func memoryDirModTime(dir string) time.Time {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return time.Time{}
	}
	var latest time.Time
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest
}

// PostTurnMemoryEvent carries the result of a post-turn memory extraction to
// the controller's event stream.
type PostTurnMemoryEvent struct {
	Kind            string // "session_memory" | "extract_memories"
	FilesWritten    []string
	MemoriesSaved   int
	Notice          string
	Err             error
}

// ToEvent converts a PostTurnMemoryEvent to an event.Event for the sink.
func (e PostTurnMemoryEvent) ToEvent() event.Event {
	if e.Err != nil {
		return event.Event{
			Kind:  event.Notice,
			Level: event.LevelWarn,
			Text:  fmt.Sprintf("%s: %v", e.Kind, e.Err),
		}
	}
	if e.Notice != "" {
		return event.Event{
			Kind:  event.Notice,
			Level: event.LevelInfo,
			Text:  e.Notice,
		}
	}
	return event.Event{}
}
