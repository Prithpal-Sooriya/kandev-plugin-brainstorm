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
	raw, ok := extractJSONObject(text)
	if !ok {
		return TurnResult{}, errors.New("agent reply did not contain a JSON object")
	}
	var envelope struct {
		Reply string          `json:"reply"`
		Board json.RawMessage `json:"board"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return TurnResult{}, err
	}
	boardRaw := envelope.Board
	if len(boardRaw) == 0 {
		// Some models return the board fields at the top level.
		boardRaw = json.RawMessage(raw)
	}
	var board Board
	if err := json.Unmarshal(boardRaw, &board); err != nil {
		return TurnResult{}, err
	}
	normalizeBoard(&board)
	reply := strings.TrimSpace(envelope.Reply)
	if reply == "" {
		reply = "Board updated."
	}
	return TurnResult{Reply: reply, Board: &board}, nil
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

// extractJSONObject returns the first balanced top-level {...} span in s,
// honouring string literals so braces inside strings do not miscount.
func extractJSONObject(s string) (string, bool) {
	start := strings.IndexByte(s, '{')
	for start >= 0 {
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
					candidate := s[start : i+1]
					if json.Valid([]byte(candidate)) {
						return candidate, true
					}
					i = len(s)
				}
			}
		}
		next := strings.IndexByte(s[start+1:], '{')
		if next < 0 {
			break
		}
		start += next + 1
	}
	return "", false
}
