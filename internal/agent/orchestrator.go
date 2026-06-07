// Package agent — orchestrator mode: the main agent acts purely as a task
// orchestrator. It receives user requests, decomposes them into sub-tasks,
// delegates each sub-task to a specialized sub-agent via the `agent` tool, and
// synthesizes results. The main agent never executes tools directly (no
// read_file, grep, edit_file, bash, etc.) — keeping its context permanently
// clean and cache-friendly.
//
// Enable in reasonix.toml:
//   [agent]
//   mode = "orchestrator"
//
// The orchestrator tools are a curated subset: agent (spawn sub-agents),
// todo_write (track progress), ask (confirm with user), and optionally
// read_file + ls for lightweight context inspection.

package agent

import (
	"strings"

	"wenhao/internal/tool"
)

// OrchestratorMode is the agent.mode value that enables pure orchestration.
const OrchestratorMode = "orchestrator"

// OrchestratorSystemPrompt is the system prompt for orchestrator mode. It
// steers the model away from direct execution and toward delegation.
const OrchestratorSystemPrompt = `You are a task orchestrator. You CANNOT read files, search code, edit files, or run commands — those tools are not available to you. Your ONLY available tools are: agent (spawn sub-agents), todo_write (track tasks), and ask (confirm with user).

## How to handle ANY user request

1. Understand what the user wants
2. Break it into concrete sub-tasks (use todo_write)
3. For EACH sub-task, spawn a sub-agent via the agent tool:
   - agent_type="explore" for codebase questions, reading files, searching
   - agent_type="general-purpose" for implementation work
   - agent_type="code-reviewer" for reviewing changes
   - agent_type="test-runner" for running tests/commands
4. When sub-agents return, synthesize results and reply

## Sub-agent prompt tips

Be specific and self-contained in every sub-agent prompt. The sub-agent does NOT see this conversation, so include all context it needs: file paths, what to look for, what format to reply in. Write prompts as directives ("Read X and report Y") not open-ended questions.

## Example

User: "What does the README say?"
Your response:
1. todo_write: [{content: "Read README", status: "in_progress"}]
2. agent(description="Read README", agent_type="explore", prompt="Read the file README.md at the project root. Summarize its contents in 3-5 bullet points.")

Then wait for the sub-agent to return, update the todo, and reply to the user.`

// OrchestratorTools is the tool allowlist for orchestrator mode. The main agent
// may ONLY call these tools; all others are filtered out of its registry. The
// agent tool is the primary workhorse — everything else goes through it.
//
// read_file and ls are included for lightweight context (e.g. reading
// REASONIX.md or checking a file exists) but the prompt steers the model AWAY
// from using them for real work.
// OrchestratorTools is the tool allowlist for orchestrator mode. The main agent
// may ONLY call these tools; all others are filtered out. The only way to
// interact with files, code, or commands is through the agent tool.
var OrchestratorTools = []string{
	"agent",
	"task",
	"todo_write",
	"ask",
	"complete_step",
	"wait",
	"bash_output",
}

// IsOrchestratorMode reports whether the given mode string enables orchestration.
func IsOrchestratorMode(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), OrchestratorMode)
}

// OrchestratorToolSet returns the tool names the orchestrator may call.
func OrchestratorToolSet() []string {
	out := make([]string, len(OrchestratorTools))
	copy(out, OrchestratorTools)
	return out
}

// FilterOrchestratorRegistry builds a tool registry containing only the
// orchestrator's allowed tools from the parent registry.
func FilterOrchestratorRegistry(parent *tool.Registry) *tool.Registry {
	sub := tool.NewRegistry()
	for _, name := range OrchestratorTools {
		if tl, ok := parent.Get(name); ok {
			sub.Add(tl)
		}
	}
	return sub
}
