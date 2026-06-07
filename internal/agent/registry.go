// Package agent — Agent Registry: configurable agent definitions that define
// sub-agent types the model can spawn via the agent tool. Similar to Claude
// Code's AgentDefinition, each entry carries a system prompt, tool set, model
// preference, isolation mode, and metadata that steers when and how the agent
// is used.
//
// Agents are loaded from wenhao.toml ([[agents]] sections) and overlaid onto
// built-in defaults (general-purpose, explore, code-reviewer, test-runner).
// User-defined entries with the same type override built-ins.

package agent

import (
	"fmt"
	"strings"
)

// IsolationMode selects how a spawned agent's workspace is separated from the
// parent's. "in_process" shares the same directory and tool registry (least
// isolation, fastest startup); "worktree" creates a temporary git worktree so
// file changes are isolated and can be discarded or merged; "remote" delegates
// to a remote CCR environment (async only).
type IsolationMode string

const (
	IsolationInProcess IsolationMode = "in_process"
	IsolationWorktree  IsolationMode = "worktree"
	IsolationRemote    IsolationMode = "remote"
)

// AgentDefinition describes one agent type the model can spawn.
type AgentDefinition struct {
	// Type is the canonical identifier, used as the agent_type argument to the
	// agent tool. Must be a valid skill-name-like string (letters/digits/_-.).
	Type string
	// Description is a one-liner shown in the agent listing attachment.
	Description string
	// WhenToUse steers the model toward this agent for the right tasks — a
	// short phrase like "after writing significant code, before declaring done".
	WhenToUse string
	// SystemPrompt is the agent's persona and instructions. Loaded from a file
	// when SystemPromptFile is set.
	SystemPrompt     string
	SystemPromptFile string
	// Tools is the allowlist of tool names the agent may call. Empty means all
	// parent tools except meta-tools (task/agent/run_skill/install_skill).
	Tools []string
	// DisallowedTools lists tool names to exclude even when Tools is empty (i.e.
	// "all parent tools except these"). Only one of Tools/DisallowedTools should
	// be set.
	DisallowedTools []string
	// Model, when non-empty, names a configured provider/model to use for this
	// agent. Empty inherits the parent's model.
	Model string
	// Effort is the reasoning effort override for this agent.
	Effort string
	// Isolation selects the workspace isolation level.
	Isolation IsolationMode
	// MaxSteps caps the agent's tool-call rounds. 0 inherits the parent's cap.
	MaxSteps int
	// Builtin reports whether this is a shipped built-in (true) or a
	// user-configured override (false).
	Builtin bool
}

// Validate reports whether the definition has the minimum required fields.
func (d AgentDefinition) Validate() error {
	if !isValidAgentType(d.Type) {
		return fmt.Errorf("invalid agent type %q: must be alphanumeric with dashes/underscores/dots", d.Type)
	}
	if strings.TrimSpace(d.Description) == "" {
		return fmt.Errorf("agent %q: description is required", d.Type)
	}
	return nil
}

// EffectiveSystemPrompt returns the resolved system prompt. When
// SystemPromptFile is set, it is loaded at boot time by the caller.
func (d AgentDefinition) EffectiveSystemPrompt() string {
	if strings.TrimSpace(d.SystemPrompt) != "" {
		return d.SystemPrompt
	}
	return DefaultTaskSystemPrompt
}

// EffectiveIsolation returns the resolved isolation mode, defaulting to
// in_process.
func (d AgentDefinition) EffectiveIsolation() IsolationMode {
	switch d.Isolation {
	case IsolationWorktree, IsolationRemote:
		return d.Isolation
	default:
		return IsolationInProcess
	}
}

// formatAgentLine returns the one-line agent listing entry used in the
// agent_listing attachment, matching Claude Code's format.
func (d AgentDefinition) formatAgentLine() string {
	tools := d.toolsDescription()
	return fmt.Sprintf("- %s: %s (Tools: %s)", d.Type, d.WhenToUse, tools)
}

func (d AgentDefinition) toolsDescription() string {
	if len(d.Tools) > 0 {
		return strings.Join(d.Tools, ", ")
	}
	if len(d.DisallowedTools) > 0 {
		return "All tools except " + strings.Join(d.DisallowedTools, ", ")
	}
	return "All tools"
}

// Registry holds the agent definitions available to a session, resolved from
// config and built-ins. Lookup by type; the first match wins (user overrides
// built-ins).
type Registry struct {
	agents []AgentDefinition
	byType map[string]AgentDefinition
}

// NewRegistry builds a Registry from the given definitions. Later entries
// override earlier ones with the same Type.
func NewRegistry(defs []AgentDefinition) *Registry {
	r := &Registry{byType: map[string]AgentDefinition{}}
	for _, d := range defs {
		if err := d.Validate(); err != nil {
			continue
		}
		key := strings.ToLower(d.Type)
		if _, exists := r.byType[key]; !exists || !d.Builtin {
			r.byType[key] = d
		}
	}
	// Build the ordered list.
	seen := map[string]bool{}
	for _, d := range defs {
		key := strings.ToLower(d.Type)
		if seen[key] {
			continue
		}
		if rd, ok := r.byType[key]; ok {
			r.agents = append(r.agents, rd)
			seen[key] = true
		}
	}
	return r
}

// Get returns the agent definition for type, or the zero value when not found.
func (r *Registry) Get(agentType string) (AgentDefinition, bool) {
	if r == nil {
		return AgentDefinition{}, false
	}
	d, ok := r.byType[strings.ToLower(strings.TrimSpace(agentType))]
	return d, ok
}

// List returns all registered agent definitions in registration order.
func (r *Registry) List() []AgentDefinition {
	if r == nil {
		return nil
	}
	return r.agents
}

// AgentListingAttachment returns the Markdown attachment block that tells the
// model which agents are available and when to use them. Follows Claude Code's
// agent_listing_delta format. An empty registry returns "".
func (r *Registry) AgentListingAttachment() string {
	if r == nil || len(r.agents) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Available agent types:\n")
	for _, d := range r.agents {
		b.WriteString(d.formatAgentLine())
		b.WriteString("\n")
	}
	return b.String()
}

// isValidAgentType checks an agent type identifier.
func isValidAgentType(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i, r := range s {
		if i == 0 && !isAlphaNum(r) {
			return false
		}
		if !isAlphaNum(r) && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}

func isAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// --- Built-in agent definitions ---

// BuiltinAgentDefs returns the shipped agent definitions that are always
// available. User config entries with the same Type override these.
func BuiltinAgentDefs() []AgentDefinition {
	return []AgentDefinition{
		{
			Type:        "general-purpose",
			Description: "General-purpose agent for researching complex questions and executing multi-step tasks. Use for tasks that require searching across multiple files/directories, patterns, or investigation.",
			WhenToUse:   "for complex research tasks requiring multiple steps",
			SystemPrompt: `You are a general-purpose sub-agent. Carry out the task the parent gave you.
Use the tools available to investigate or act. Return a single concise final answer.
Be thorough but efficient — don't over-explore or re-derive what the parent already knows.
If you can't complete the task with the tools available, say so plainly.`,
			Builtin: true,
		},
		{
			Type:        "explore",
			Description: "Fast agent specialized in exploring codebases. Use for finding files by patterns, searching code, answering specific questions about the codebase.",
			WhenToUse:   "for codebase exploration and search tasks",
			SystemPrompt: `You are a code-exploration sub-agent. Your job is to answer specific questions about the codebase.
Cast a wide net first (grep for symbols, ls/glob for structure), then drill into the most relevant files.
Stop as soon as you can answer the question. Your final answer should be one paragraph with file:line citations.
If you can't find something, say which searches you ran — a negative claim needs evidence.`,
			Tools:   []string{"read_file", "ls", "glob", "grep"},
			Builtin: true,
		},
		{
			Type:        "code-reviewer",
			Description: "Reviews code changes for correctness, security, and best practices. Use after writing significant code.",
			WhenToUse:   "after writing significant code, before declaring done",
			SystemPrompt: `You are a code-reviewer sub-agent. Review the changes the parent made.
Look for: correctness bugs, security issues, missing error handling, test gaps.
Be specific: cite file:line, describe the problem in one sentence, suggest the fix.
Lead with a one-sentence verdict. If everything looks clean, say so. Don't manufacture issues.`,
			Tools:   []string{"read_file", "grep", "glob", "bash"},
			Model:   "", // inherit parent
			Builtin: true,
		},
		{
			Type:        "test-runner",
			Description: "Runs tests and diagnoses failures. Use after making code changes to verify nothing is broken.",
			WhenToUse:   "after code changes to verify correctness",
			SystemPrompt: `You are a test-runner sub-agent. Run the tests for the project.
1. Detect the test command from project files (go.mod → go test, package.json → npm test, etc.)
2. Run it and capture output.
3. If tests fail, diagnose the failures: which test, what error, where.
4. Report clearly: "all passed" or "N failures: [summary]".
Do NOT fix code — only diagnose. Report failures with file:line and error text.`,
			Tools:   []string{"read_file", "grep", "glob", "bash"},
			Builtin: true,
		},
	}
}
