// Package brainstorm holds the plugin's domain logic: the request/response
// wire shapes, the prompt builder for each brainstorming mode, the board
// parser, and the in-memory job store that lets the browser poll a slow
// agent turn instead of holding one long HTTP request open.
package brainstorm

// Mode selects how the brainstorm partner responds to the latest turn.
type Mode string

const (
	// ModeChat is the default conversational turn: reflect, build on the
	// idea, ask at most one clarifying question.
	ModeChat Mode = "chat"
	// ModeDiverge asks for a burst of new, varied ideas.
	ModeDiverge Mode = "diverge"
	// ModeChallenge plays devil's advocate: risks, blind spots, assumptions.
	ModeChallenge Mode = "challenge"
	// ModeConverge summarizes the whole session into a structured board.
	ModeConverge Mode = "converge"
)

// Valid reports whether m is one of the supported modes.
func (m Mode) Valid() bool {
	switch m {
	case ModeChat, ModeDiverge, ModeChallenge, ModeConverge:
		return true
	}
	return false
}

// Message is one transcript entry. Role is "user" or "assistant".
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// TurnRequest is the body the browser POSTs to start an agent turn.
type TurnRequest struct {
	Mode     Mode      `json:"mode"`
	Topic    string    `json:"topic,omitempty"`
	Context  string    `json:"context,omitempty"`
	Messages []Message `json:"messages"`
	Board    *Board    `json:"board,omitempty"`
}

// TurnResult is what a finished turn produces. Board is set only for
// ModeConverge.
type TurnResult struct {
	Reply string `json:"reply"`
	Board *Board `json:"board,omitempty"`
}

// Board is the structured summary the partner maintains for a session.
type Board struct {
	Goal        string       `json:"goal"`
	Stages      []Stage      `json:"stages"`
	Ideas       []Idea       `json:"ideas"`
	Questions   []string     `json:"questions"`
	Decisions   []string     `json:"decisions"`
	Risks       []string     `json:"risks"`
	NextActions []NextAction `json:"nextActions"`
}

// Stage is one phase of a staged plan ("Stage 1: reports", ...).
type Stage struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

// Idea is one captured idea. Status is "explore", "parked", or "chosen".
type Idea struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Status string `json:"status"`
}

// NextAction is a concrete step that can become a Kandev task.
type NextAction struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}
