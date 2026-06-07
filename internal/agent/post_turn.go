// Package agent — PostTurnHook: fires after every agent turn to drive background
// memory extraction (session memory + durable memory). The hook is invoked by the
// controller after the runner's Run returns and before plan-approval gating.
//
// It is fire-and-forget from the controller's perspective: extraction runs in a
// background goroutine so the UI stays responsive. Results surface as Notices on
// the controller's event sink.

package agent

import (
	"context"
	"fmt"
	"strings"

	"wenhao/internal/event"
	"wenhao/internal/provider"
)

// PostTurner is the interface the controller calls after every turn to drive
// background memory work. A nil PostTurner means no post-turn work is wired in.
type PostTurner interface {
	// AfterTurn fires after Run returns. msgs is the full session message slice
	// at the moment the turn completed. ctx is the turn's context (may be
	// cancelled shortly after the turn finishes, so background work should
	// derive its own context).
	AfterTurn(ctx context.Context, msgs []provider.Message)
}

// PostTurnConfig controls which post-turn work is enabled and with what
// parameters.
type PostTurnConfig struct {
	// SessionMemory, when non-nil, drives incremental session-memory updates.
	SessionMemory SessionMemoryDriver
	// ExtractMemories, when non-nil, drives durable-memory extraction.
	ExtractMemories ExtractMemoriesDriver
}

// SessionMemoryDriver abstracts the session-memory extraction so the agent
// package stays independent of the memory package.
type SessionMemoryDriver interface {
	// ShouldExtract returns true when session memory should fire.
	ShouldExtract(msgs []provider.Message) bool
	// StartExtraction marks an extraction as in-progress. Returns false when
	// one is already running.
	StartExtraction(msgs []provider.Message) bool
	// FinishExtraction marks the extraction as done and returns any stashed
	// trailing snapshot.
	FinishExtraction() []provider.Message
	// Run fires the actual extraction. Called from a background goroutine.
	Run(ctx context.Context, msgs []provider.Message, prov provider.Provider, sysPrompt string) (notice string, err error)
	// Content returns the current memory content for compaction injection.
	Content() string
}

// ExtractMemoriesDriver abstracts the durable-memory extraction.
type ExtractMemoriesDriver interface {
	// ShouldExtract returns true when extraction should fire.
	ShouldExtract(msgs []provider.Message) bool
	// StartExtraction marks an extraction as in-progress. Returns false when
	// one is already running.
	StartExtraction(msgs []provider.Message) bool
	// FinishExtraction marks the extraction as done and returns any stashed
	// trailing snapshot.
	FinishExtraction() []provider.Message
	// Run fires the actual extraction. Called from a background goroutine.
	Run(ctx context.Context, msgs []provider.Message, prov provider.Provider, sysPrompt string) (notice string, err error)
}

// PostTurnRunner is the concrete PostTurner that the controller creates.
type PostTurnRunner struct {
	cfg   PostTurnConfig
	sink  event.Sink
	prov  provider.Provider
	sys   string // system prompt (for forked extraction agent prompt building)
}

// NewPostTurnRunner creates a PostTurnRunner. Returns nil (the interface value,
// not a typed nil) when no drivers are configured, so the controller's nil-check
// works correctly.
func NewPostTurnRunner(cfg PostTurnConfig, sink event.Sink, prov provider.Provider, sys string) PostTurner {
	if cfg.SessionMemory == nil && cfg.ExtractMemories == nil {
		return nil
	}
	return &PostTurnRunner{cfg: cfg, sink: sink, prov: prov, sys: sys}
}

// AfterTurn checks both drivers and fires extractions in background goroutines.
func (r *PostTurnRunner) AfterTurn(ctx context.Context, msgs []provider.Message) {
	if r == nil {
		return
	}

	// Session memory: check trigger and fire.
	if r.cfg.SessionMemory != nil && r.cfg.SessionMemory.ShouldExtract(msgs) {
		if r.cfg.SessionMemory.StartExtraction(msgs) {
			go r.runSessionMemory(ctx, msgs)
		}
	}

	// Durable memory: check trigger and fire.
	if r.cfg.ExtractMemories != nil && r.cfg.ExtractMemories.ShouldExtract(msgs) {
		if r.cfg.ExtractMemories.StartExtraction(msgs) {
			go r.runExtractMemories(ctx, msgs)
		}
	}
}

func (r *PostTurnRunner) runSessionMemory(parentCtx context.Context, msgs []provider.Message) {
	ctx := context.WithoutCancel(parentCtx)
	notice, err := r.cfg.SessionMemory.Run(ctx, msgs, r.prov, r.sys)
	if err != nil {
		r.sink.Emit(event.Event{
			Kind:  event.Notice,
			Level: event.LevelWarn,
			Text:  fmt.Sprintf("session memory update failed: %v", err),
		})
	} else if notice != "" {
		r.sink.Emit(event.Event{
			Kind:  event.Notice,
			Level: event.LevelInfo,
			Text:  notice,
		})
	}

	// Check for trailing run.
	if trailing := r.cfg.SessionMemory.FinishExtraction(); trailing != nil {
		if r.cfg.SessionMemory.StartExtraction(trailing) {
			go r.runSessionMemory(ctx, trailing)
		}
	}
}

func (r *PostTurnRunner) runExtractMemories(parentCtx context.Context, msgs []provider.Message) {
	ctx := context.WithoutCancel(parentCtx)
	notice, err := r.cfg.ExtractMemories.Run(ctx, msgs, r.prov, r.sys)
	if err != nil {
		r.sink.Emit(event.Event{
			Kind:  event.Notice,
			Level: event.LevelWarn,
			Text:  fmt.Sprintf("memory extraction failed: %v", err),
		})
	} else if notice != "" {
		r.sink.Emit(event.Event{
			Kind:  event.Notice,
			Level: event.LevelInfo,
			Text:  notice,
		})
	}

	// Check for trailing run.
	if trailing := r.cfg.ExtractMemories.FinishExtraction(); trailing != nil {
		if r.cfg.ExtractMemories.StartExtraction(trailing) {
			go r.runExtractMemories(ctx, trailing)
		}
	}
}

// InjectSessionMemory reads the current session memory and returns it as a
// Markdown block suitable for insertion into the compaction summary context.
// Returns "" when no session memory driver is configured or the file is empty.
func (r *PostTurnRunner) InjectSessionMemory() string {
	if r == nil || r.cfg.SessionMemory == nil {
		return ""
	}
	content := strings.TrimSpace(r.cfg.SessionMemory.Content())
	if content == "" {
		return ""
	}
	return "\n## Session Memory\n\nThe following is auto-maintained session context. " +
		"Use it to ground the compaction summary in what's actually happened:\n\n" + content
}
