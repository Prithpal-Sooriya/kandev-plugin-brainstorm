package brainstorm

import (
	"encoding/json"
	"errors"
	"strings"
)

// ParseConverge extracts the {"reply", "board"} object from a converge-mode
// completion. Models sometimes wrap JSON in a code fence or add a sentence
// around it, so this scans for the outermost balanced object rather than
// requiring the whole string to be JSON.
func ParseConverge(text string) (TurnResult, error) {
	for _, raw := range jsonObjects(text) {
		result, ok := parseEnvelope(raw)
		if ok {
			return result, nil
		}
	}
	return TurnResult{}, errors.New("agent reply did not contain a board JSON object")
}

// parseEnvelope accepts {"reply", "board": {...}} or a bare board object, and
// rejects objects that carry no board content at all (e.g. a stray {} or an
// unrelated JSON snippet the agent echoed before its answer).
func parseEnvelope(raw string) (TurnResult, bool) {
	var envelope struct {
		Reply string          `json:"reply"`
		Board json.RawMessage `json:"board"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return TurnResult{}, false
	}
	boardRaw := envelope.Board
	if len(boardRaw) == 0 || string(boardRaw) == "null" {
		boardRaw = json.RawMessage(raw)
	}
	var board Board
	if err := json.Unmarshal(boardRaw, &board); err != nil || boardEmpty(board) {
		return TurnResult{}, false
	}
	normalizeBoard(&board)
	reply := strings.TrimSpace(envelope.Reply)
	if reply == "" {
		reply = "Board updated."
	}
	return TurnResult{Reply: reply, Board: &board}, true
}

func boardEmpty(b Board) bool {
	return strings.TrimSpace(b.Goal) == "" && len(b.Stages) == 0 && len(b.Ideas) == 0 &&
		len(b.Questions) == 0 && len(b.Decisions) == 0 && len(b.Risks) == 0 && len(b.NextActions) == 0
}

func normalizeBoard(b *Board) {
	if b.Stages == nil {
		b.Stages = []Stage{}
	}
	if b.Ideas == nil {
		b.Ideas = []Idea{}
	}
	for i := range b.Ideas {
		switch b.Ideas[i].Status {
		case "explore", "parked", "chosen":
		default:
			b.Ideas[i].Status = "explore"
		}
	}
	if b.Questions == nil {
		b.Questions = []string{}
	}
	if b.Decisions == nil {
		b.Decisions = []string{}
	}
	if b.Risks == nil {
		b.Risks = []string{}
	}
	if b.NextActions == nil {
		b.NextActions = []NextAction{}
	}
}

// jsonObjects returns every balanced top-level {...} span in s that is valid
// JSON, in order, honouring string literals so braces inside strings do not
// miscount.
func jsonObjects(s string) []string {
	var out []string
	start := strings.IndexByte(s, '{')
	for start >= 0 {
		end := matchBrace(s, start)
		if end > 0 && json.Valid([]byte(s[start:end+1])) {
			out = append(out, s[start:end+1])
			next := strings.IndexByte(s[end+1:], '{')
			if next < 0 {
				break
			}
			start = end + 1 + next
			continue
		}
		next := strings.IndexByte(s[start+1:], '{')
		if next < 0 {
			break
		}
		start += next + 1
	}
	return out
}

// matchBrace returns the index of the brace closing the one at start, or -1.
func matchBrace(s string, start int) int {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
