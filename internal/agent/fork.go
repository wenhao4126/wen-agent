// Package agent — Fork semantics: a sub-agent that shares the parent's
// conversation context (system prompt, tools, message history) so both sides
// benefit from prompt-cache reuse. A fork runs in an isolated session with its
// own tool-call budget, but starts from the same prefix the parent uses.
//
// "Fork" is the default when the model calls the agent tool without specifying
// an agent_type: the sub-agent inherits the parent's full context and tools
// (minus meta-tools), the same as Claude Code's fork semantics.

package agent

import (
	"context"

	"wenhao/internal/event"
	"wenhao/internal/provider"
	"wenhao/internal/tool"
)

// ForkAgent runs a sub-agent that inherits the parent's conversation context.
// The system prompt, tools, and message history are shared so the sub-agent
// starts with the same knowledge as the parent. Only its final answer is
// returned; its intermediate tool calls and reasoning are hidden.
//
// Cache implication: because the fork uses identical system prompt + tool
// schemas + message prefix, the provider's prompt cache can hit on the shared
// prefix — the fork pays only for the delta (its own tool turns + final answer).
type ForkAgent struct {
	prov          provider.Provider
	pricing       *provider.Pricing
	parentReg     *tool.Registry
	maxSteps      int
	contextWindow int
	softCompactRatio  float64
	compactRatio      float64
	compactForceRatio float64
	temperature   float64
	archiveDir    string
	gate          Gate
}

// NewForkAgent builds a fork runner. It uses the same provider and tool registry
// as the parent, sharing the cache-stable prefix.
func NewForkAgent(prov provider.Provider, pricing *provider.Pricing, parentReg *tool.Registry,
	maxSteps, contextWindow int, softCompactRatio, compactRatio, compactForceRatio, temperature float64,
	archiveDir string, gate Gate) *ForkAgent {
	return &ForkAgent{
		prov:              prov,
		pricing:           pricing,
		parentReg:         parentReg,
		maxSteps:          maxSteps,
		contextWindow:     contextWindow,
		softCompactRatio:  softCompactRatio,
		compactRatio:      compactRatio,
		compactForceRatio: compactForceRatio,
		temperature:       temperature,
		archiveDir:        archiveDir,
		gate:              gate,
	}
}

// Run executes the fork: it creates a sub-agent session from the parent's
// system prompt + message history, then runs the task prompt to completion.
// The sub-agent's tool calls are forwarded to sink (for UI nesting), and only
// its final assistant answer is returned.
//
// parentSys is the parent's cached system prompt; parentMsgs is the full
// conversation history at the fork point. The fork starts from this prefix
// so the provider's cache can hit.
func (f *ForkAgent) Run(ctx context.Context, parentSys string, parentMsgs []provider.Message, task string, sink event.Sink) (string, error) {
	// Clone the parent's system prompt and message history so the fork starts
	// from the same cache-stable prefix. The fork's own session is a fresh
	// continuation from the fork point.
	subReg := FilterRegistry(f.parentReg, nil, SubagentMetaTools()...)

	// Build the fork session: system prompt + parent messages + the task as a
	// fresh user message. This keeps the prefix byte-identical to the parent.
	sess := NewSession(parentSys)
	// Copy the parent messages (after system prompt) into the fork session.
	for _, m := range parentMsgs {
		if m.Role == provider.RoleSystem {
			continue // system prompt already set
		}
		sess.Add(m)
	}

	sub := New(f.prov, subReg, sess, Options{
		MaxSteps:          f.maxSteps,
		Temperature:       f.temperature,
		Pricing:           f.pricing,
		Gate:              f.gate,
		ContextWindow:     f.contextWindow,
		SoftCompactRatio:  f.softCompactRatio,
		CompactRatio:      f.compactRatio,
		CompactForceRatio: f.compactForceRatio,
		ArchiveDir:        f.archiveDir,
	}, sink)

	if err := sub.Run(ctx, task); err != nil {
		return "", err
	}

	// Return the last assistant message with content. Use Snapshot for
	// safe concurrent access since the session may still be referenced.
	msgs := sess.Snapshot()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleAssistant && msgs[i].Content != "" {
			return msgs[i].Content, nil
		}
	}
	return "", nil
}

// ForkToolRegistry builds the default tool set for a fork: all parent tools
// except meta-tools (agent/task/run_skill/install_skill) that would let the
// fork recursively spawn.
func ForkToolRegistry(parent *tool.Registry) *tool.Registry {
	return FilterRegistry(parent, nil, SubagentMetaTools()...)
}

// forkMetaTools lists the tools excluded from fork sub-agents.
var forkMetaTools = []string{"task", "agent", "run_skill", "install_skill", "explore", "research", "review", "security_review"}

// ForkMetaTools returns the tool names a fork sub-agent should not inherit.
func ForkMetaTools() []string {
	out := make([]string, len(forkMetaTools))
	copy(out, forkMetaTools)
	return out
}
