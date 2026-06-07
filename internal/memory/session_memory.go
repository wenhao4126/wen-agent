// Package memory — Session Memory: incremental, per-session notes maintained
// automatically in the background. A forked sub-agent periodically updates a
// session-level memory.md file so the model retains project-specific context
// (goals, decisions, files touched, errors hit) across compaction boundaries.
//
// Design mirrors Claude Code's SessionMemory: the trigger is token-growth-based
// (initThreshold + updateThreshold) OR a natural conversation break (no tool
// calls in the last turn). Each extraction fires a sub-agent that shares the
// parent's tool registry and cache prefix, constrained to read-only tools +
// write access only to the session memory file.
//
// State is closure-scoped inside NewSessionMemory to avoid module-level globals;
// tests create a fresh instance per run.
package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"wenhao/internal/provider"
)

// SessionMemoryConfig controls the session-memory extraction trigger.
type SessionMemoryConfig struct {
	// InitThreshold is the approximate token count at which session memory
	// initialisation fires for the first time in a session.
	InitThreshold int
	// UpdateThreshold is the approximate newly-added token count since the
	// last extraction that triggers an incremental update.
	UpdateThreshold int
	// ToolCallsBetweenUpdates is the floor on tool-call count between extractions.
	// Prevents firing on every turn when the session is very active.
	ToolCallsBetweenUpdates int
}

// DefaultSessionMemoryConfig is the built-in trigger thresholds.
var DefaultSessionMemoryConfig = SessionMemoryConfig{
	InitThreshold:           6000,
	UpdateThreshold:         4000,
	ToolCallsBetweenUpdates: 3,
}

// SessionMemory manages one session's memory file. Created once per session by
// the controller; safe for concurrent use (the controller serialises turns, but
// extraction may overlap the next turn's start).
type SessionMemory struct {
	cfg    SessionMemoryConfig
	dir    string // directory containing memory.md
	path   string // full path to memory.md

	mu sync.Mutex
	// lastMessageIdx is the index (into the session message slice) of the last
	// message seen by an extraction, or -1 when none has run.
	lastMessageIdx int
	// initialized flips true after the first extraction, so the init threshold
	// is only checked once.
	initialized bool
	// toolCallsSince tracks tool-call count since the last extraction.
	toolCallsSince int
	// inProgress guards against concurrent extractions.
	inProgress bool
	// pending is set when a trigger fires during an in-progress extraction so
	// a trailing run can follow.
	pending bool
	// pendingMessages carries the message snapshot for the trailing run.
	pendingMessages []provider.Message
}

// NewSessionMemory creates a session-memory manager. dir is the session-scoped
// directory (typically <sessionDir>/<id>/).
func NewSessionMemory(dir string, cfg SessionMemoryConfig) *SessionMemory {
	if cfg.InitThreshold <= 0 {
		cfg.InitThreshold = DefaultSessionMemoryConfig.InitThreshold
	}
	if cfg.UpdateThreshold <= 0 {
		cfg.UpdateThreshold = DefaultSessionMemoryConfig.UpdateThreshold
	}
	if cfg.ToolCallsBetweenUpdates <= 0 {
		cfg.ToolCallsBetweenUpdates = DefaultSessionMemoryConfig.ToolCallsBetweenUpdates
	}
	return &SessionMemory{
		cfg:            cfg,
		dir:            dir,
		path:           filepath.Join(dir, "memory.md"),
		lastMessageIdx: -1,
	}
}

// Path returns the absolute path to the session memory file.
func (sm *SessionMemory) Path() string { return sm.path }

// EnsureFile creates the memory file with a starter template if it doesn't
// already exist. Idempotent; safe to call every turn.
func (sm *SessionMemory) EnsureFile() error {
	if sm.dir == "" {
		return nil
	}
	if err := os.MkdirAll(sm.dir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(sm.path); os.IsNotExist(err) {
		return os.WriteFile(sm.path, []byte(sessionMemoryTemplate), 0o644)
	}
	return nil
}

// Content reads the current session memory file. Returns "" when the file
// doesn't exist or is unreadable.
func (sm *SessionMemory) Content() string {
	b, err := os.ReadFile(sm.path)
	if err != nil {
		return ""
	}
	return string(b)
}

// ShouldExtract reports whether the session-memory extraction should fire given
// the current session state. msgs is the full session message slice.
func (sm *SessionMemory) ShouldExtract(msgs []provider.Message) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.shouldExtractLocked(msgs)
}

func (sm *SessionMemory) shouldExtractLocked(msgs []provider.Message) bool {
	if sm.dir == "" || len(msgs) == 0 {
		return false
	}

	// Count new messages and tool calls since the last extraction.
	newCount := 0
	newToolCalls := 0
	for i := sm.lastMessageIdx + 1; i < len(msgs); i++ {
		newCount++
		if msgs[i].Role == provider.RoleAssistant {
			newToolCalls += len(msgs[i].ToolCalls)
		}
	}

	// Estimate token growth from the new messages.
	newTokens := estimateMessagesTokens(msgs[sm.lastMessageIdx+1:])

	if !sm.initialized {
		if newTokens < sm.cfg.InitThreshold {
			return false
		}
		sm.initialized = true
	}

	// Token threshold must be met.
	if newTokens < sm.cfg.UpdateThreshold {
		return false
	}

	// Tool-call threshold OR natural break (last assistant turn has no tool calls).
	toolCallsMet := newToolCalls >= sm.cfg.ToolCallsBetweenUpdates
	lastTurnNoTools := false
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleAssistant {
			lastTurnNoTools = len(msgs[i].ToolCalls) == 0
			break
		}
	}

	if !toolCallsMet && !lastTurnNoTools {
		return false
	}

	return true
}

// MarkExtractionStarting records that an extraction is beginning for this
// message snapshot. Returns false if one is already in progress (caller should
// stash for trailing run). The caller MUST call MarkExtractionDone when
// finished.
func (sm *SessionMemory) MarkExtractionStarting(msgs []provider.Message) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.inProgress {
		sm.pending = true
		sm.pendingMessages = msgs
		return false
	}
	sm.inProgress = true
	sm.lastMessageIdx = len(msgs) - 1
	return true
}

// MarkExtractionDone signals that the extraction finished. It returns a
// trailing message snapshot if one was stashed during the run.
func (sm *SessionMemory) MarkExtractionDone() []provider.Message {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.inProgress = false
	sm.toolCallsSince = 0
	if sm.pending {
		sm.pending = false
		msgs := sm.pendingMessages
		sm.pendingMessages = nil
		return msgs
	}
	return nil
}

// RecordToolCalls tells the manager how many tool calls happened in the last
// turn so the trigger can count them. Called by the controller after each turn.
func (sm *SessionMemory) RecordToolCalls(n int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.toolCallsSince += n
}

// SessionMemorySink is a per-session handle that knows how to fork a sub-agent
// to update the session memory file. The controller wires it into the boot
// assembly.
type SessionMemorySink interface {
	// UpdateSessionMemory runs one extraction pass using the given context
	// (for cancellation) and the full session message slice. It is fire-and-
	// forget from the controller's perspective; errors are best-effort logged
	// to the controller's sink as Notices.
	UpdateSessionMemory(ctx context.Context, msgs []provider.Message) error
}

// sessionMemoryTemplate is the starter content for a new memory.md.
const sessionMemoryTemplate = `# Session Notes

*Auto-maintained by Wenhao — updated as the conversation progresses.*

## Goal
<!-- The user's request and intent -->

## Decisions & Rationale
<!-- Key choices made and why -->

## Files & Code
<!-- Files read or modified, with relevant details -->

## Commands & Outcomes
<!-- Commands run and their results -->

## Errors & Fixes
<!-- Problems hit and resolutions -->

## Pending
<!-- Still in progress or unstarted -->
`

// BuildSessionMemoryPrompt constructs the extraction prompt for the forked
// sub-agent, given the current memory content and a transcript of recent
// messages.
func BuildSessionMemoryPrompt(currentMemory string, recentTranscript string) string {
	var b strings.Builder
	b.WriteString("You are updating the session notes for a coding agent's ongoing conversation.\n\n")
	b.WriteString("Below is the CURRENT session memory file. Your job is to update it by incorporating new information from the RECENT TRANSCRIPT that follows.\n\n")
	b.WriteString("## Current Session Memory\n\n")
	if currentMemory != "" {
		b.WriteString(currentMemory)
	} else {
		b.WriteString("(empty — this is the first extraction)\n")
	}
	b.WriteString("\n## Recent Transcript\n\n")
	b.WriteString(recentTranscript)
	b.WriteString("\n## Instructions\n\n")
	b.WriteString("Read the transcript above. Update the session memory file (`")
	b.WriteString("memory.md")
	b.WriteString("`) by:\n")
	b.WriteString("1. Filling in or updating each section with the new information from the transcript\n")
	b.WriteString("2. Keeping existing information that is still relevant — do NOT remove accurate context\n")
	b.WriteString("3. Being concrete: include file paths, line numbers, function signatures, error text\n")
	b.WriteString("4. Keeping each section terse — bullet points and fragments, not prose\n")
	b.WriteString("5. Preserving the ## heading structure exactly as it exists\n\n")
	b.WriteString("Use the write_file tool to save the updated file. Only write to `memory.md`.\n")
	return b.String()
}

// RecentTranscript renders the most recent N messages from the session into a
// readable transcript for the extraction sub-agent. If count is 0 or negative,
// it renders all messages after the system prompt.
func RecentTranscript(msgs []provider.Message, count int) string {
	if count <= 0 || count > len(msgs) {
		count = len(msgs)
	}
	// Skip the system prompt for the transcript.
	start := 0
	if len(msgs) > 0 && msgs[0].Role == provider.RoleSystem {
		start = 1
	}
	if count > len(msgs)-start {
		count = len(msgs) - start
	}
	recent := msgs[len(msgs)-count:]
	return renderTranscript(recent)
}

// renderTranscript flattens a slice of messages into a readable text block.
// Mirrors the format used in compact.go for consistency.
func renderTranscript(msgs []provider.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case provider.RoleUser:
			fmt.Fprintf(&b, "[user]\n%s\n\n", m.Content)
		case provider.RoleAssistant:
			if m.Content != "" {
				fmt.Fprintf(&b, "[assistant]\n%s\n", m.Content)
			}
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&b, "[assistant calls %s] %s\n", tc.Name, tc.Arguments)
			}
			b.WriteString("\n")
		case provider.RoleTool:
			fmt.Fprintf(&b, "[tool %s result]\n%s\n\n", m.Name, truncateForTranscript(m.Content))
		case provider.RoleSystem:
			// Skip system messages in transcript (the sub-agent doesn't need them).
		}
	}
	return b.String()
}

// truncateForTranscript caps a tool result at a reasonable size so the
// extraction agent's prompt stays bounded.
func truncateForTranscript(content string) string {
	const maxLen = 2048
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "\n... (truncated)"
}

// SessionMemoryInjection returns the session memory content as a context block
// suitable for injecting into the compaction summary prompt or the system
// prompt. Returns "" when the file is empty or unreadable.
func (sm *SessionMemory) Injection() string {
	content := strings.TrimSpace(sm.Content())
	if content == "" {
		return ""
	}
	return "\n## Session Memory\n\nThe following is auto-maintained session context — keep it in mind when resuming work after a compaction:\n\n" + content
}

// estimateMessagesTokens returns a rough token count for a slice of messages.
// Mirrors agent.estimateMessagesTokens for use in the memory package.
func estimateMessagesTokens(msgs []provider.Message) int {
	total := 0
	for _, m := range msgs {
		total += 4 // chat-message framing overhead
		total += estimateTextTokens(m.Content)
		total += estimateTextTokens(m.ReasoningContent)
		total += estimateTextTokens(m.Name)
		total += estimateTextTokens(m.ToolCallID)
		for _, tc := range m.ToolCalls {
			total += 8
			total += estimateTextTokens(tc.ID)
			total += estimateTextTokens(tc.Name)
			total += estimateTextTokens(tc.Arguments)
		}
	}
	return total
}

func estimateTextTokens(s string) int {
	if s == "" {
		return 0
	}
	bytes := len(s)
	runes := utf8.RuneCountInString(s)
	byBytes := (bytes + 3) / 4
	if runes > byBytes {
		return runes
	}
	return byBytes
}

// TimestampedPath returns the memory path with the last-modified time appended
// for display purposes.
func (sm *SessionMemory) TimestampedPath() string {
	info, err := os.Stat(sm.path)
	if err != nil {
		return sm.path
	}
	return fmt.Sprintf("%s (updated %s)", sm.path, info.ModTime().Format(time.RFC3339))
}
