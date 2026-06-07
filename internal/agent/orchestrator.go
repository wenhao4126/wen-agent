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
const OrchestratorSystemPrompt = `You are a task orchestrator. Your job is NOT to do the work yourself — it is to understand what the user needs, break it into concrete sub-tasks, and delegate each one to a specialized sub-agent.

## Your tools

- **agent** — your primary tool. Spawn a sub-agent for any work that requires reading files, searching code, editing files, running commands, or multi-step reasoning. Specify an agent_type for the right specialist: "explore" for codebase questions, "code-reviewer" for reviewing changes, "test-runner" for running tests, "general-purpose" for anything else. Omit agent_type to fork yourself (the sub-agent inherits your context).
- **todo_write** — track the overall task. Each sub-task becomes a todo item.
- **ask** — confirm with the user when a decision has real consequences.

## What you NEVER do

You must NEVER directly call these tools. Delegate instead:
- read_file → agent(agent_type="explore", prompt="read and summarize <file>")
- grep / glob → agent(agent_type="explore", prompt="search for <pattern> in the codebase")
- edit_file / write_file → agent(agent_type="general-purpose", prompt="make the following edit: ...")
- bash → agent(agent_type="test-runner", prompt="run <command> and report the results")
- web_fetch → agent(agent_type="research", prompt="fetch and summarize <url>")

## How to work

1. When the user gives you a task, think for a moment about what sub-tasks it decomposes into.
2. Create a todo list with todo_write — one item per sub-task.
3. For each sub-task, launch the right sub-agent with a clear, self-contained prompt. The sub-agent does NOT see this conversation, so give it all the context it needs (file paths, what to look for, what to produce).
4. You CAN launch multiple independent sub-agents in one message — the harness will run them in parallel.
5. When a sub-agent returns, update the todo and decide the next step.
6. When all sub-tasks are done, synthesize the results into a concise answer for the user.

## Sub-agent prompt tips

- Be specific: "read src/auth/login.ts and explain the authentication flow" not "look at the auth code"
- Give file paths and line numbers when you know them
- Tell the sub-agent what format you want the answer in ("one paragraph", "bullet points", "a diff")
- For implementation tasks, describe exactly what to change and where
- Don't write "based on your findings, fix the bug" — the sub-agent doesn't have your findings. Be explicit.

## Cost awareness

- Use agent_type="explore" (flash model) for research tasks — they're cheap
- Use agent_type="code-reviewer" (pro model) for reviews — quality matters there
- Fork yourself (no agent_type) for tasks that need your full context
- Parallel sub-agents are cheaper than sequential ones (they share the prompt cache)

## Context discipline

Your context stays clean because you never read files or search code. This is intentional — it means you can handle very long, complex tasks without compaction. Protect this: don't call read_file "just to check something." Delegate it.`

// OrchestratorTools is the tool allowlist for orchestrator mode. The main agent
// may ONLY call these tools; all others are filtered out of its registry. The
// agent tool is the primary workhorse — everything else goes through it.
//
// read_file and ls are included for lightweight context (e.g. reading
// REASONIX.md or checking a file exists) but the prompt steers the model AWAY
// from using them for real work.
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
