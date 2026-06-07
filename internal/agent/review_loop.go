// Package agent — Review Loop: a coordinator mode where an executor makes
// changes and a reviewer checks them in a feedback loop. Each round: the
// executor runs the task, then the reviewer inspects the diff. If the reviewer
// approves, the loop ends; otherwise the reviewer's feedback becomes the next
// round's task context. Configurable max rounds prevents infinite loops.

package agent

import (
	"context"
	"fmt"
	"strings"

	"wenhao/internal/event"
	"wenhao/internal/provider"
	"wenhao/internal/tool"
)

// ReviewLoop runs an executor→reviewer→feedback cycle. It is a Runner
// implementation, so it can be used wherever a single Agent or Coordinator
// is used.
type ReviewLoop struct {
	executorProv provider.Provider
	reviewerProv provider.Provider
	parentReg    *tool.Registry
	executorDef  AgentDefinition
	reviewerDef  AgentDefinition
	maxRounds    int
	temperature  float64
	pricing      *provider.Pricing
	contextWindow int
	archiveDir   string
	gate         Gate
	sink         event.Sink
}

// NewReviewLoop builds a review loop runner. executorDef and reviewerDef supply
// the system prompts and tool sets. maxRounds caps the number of review cycles
// (default 3).
func NewReviewLoop(executorProv, reviewerProv provider.Provider, parentReg *tool.Registry,
	executorDef, reviewerDef AgentDefinition, maxRounds int, temperature float64,
	pricing *provider.Pricing, contextWindow int, archiveDir string, gate Gate, sink event.Sink) *ReviewLoop {
	if maxRounds <= 0 {
		maxRounds = 3
	}
	return &ReviewLoop{
		executorProv:  executorProv,
		reviewerProv:  reviewerProv,
		parentReg:     parentReg,
		executorDef:   executorDef,
		reviewerDef:   reviewerDef,
		maxRounds:     maxRounds,
		temperature:   temperature,
		pricing:       pricing,
		contextWindow: contextWindow,
		archiveDir:    archiveDir,
		gate:          gate,
		sink:          sink,
	}
}

// Run implements Runner.
func (r *ReviewLoop) Run(ctx context.Context, input string) error {
	for round := 1; round <= r.maxRounds; round++ {
		r.sink.Emit(event.Event{
			Kind: event.Phase,
			Text: fmt.Sprintf("review round %d/%d · executing", round, r.maxRounds),
		})

		// 1. Executor implements or refines the task.
		execReg := buildAgentReg(r.parentReg, r.executorDef)
		execResult, err := RunSubAgent(ctx, r.executorProv, execReg,
			r.executorDef.EffectiveSystemPrompt(), input, Options{
				Temperature:   r.temperature,
				Pricing:       r.pricing,
				Gate:          r.gate,
				ContextWindow: r.contextWindow,
				ArchiveDir:    r.archiveDir,
			}, r.sink)
		if err != nil {
			return fmt.Errorf("executor round %d: %w", round, err)
		}

		// 2. Reviewer inspects the executor's result.
		r.sink.Emit(event.Event{
			Kind: event.Phase,
			Text: fmt.Sprintf("review round %d/%d · reviewing", round, r.maxRounds),
		})

		reviewReg := buildAgentReg(r.parentReg, r.reviewerDef)
		reviewInput := fmt.Sprintf("Review the following work:\n\n%s\n\nOriginal task: %s\n\nProvide your verdict on one line (APPROVED or NEEDS_WORK), then bullet points of issues to fix. If APPROVED, say so clearly and don't list nits.", execResult, input)
		reviewResult, err := RunSubAgent(ctx, r.reviewerProv, reviewReg,
			r.reviewerDef.EffectiveSystemPrompt(), reviewInput, Options{
				Temperature:   r.temperature,
				Pricing:       r.pricing,
				Gate:          r.gate,
				ContextWindow: r.contextWindow,
				ArchiveDir:    r.archiveDir,
			}, r.sink)
		if err != nil {
			return fmt.Errorf("reviewer round %d: %w", round, err)
		}

		// 3. Check verdict.
		if isApproved(reviewResult) {
			r.sink.Emit(event.Event{
				Kind:  event.Notice,
				Level: event.LevelInfo,
				Text:  fmt.Sprintf("review approved after %d round(s)", round),
			})
			return nil
		}

		// 4. Feedback becomes the next round's input.
		input = fmt.Sprintf("The reviewer gave feedback. Address each issue:\n\n%s\n\nOriginal task: %s", reviewResult, input)
		r.sink.Emit(event.Event{
			Kind:  event.Notice,
			Level: event.LevelInfo,
			Text:  fmt.Sprintf("review round %d: changes requested, refining…", round),
		})
	}

	return fmt.Errorf("review loop: exceeded max rounds (%d)", r.maxRounds)
}

// isApproved checks whether the reviewer's output signals approval.
func isApproved(review string) bool {
	upper := strings.ToUpper(strings.TrimSpace(review))
	// Check the first line for a clear verdict.
	lines := strings.SplitN(upper, "\n", 2)
	firstLine := strings.TrimSpace(lines[0])
	if strings.Contains(firstLine, "APPROVED") && !strings.Contains(firstLine, "NOT APPROVED") {
		return true
	}
	// Also check if there are no actionable issues listed.
	if !strings.Contains(upper, "NEEDS_WORK") &&
		!strings.Contains(upper, "ISSUE") &&
		!strings.Contains(upper, "FIX") &&
		!strings.Contains(upper, "CHANGE") {
		// Review looks clean — consider it approved.
		return strings.Contains(upper, "LOOKS GOOD") ||
			strings.Contains(upper, "LGTM") ||
			strings.Contains(upper, "SHIP") ||
			strings.Contains(upper, "NO ISSUES")
	}
	return false
}

// buildAgentReg constructs a tool registry from a definition.
func buildAgentReg(parent *tool.Registry, def AgentDefinition) *tool.Registry {
	if len(def.Tools) > 0 {
		return FilterRegistry(parent, def.Tools, SubagentMetaTools()...)
	}
	if len(def.DisallowedTools) > 0 {
		return FilterRegistry(parent, nil, append(def.DisallowedTools, SubagentMetaTools()...)...)
	}
	return FilterRegistry(parent, nil, SubagentMetaTools()...)
}
