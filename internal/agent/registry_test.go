package agent

import (
	"testing"
)

func TestAgentRegistry_Builtins(t *testing.T) {
	defs := BuiltinAgentDefs()
	if len(defs) < 3 {
		t.Fatalf("expected at least 3 built-in agents, got %d", len(defs))
	}

	reg := NewRegistry(defs)
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}

	// Check that built-in types are present.
	for _, want := range []string{"general-purpose", "explore", "code-reviewer", "test-runner"} {
		if _, ok := reg.Get(want); !ok {
			t.Errorf("expected built-in agent %q to be present", want)
		}
	}

	// Check listing.
	list := reg.List()
	if len(list) < 3 {
		t.Fatalf("expected at least 3 agents in listing, got %d", len(list))
	}
}

func TestAgentRegistry_UserOverride(t *testing.T) {
	// User defines an override for explore.
	userDefs := []AgentDefinition{
		{
			Type:        "explore",
			Description: "My custom explorer",
			WhenToUse:   "for everything",
			SystemPrompt: "You are a custom explorer.",
			Tools:       []string{"read_file", "grep"},
			Builtin:     false,
		},
	}

	// Merge with built-ins.
	defs := append(BuiltinAgentDefs(), userDefs...)
	reg := NewRegistry(defs)

	d, ok := reg.Get("explore")
	if !ok {
		t.Fatal("expected explore agent to exist")
	}
	if d.Description != "My custom explorer" {
		t.Errorf("expected custom description, got %q", d.Description)
	}
	if d.Builtin {
		t.Error("expected Builtin=false for user override")
	}

	// Other built-ins should still be intact.
	if _, ok := reg.Get("code-reviewer"); !ok {
		t.Error("expected code-reviewer to still be present after override")
	}
}

func TestAgentRegistry_Listing(t *testing.T) {
	reg := NewRegistry(BuiltinAgentDefs())
	listing := reg.AgentListingAttachment()

	if listing == "" {
		t.Fatal("expected non-empty listing")
	}

	// Should contain agent types.
	for _, want := range []string{"general-purpose", "explore", "code-reviewer", "test-runner"} {
		if !contains(listing, want) {
			t.Errorf("listing should contain %q", want)
		}
	}
}

func TestAgentDefinition_Validate(t *testing.T) {
	// Valid.
	d := AgentDefinition{
		Type:        "my-agent",
		Description: "Does things",
	}
	if err := d.Validate(); err != nil {
		t.Errorf("expected valid agent, got: %v", err)
	}

	// Invalid type.
	d2 := AgentDefinition{
		Type:        "",
		Description: "No type",
	}
	if err := d2.Validate(); err == nil {
		t.Error("expected error for empty type")
	}

	// Missing description.
	d3 := AgentDefinition{
		Type: "no-desc",
	}
	if err := d3.Validate(); err == nil {
		t.Error("expected error for missing description")
	}
}

func TestAgentRegistry_Get_Missing(t *testing.T) {
	reg := NewRegistry(BuiltinAgentDefs())
	if _, ok := reg.Get("nonexistent"); ok {
		t.Error("expected false for missing agent type")
	}

	// Nil registry.
	var nilReg *Registry
	if _, ok := nilReg.Get("anything"); ok {
		t.Error("expected false for nil registry")
	}
	if nilReg.List() != nil {
		t.Error("expected nil for nil registry List()")
	}
	if nilReg.AgentListingAttachment() != "" {
		t.Error("expected empty listing for nil registry")
	}
}

func TestAgentDefinition_EffectiveSystemPrompt(t *testing.T) {
	d := AgentDefinition{
		Type:         "test",
		Description:  "test agent",
		SystemPrompt: "Custom prompt",
	}
	if d.EffectiveSystemPrompt() != "Custom prompt" {
		t.Error("expected custom prompt")
	}

	d2 := AgentDefinition{
		Type:        "test2",
		Description: "no prompt",
	}
	if d2.EffectiveSystemPrompt() != DefaultTaskSystemPrompt {
		t.Error("expected default system prompt when none set")
	}
}

func TestAgentDefinition_EffectiveIsolation(t *testing.T) {
	// Default.
	d := AgentDefinition{Type: "test", Description: "test"}
	if d.EffectiveIsolation() != IsolationInProcess {
		t.Errorf("expected in_process default, got %s", d.EffectiveIsolation())
	}

	// Explicit.
	d2 := AgentDefinition{Type: "test2", Description: "test", Isolation: IsolationWorktree}
	if d2.EffectiveIsolation() != IsolationWorktree {
		t.Errorf("expected worktree, got %s", d2.EffectiveIsolation())
	}
}

func TestForkMetaTools(t *testing.T) {
	tools := ForkMetaTools()
	if len(tools) == 0 {
		t.Fatal("expected non-empty fork meta tools")
	}
	// task and agent must be excluded.
	hasTask := false
	hasAgent := false
	for _, name := range tools {
		if name == "task" {
			hasTask = true
		}
		if name == "agent" {
			hasAgent = true
		}
	}
	if !hasTask {
		t.Error("expected 'task' in fork meta tools")
	}
	if !hasAgent {
		t.Error("expected 'agent' in fork meta tools")
	}
}

func TestReviewLoop_IsApproved(t *testing.T) {
	tests := []struct {
		review   string
		expected bool
	}{
		{"APPROVED", true},
		{"APPROVED — looks good", true},
		{"NEEDS_WORK: fix the nil check", false},
		{"Looks good to me", false}, // no clear approval keyword
		{"LGTM", true},
		{"SHIP IT", true},
		{"No issues found", true},
		{"NOT APPROVED — security concern", false},
		{"", false},
	}

	for _, tc := range tests {
		got := isApproved(tc.review)
		if got != tc.expected {
			t.Errorf("isApproved(%q) = %v, want %v", tc.review, got, tc.expected)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
