package brainstorm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MaxTranscriptChars bounds how much transcript is replayed into one prompt.
// The utility agent is one-shot, so every turn resends history; past this
// size the oldest middle turns are dropped (the opening message is kept
// because it usually states the problem).
const MaxTranscriptChars = 48000

const persona = `You are a brainstorming partner inside Kandev, a developer's agent workbench.
The person you are talking with thinks out loud: messages may be rambling,
dictated, or cut off mid-thought ("wait, wait", "sorry, go ahead"). Your job is
to help them think, not to take over.

Ground rules:
- Stay at the altitude they ask for. If they say "high level", do not drop into
  implementation detail until they ask.
- Be concise: short paragraphs or tight bullet lists. No filler, no flattery.
- Build on their idea first, then add one or two sharp additions of your own.
- Ask at most ONE clarifying question per reply, and only when it unblocks the
  next step. Never end every reply with a question by reflex.
- When they correct your summary, accept the correction and restate it exactly.
- Prefer reversible, auditable approaches when automation or autonomy comes up
  (e.g. mark as cancelled with a reason instead of deleting).
- Plain Markdown only: paragraphs, "- " bullets, **bold**, and "### " headings.`

func modeInstruction(mode Mode) string {
	switch mode {
	case ModeDiverge:
		return `MODE: DIVERGE. Generate 5-8 genuinely different ideas or angles on the
current topic, including at least one unconventional one. One line each,
"- **Name** - why it is interesting". Do not evaluate them yet.`
	case ModeChallenge:
		return `MODE: CHALLENGE. Act as a constructive devil's advocate. List the riskiest
assumptions, blind spots, failure modes, and who might object - each with a
one-line mitigation. Be direct but not dismissive.`
	case ModeConverge:
		return `MODE: CONVERGE. Summarize the whole conversation so far into a structured
board. Respond with ONLY a JSON object (no prose before or after, no code
fence) of this exact shape:
{
  "reply": "2-4 sentence plain-language recap, ending with the single most useful next step",
  "board": {
    "goal": "one sentence",
    "stages": [{"title": "Stage 1: ...", "summary": "..."}],
    "ideas": [{"title": "...", "detail": "...", "status": "explore|parked|chosen"}],
    "questions": ["open question"],
    "decisions": ["decision already made in the conversation"],
    "risks": ["risk"],
    "nextActions": [{"title": "imperative, task-sized", "detail": "what done looks like"}]
  }
}
Use the person's own framing and words (if they described stages, keep their
stages). Only record decisions they actually made. Keep 3-6 nextActions, each
small enough to hand to a coding or research agent as one task. Use empty
arrays rather than inventing content.`
	default:
		return `MODE: CHAT. Respond to the latest message as a thinking partner.`
	}
}

// BuildPrompt renders the one-shot prompt for a turn.
func BuildPrompt(req TurnRequest, personaNotes string) string {
	var b strings.Builder
	b.WriteString(persona)
	if notes := strings.TrimSpace(personaNotes); notes != "" {
		b.WriteString("\n\nExtra guidance from the operator:\n")
		b.WriteString(notes)
	}
	if topic := strings.TrimSpace(req.Topic); topic != "" {
		fmt.Fprintf(&b, "\n\nSession topic: %s", topic)
	}
	if ctx := strings.TrimSpace(req.Context); ctx != "" {
		b.WriteString("\n\nBackground the person attached to this session:\n")
		b.WriteString(ctx)
	}
	if req.Board != nil && req.Mode != ModeConverge {
		if raw, err := json.Marshal(req.Board); err == nil {
			b.WriteString("\n\nCurrent board (your last structured summary):\n")
			b.Write(raw)
		}
	}
	b.WriteString("\n\n")
	b.WriteString(modeInstruction(req.Mode))
	b.WriteString("\n\nConversation so far:\n")
	b.WriteString(renderTranscript(req.Messages, MaxTranscriptChars))
	if req.Mode == ModeConverge {
		b.WriteString("\n\nNow output the JSON object.")
	} else {
		b.WriteString("\n\nNow write your reply to the person (just the reply, no speaker label).")
	}
	return b.String()
}

// renderTranscript formats messages as labelled turns, dropping the oldest
// middle turns once the total exceeds limit.
func renderTranscript(messages []Message, limit int) string {
	lines := make([]string, 0, len(messages))
	for _, m := range messages {
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		speaker := "Person"
		if m.Role == "assistant" {
			speaker = "Partner"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", speaker, content))
	}
	if len(lines) == 0 {
		return "(no messages yet)"
	}
	total := 0
	for _, l := range lines {
		total += len(l) + 2
	}
	dropped := 0
	for total > limit && len(lines) > 2 {
		total -= len(lines[1]) + 2
		lines = append(lines[:1], lines[2:]...)
		dropped++
	}
	if dropped > 0 {
		marker := fmt.Sprintf("[... %d earlier turns omitted for length ...]", dropped)
		lines = append(lines[:1], append([]string{marker}, lines[1:]...)...)
	}
	return strings.Join(lines, "\n\n")
}
