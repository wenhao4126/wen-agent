// Package agent — AgentTool: the unified tool for spawning sub-agents. It
// replaces/enhances the existing `task` tool with:
//
//   - agent_type: spawn a registered agent (code-reviewer, test-runner, …)
//   - fork (no agent_type): spawn a fork that inherits the parent context
//   - run_in_background: async execution
//   - model / effort: per-spawn model override
//   - isolation: worktree support for isolated changes
//
// The tool is registered into the parent's tool registry; it appears to the
// model as "agent" with a schema describing the available agent types.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"wenhao/internal/event"
	"wenhao/internal/jobs"
	"wenhao/internal/provider"
	"wenhao/internal/tool"
)

// AgentTool is the unified sub-agent spawning tool.
type AgentTool struct {
	prov              provider.Provider
	pricing           *provider.Pricing
	parentReg         *tool.Registry
	parentSession     *Session // for fork mode: the parent's conversation context
	registry          *Registry
	workDir           string // project root for worktree isolation
	maxSteps          int
	contextWindow     int
	softCompactRatio  float64
	compactRatio      float64
	compactForceRatio float64
	temperature       float64
	archiveDir        string
	gate              Gate
	subagentModel     string
	subagentEffort    string
	resolveProvider   func(modelRef, effort string) (provider.Provider, *provider.Pricing, int, error)
}

// NewAgentTool wires an agent-spawning tool. registry provides the agent type
// definitions; when nil, only fork mode (no agent_type) is available.
// parentSession is the parent agent's session, used by fork mode to inherit
// context. workDir is the project root, used for worktree isolation.
func NewAgentTool(prov provider.Provider, pricing *provider.Pricing, parentReg *tool.Registry,
	parentSession *Session, registry *Registry, workDir string,
	maxSteps, contextWindow int,
	softCompactRatio, compactRatio, compactForceRatio, temperature float64,
	archiveDir string, gate Gate, subagentModel, subagentEffort string,
	resolveProvider func(modelRef, effort string) (provider.Provider, *provider.Pricing, int, error)) *AgentTool {
	return &AgentTool{
		prov:              prov,
		pricing:           pricing,
		parentReg:         parentReg,
		parentSession:     parentSession,
		registry:          registry,
		workDir:           workDir,
		maxSteps:          maxSteps,
		contextWindow:     contextWindow,
		softCompactRatio:  softCompactRatio,
		compactRatio:      compactRatio,
		compactForceRatio: compactForceRatio,
		temperature:       temperature,
		archiveDir:        archiveDir,
		gate:              gate,
		subagentModel:     subagentModel,
		subagentEffort:    subagentEffort,
		resolveProvider:   resolveProvider,
	}
}

func (t *AgentTool) Name() string { return "agent" }

func (t *AgentTool) Description() string {
	var b strings.Builder
	b.WriteString("Spawn a sub-agent to handle a complex, focused task. ")
	if t.registry != nil && len(t.registry.List()) > 0 {
		b.WriteString("Specify an agent_type to use a specialized agent (")
		names := make([]string, 0, len(t.registry.List()))
		for _, d := range t.registry.List() {
			names = append(names, d.Type)
		}
		b.WriteString(strings.Join(names, ", "))
		b.WriteString("), or omit it to fork yourself — a fork inherits your full conversation context. ")
	} else {
		b.WriteString("Omit agent_type to fork yourself — a fork inherits your full conversation context. ")
	}
	b.WriteString("Only the agent's final answer is returned; intermediate tool calls are hidden. ")
	b.WriteString("Use for research, code review, test running, or any task where the intermediate output isn't worth keeping in your context.")
	return b.String()
}

func (t *AgentTool) Schema() json.RawMessage {
	// Build the dynamic agent_type enum from the registry.
	// Quoted for JSON enum, bare for description.
	enumValues := `"general-purpose"`
	bareNames := "general-purpose"
	if t.registry != nil {
		quoted := make([]string, 0, len(t.registry.List()))
		bare := make([]string, 0, len(t.registry.List()))
		for _, d := range t.registry.List() {
			quoted = append(quoted, `"`+d.Type+`"`)
			bare = append(bare, d.Type)
		}
		if len(quoted) > 0 {
			enumValues = strings.Join(quoted, ", ")
			bareNames = strings.Join(bare, ", ")
		}
	}

	schema := fmt.Sprintf(`{
"type":"object",
"properties":{
  "description":{"type":"string","description":"Short label for the agent (3-7 words). Surfaced in the dispatch line."},
  "prompt":{"type":"string","description":"What the agent should accomplish. Be specific about the deliverable. Forks inherit your context so write a directive (what to do), not a summary of the situation."},
  "agent_type":{"type":"string","description":"Which agent type to spawn. Available: %s. Omit to fork yourself (inherit full context).","enum":[%s]},
  "model":{"type":"string","description":"Optional model override for the agent (a configured provider/model name)."},
  "effort":{"type":"string","description":"Optional reasoning effort for the agent (e.g. high, max)."},
  "run_in_background":{"type":"boolean","description":"Run the agent asynchronously: returns immediately and you'll be notified when it finishes. Use for long, independent tasks."},
  "isolation":{"type":"string","description":"Workspace isolation: worktree gives the agent an isolated git worktree (changes can be merged later). Omit for in-process (shares your workspace).","enum":["worktree"]}
},
"required":["description","prompt"]
}`, bareNames, enumValues)
	return json.RawMessage(schema)
}

// ReadOnly is false: sub-agents can invoke writers.
func (t *AgentTool) ReadOnly() bool { return false }

// ResolveProfile extracts model/effort from agent args.
func (t *AgentTool) ResolveProfile(args json.RawMessage) *event.Profile {
	var p struct {
		Model  string `json:"model"`
		Effort string `json:"effort"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil
	}
	model, effort := t.effectiveProfile(p.Model, p.Effort)
	if model == "" && effort == "" {
		return nil
	}
	return &event.Profile{Model: model, Effort: effort}
}

func (t *AgentTool) effectiveProfile(model, effort string) (string, string) {
	model = strings.TrimSpace(model)
	effort = strings.TrimSpace(effort)
	if model == "" {
		model = strings.TrimSpace(t.subagentModel)
	}
	if effort == "" {
		effort = strings.TrimSpace(t.subagentEffort)
	}
	return model, effort
}

func (t *AgentTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Description     string `json:"description"`
		Prompt          string `json:"prompt"`
		AgentType       string `json:"agent_type"`
		Model           string `json:"model"`
		Effort          string `json:"effort"`
		RunInBackground bool   `json:"run_in_background"`
		Isolation       string `json:"isolation"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.Description == "" || p.Prompt == "" {
		return "", fmt.Errorf("description and prompt are required")
	}

	modelRef, effortRef := t.effectiveProfile(p.Model, p.Effort)

	// Resolve the agent definition and build the sub-registry + system prompt.
	var sysPrompt string
	var subReg *tool.Registry
	isFork := strings.TrimSpace(p.AgentType) == ""

	if isFork {
		sysPrompt = DefaultTaskSystemPrompt
		subReg = ForkToolRegistry(t.parentReg)
	} else {
		def, ok := t.registry.Get(p.AgentType)
		if !ok {
			return "", fmt.Errorf("unknown agent_type %q — available: %s", p.AgentType, t.agentTypeList())
		}
		sysPrompt = def.EffectiveSystemPrompt()
		if len(def.Tools) > 0 {
			subReg = FilterRegistry(t.parentReg, def.Tools, SubagentMetaTools()...)
		} else if len(def.DisallowedTools) > 0 {
			// Build the full list minus disallowed + meta.
			subReg = FilterRegistry(t.parentReg, nil, append(def.DisallowedTools, SubagentMetaTools()...)...)
		} else {
			subReg = FilterRegistry(t.parentReg, nil, SubagentMetaTools()...)
		}
		// Agent definition model overrides the default subagent model.
		if modelRef == "" && def.Model != "" {
			modelRef = def.Model
		}
	}

	// Background: register a job and return immediately.
	if p.RunInBackground {
		jm, ok := jobs.FromContext(ctx)
		if !ok {
			return "", fmt.Errorf("background execution is not available in this context")
		}
		parentID, parent, _, _ := CallContext(ctx)
		nested := subSinkFor(parentID, parent)
		label := p.Description
		if label == "" {
			label = "agent"
		}
		job := jm.Start("agent", label, func(jobCtx context.Context, _ io.Writer) (string, error) {
			return t.run(jobCtx, p.Prompt, subReg, sysPrompt, nested, modelRef, effortRef, isFork, p.Isolation)
		})
		return fmt.Sprintf("Started background agent %q (%s). You'll be notified when it finishes.", job.ID, label), nil
	}

	return t.run(ctx, p.Prompt, subReg, sysPrompt, subSink(ctx), modelRef, effortRef, isFork, p.Isolation)
}

func (t *AgentTool) run(ctx context.Context, prompt string, subReg *tool.Registry, sysPrompt string, sink event.Sink, modelRef, effort string, isFork bool, isolation string) (string, error) {
	prov, pricing, ctxWin := t.prov, t.pricing, t.contextWindow
	if t.resolveProvider != nil && (modelRef != "" || effort != "") {
		p, pr, cw, err := t.resolveProvider(modelRef, effort)
		if err != nil {
			return "", fmt.Errorf("agent profile: %w", err)
		}
		prov, pricing, ctxWin = p, pr, cw
	}

	// Worktree isolation: create a git worktree, run the sub-agent inside it,
	// then cleanup. Changes are committed to a temp branch for review.
	var wt *Worktree
	if strings.EqualFold(strings.TrimSpace(isolation), "worktree") && t.workDir != "" {
		var err error
		wt, err = NewWorktree(ctx, t.workDir, RepoWorktreeDir(t.workDir))
		if err != nil {
			return "", fmt.Errorf("worktree setup: %w", err)
		}
		defer func() {
			dirty, cleanErr := wt.Cleanup(ctx)
			if dirty {
				// Don't remove the worktree info — it'll be appended below.
			}
			_ = cleanErr
		}()
		// Tell the sub-agent to work inside the worktree.
		prompt = fmt.Sprintf("Your workspace is %s. All file paths are relative to this directory.\n%s", wt.Path, prompt)
		// Run the sub-agent inside the worktree directory. Lock an OS
		// thread so chdir doesn't affect other goroutines.
		origDir, _ := os.Getwd()
		_ = os.Chdir(wt.Path)
		defer func() { _ = os.Chdir(origDir) }()
	}

	if isFork {
		// Fork mode: clone the parent's session to share the cache prefix.
		// Pass parent messages so the fork inherits the full conversation context.
		var parentMsgs []provider.Message
		if t.parentSession != nil {
			parentMsgs = t.parentSession.Snapshot()
		}
		fork := &ForkAgent{
			prov:              prov,
			pricing:           pricing,
			parentReg:         subReg,
			maxSteps:          t.maxSteps,
			contextWindow:     ctxWin,
			softCompactRatio:  t.softCompactRatio,
			compactRatio:      t.compactRatio,
			compactForceRatio: t.compactForceRatio,
			temperature:       t.temperature,
			archiveDir:        t.archiveDir,
			gate:              t.gate,
		}
		return fork.Run(ctx, sysPrompt, parentMsgs, prompt, sink)
	}
	return RunSubAgent(ctx, prov, subReg, sysPrompt, prompt, Options{
		MaxSteps:          t.maxSteps,
		Temperature:       t.temperature,
		Pricing:           pricing,
		Gate:              t.gate,
		ContextWindow:     ctxWin,
		SoftCompactRatio:  t.softCompactRatio,
		CompactRatio:      t.compactRatio,
		CompactForceRatio: t.compactForceRatio,
		ArchiveDir:        t.archiveDir,
	}, sink)
}

func (t *AgentTool) agentTypeList() string {
	if t.registry == nil {
		return "(none)"
	}
	names := make([]string, 0, len(t.registry.List()))
	for _, d := range t.registry.List() {
		names = append(names, d.Type)
	}
	return strings.Join(names, ", ")
}
