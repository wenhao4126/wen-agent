package cli

import (
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"

	"wenhao/internal/config"
	"wenhao/internal/i18n"
)

// runModelSubcommand handles "/model": with no argument it opens an interactive
// picker (↑/↓ to browse, Enter to select); "/model <ref>" switches the session
// to that model in place, carrying the conversation across.
func (m *chatTUI) runModelSubcommand(input string) {
	args := tokenizeArgs(input) // args[0] == "/model"
	if len(args) < 2 {
		m.openModelPicker()
		return
	}
	ref := args[1]
	if m.buildController == nil {
		m.notice(i18n.M.ModelSwitchUnavailable)
		return
	}
	if m.ctrl.Running() {
		m.notice(i18n.M.ModelSwitchBusy)
		return
	}
	if ref == m.modelRef {
		m.notice(fmt.Sprintf(i18n.M.ModelAlreadyOnFmt, ref))
		return
	}
	carried := m.ctrl.History()
	prevPath := m.ctrl.SessionPath()
	if err := m.ctrl.Snapshot(); err != nil {
		slog.Warn("model switch: snapshot failed", "err", err)
	}
	m.notice(fmt.Sprintf(i18n.M.ModelSwitchingFmt, ref))

	// Capture old controller for cleanup after the async build succeeds.
	oldCtrl := m.ctrl
	build := m.buildController

	// Fire the build off the event loop; the result arrives as a tea.Cmd.
	// Both the build AND the old-controller close run in the goroutine so
	// neither blocks the bubbletea event loop. The old controller's Close
	// kills plugin subprocesses (incl. CodeGraph), which can disrupt the
	// terminal's cancelReader if called synchronously inside Update — so it
	// must happen here, before we hand the new controller back.
	m.modelSwitchPending = true
	m.pendingModelSwitch = func() tea.Msg {
		c, err := build(ref, carried, prevPath)
		if err != nil {
			return modelSwitchMsg{ref: ref, err: err}
		}
		// Do NOT close the old controller here. Controller.Close() runs
		// SessionEnd hooks (arbitrary shell commands) and kills plugin
		// subprocesses — operations that corrupt bubbletea's terminal raw
		// mode when executed from a goroutine. Instead, pass the old
		// controller back in the message so the Update handler can defer
		// its cleanup as a tea.Cmd that runs after the next render.
		return modelSwitchMsg{
			ref:      ref,
			ctrl:     c,
			oldCtrl:  oldCtrl,
			label:    c.Label(),
			commands: c.Commands(),
			skills:   c.Skills(),
			host:     c.Host(),
		}
	}
}

// --- Model picker overlay ---

type modelRef struct {
	ref    string // "provider/model"
	active bool   // currently selected model
}

// modelPicker is an in-chat overlay for "/model" that lets the user pick a
// model with ↑/↓ and confirm with Enter. Mirrors the resumePicker pattern.
type modelPicker struct {
	models []modelRef
	sel    int // selected index
}

// openModelPicker populates the picker from configured providers and opens it.
func (m *chatTUI) openModelPicker() {
	cfg, err := config.Load()
	if err != nil {
		m.notice("model: " + err.Error())
		return
	}
	var refs []modelRef
	sel := 0
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		if !p.Configured() {
			continue
		}
		for _, model := range p.ModelList() {
			ref := p.Name + "/" + model
			active := ref == m.modelRef
			refs = append(refs, modelRef{ref: ref, active: active})
			if active {
				sel = len(refs) - 1
			}
		}
	}
	if len(refs) == 0 {
		m.notice(i18n.M.ModelSwitchUnavailable)
		return
	}
	m.modelPick = &modelPicker{models: refs, sel: sel}
}

func (m chatTUI) handleModelPickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.modelPick
	if p == nil {
		return m, nil
	}
	switch msg.String() {
	case "up", "k":
		if p.sel > 0 {
			p.sel--
		}
	case "down", "j":
		if p.sel < len(p.models)-1 {
			p.sel++
		}
	case "enter":
		return m.applyModelPick()
	case "esc":
		m.modelPick = nil
	}
	return m, nil
}

func (m chatTUI) applyModelPick() (tea.Model, tea.Cmd) {
	p := m.modelPick
	if p == nil || p.sel < 0 || p.sel >= len(p.models) {
		return m, nil
	}
	ref := p.models[p.sel].ref
	m.modelPick = nil
	// Delegate to the existing model switch logic.
	m.runModelSubcommand("/model " + ref)
	// Return the pending model switch command so bubbletea executes the async build.
	return m, m.pendingModelSwitch
}

func (m chatTUI) renderModelPicker() string {
	p := m.modelPick
	if p == nil {
		return ""
	}
	w := max(m.width, 10)
	var b strings.Builder
	b.WriteString(accent(i18n.M.ModelPickTitle) + "\n")
	for i, r := range p.models {
		label := r.ref
		if r.active {
			label += " " + dim(i18n.M.ModelPickActive)
		}
		b.WriteString(rowLine(i == p.sel, i+1, "", label, false) + "\n")
	}
	b.WriteString(dim(i18n.M.ModelPickHint))
	return choicePanelStyle.Width(w).Render(b.String())
}

// modelRefs returns the configured provider/model refs for slash completion.
func modelRefs() []string {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	var out []string
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		if !p.Configured() {
			continue
		}
		for _, model := range p.ModelList() {
			out = append(out, p.Name+"/"+model)
		}
	}
	return out
}
