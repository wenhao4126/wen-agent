// Package agent — replace_context tool: lets the model actively manage its
// context window by replacing stale sections with new information. This is
// a complement to the passive compaction system: the model can proactively
// prune or update context when it knows certain information is outdated.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"wenhao/internal/event"
	"wenhao/internal/provider"
)

// ReplaceContextTool lets the model replace a range of messages in the session
// with new content. The model uses this when old context is stale (e.g. a file
// was rewritten and the old read_file result is now misleading) or when it
// wants to compress verbose tool output into a concise note.
//
// Security: only non-system messages can be replaced. The system prompt is
// immutable. The replacement is a user-role message so it doesn't confuse the
// assistant/tool message pairing.
type ReplaceContextTool struct {
	session *Session
	sink    event.Sink
}

// NewReplaceContextTool creates a replace_context tool bound to a session.
func NewReplaceContextTool(session *Session, sink event.Sink) *ReplaceContextTool {
	return &ReplaceContextTool{session: session, sink: sink}
}

func (t *ReplaceContextTool) Name() string { return "replace_context" }

func (t *ReplaceContextTool) Description() string {
	return "Replace a range of older messages in the conversation with a concise summary or updated information. " +
		"Use this when old context is stale or overly verbose: an early read_file result that no longer reflects the file, " +
		"a long grep output you've already acted on, or verbose tool results you can summarise into key facts. " +
		"The messages in the range are replaced by a single user-role message with your new content. " +
		"Only use on older messages — never replace the most recent turn. " +
		"The system prompt and compaction summaries are never replaced. " +
		"Prefer this over waiting for automatic compaction when you know exactly what's stale."
}

func (t *ReplaceContextTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "from_index":{"type":"integer","description":"Index of the first message to replace (0-based, after system prompt). Must be > 0 (never replace the system prompt).","minimum":1},
  "to_index":{"type":"integer","description":"Index of the last message to replace (inclusive, 0-based). Must be >= from_index and before the last 3 messages.","minimum":1},
  "new_content":{"type":"string","description":"The replacement content. Write a concise summary of what was in those messages, preserving any still-relevant file paths, decisions, and error text. Use bullet points."},
  "reason":{"type":"string","description":"Brief reason for the replacement — helps the user understand what was pruned (shown as a notice)."}
},
"required":["from_index","to_index","new_content"]
}`)
}

func (t *ReplaceContextTool) ReadOnly() bool { return false } // mutates session

func (t *ReplaceContextTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		FromIndex  int    `json:"from_index"`
		ToIndex    int    `json:"to_index"`
		NewContent string `json:"new_content"`
		Reason     string `json:"reason"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	msgs := t.session.Snapshot()
	if len(msgs) == 0 {
		return "", fmt.Errorf("session is empty")
	}

	// Validate bounds.
	head := 0
	if msgs[0].Role == provider.RoleSystem {
		head = 1
	}
	if p.FromIndex < head {
		return "", fmt.Errorf("from_index must be >= %d (cannot replace system prompt)", head)
	}
	if p.ToIndex < p.FromIndex {
		return "", fmt.Errorf("to_index must be >= from_index")
	}
	if p.ToIndex >= len(msgs) {
		return "", fmt.Errorf("to_index %d out of bounds (max %d)", p.ToIndex, len(msgs)-1)
	}
	// Don't allow replacing the last 3 messages (current turn context).
	if p.ToIndex >= len(msgs)-3 {
		return "", fmt.Errorf("refusing to replace the most recent messages (to_index must be < %d)", len(msgs)-3)
	}

	if strings.TrimSpace(p.NewContent) == "" {
		return "", fmt.Errorf("new_content is required")
	}

	// Build the new message list.
	compacted := make([]provider.Message, 0, len(msgs)-(p.ToIndex-p.FromIndex))
	compacted = append(compacted, msgs[:p.FromIndex]...)
	compacted = append(compacted, provider.Message{
		Role:    provider.RoleUser,
		Content: "<context-replacement>\n" + strings.TrimSpace(p.NewContent) + "\n</context-replacement>",
	})
	compacted = append(compacted, msgs[p.ToIndex+1:]...)
	t.session.Replace(compacted)

	reason := p.Reason
	if reason == "" {
		reason = fmt.Sprintf("replaced messages %d-%d", p.FromIndex, p.ToIndex)
	}
	t.sink.Emit(event.Event{
		Kind:  event.Notice,
		Level: event.LevelInfo,
		Text:  fmt.Sprintf("context: %s (%d messages → 1)", reason, p.ToIndex-p.FromIndex+1),
	})

	return fmt.Sprintf("Replaced %d messages (%d–%d) with the new content.", p.ToIndex-p.FromIndex+1, p.FromIndex, p.ToIndex), nil
}
