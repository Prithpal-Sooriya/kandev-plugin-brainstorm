package brainstorm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseConvergeHandlesFencesAndProse(t *testing.T) {
	text := "Sure! Here it is:\n```json\n" +
		`{"reply":"We have three stages.","board":{"goal":"Tame the backlog","stages":[{"title":"Stage 1","summary":"Reports {with braces}"}],` +
		`"ideas":[{"title":"Archive board","detail":"","status":"weird"}],"nextActions":[{"title":"Draft report","detail":"done when posted"}]}}` +
		"\n```\nLet me know."
	got, err := ParseConverge(text)
	if err != nil {
		t.Fatalf("ParseConverge: %v", err)
	}
	if got.Reply != "We have three stages." {
		t.Errorf("reply = %q", got.Reply)
	}
	if got.Board == nil || got.Board.Goal != "Tame the backlog" {
		t.Fatalf("board = %+v", got.Board)
	}
	if got.Board.Stages[0].Summary != "Reports {with braces}" {
		t.Errorf("stage summary = %q", got.Board.Stages[0].Summary)
	}
	if got.Board.Ideas[0].Status != "explore" {
		t.Errorf("unknown status should normalize to explore, got %q", got.Board.Ideas[0].Status)
	}
	if got.Board.Questions == nil || got.Board.Risks == nil || got.Board.Decisions == nil {
		t.Errorf("nil slices should normalize to empty: %+v", got.Board)
	}
}

func TestParseConvergeAcceptsTopLevelBoard(t *testing.T) {
	got, err := ParseConverge(`{"goal":"g","nextActions":[{"title":"a"}]}`)
	if err != nil {
		t.Fatalf("ParseConverge: %v", err)
	}
	if got.Board.Goal != "g" || got.Reply != "Board updated." {
		t.Errorf("got %+v", got)
	}
}

func TestParseConvergeSkipsUnrelatedObjects(t *testing.T) {
	text := `config {"theme":"dark"} then {} and finally {"reply":"r","board":{"goal":"real"}}`
	got, err := ParseConverge(text)
	if err != nil || got.Board.Goal != "real" || got.Reply != "r" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if _, err := ParseConverge(`{"reply":"x","board":{}}`); err == nil {
		t.Error("an empty board should be rejected")
	}
}

func TestParseConvergeRejectsProse(t *testing.T) {
	if _, err := ParseConverge("no json here {not json"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildPromptIncludesModeTopicAndTranscript(t *testing.T) {
	prompt := BuildPrompt(TurnRequest{
		Mode:    ModeChallenge,
		Topic:   "Backlog hygiene",
		Context: "Task: groom the backlog",
		Messages: []Message{
			{Role: "user", Content: "We have too many tickets."},
			{Role: "assistant", Content: "Which ones hurt most?"},
		},
		Board: &Board{Goal: "fewer tickets"},
	}, "Prefer British spelling.")
	for _, want := range []string{
		"MODE: CHALLENGE", "Session topic: Backlog hygiene", "Task: groom the backlog",
		"Person: We have too many tickets.", "Partner: Which ones hurt most?",
		"Prefer British spelling.", `"goal":"fewer tickets"`,
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestRenderTranscriptKeepsOpeningAndNewestWhenTrimming(t *testing.T) {
	var msgs []Message
	msgs = append(msgs, Message{Role: "user", Content: "OPENING"})
	for i := 0; i < 50; i++ {
		msgs = append(msgs, Message{Role: "assistant", Content: strings.Repeat("x", 100)})
	}
	msgs = append(msgs, Message{Role: "user", Content: "NEWEST"})
	out := renderTranscript(msgs, 1000)
	if !strings.HasPrefix(out, "Person: OPENING") || !strings.HasSuffix(out, "Person: NEWEST") {
		t.Errorf("trim lost the ends: %q...", out[:40])
	}
	if !strings.Contains(out, "earlier turns omitted") {
		t.Error("expected omission marker")
	}
	if len(out) > 1200 {
		t.Errorf("transcript not trimmed: %d chars", len(out))
	}
}

func waitJob(t *testing.T, jobs *Jobs, id string) Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, ok := jobs.Get(id)
		if !ok {
			t.Fatal("job vanished")
		}
		if job.Status != JobRunning {
			return job
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return Job{}
}

func TestJobsLifecycle(t *testing.T) {
	jobs := NewJobs(time.Second, time.Minute, 2)
	okID, _ := jobs.Start(func(context.Context) (TurnResult, error) { return TurnResult{Reply: "hi"}, nil })
	errID, _ := jobs.Start(func(context.Context) (TurnResult, error) { return TurnResult{}, errors.New("boom") })
	if job := waitJob(t, jobs, okID); job.Status != JobDone || job.Result.Reply != "hi" {
		t.Errorf("ok job = %+v", job)
	}
	if job := waitJob(t, jobs, errID); job.Status != JobError || job.Error != "boom" {
		t.Errorf("err job = %+v", job)
	}
	if _, ok := jobs.Get("missing"); ok {
		t.Error("unknown id should not be found")
	}
}

func TestJobsBusyAndRetention(t *testing.T) {
	jobs := NewJobs(time.Second, time.Minute, 1)
	release := make(chan struct{})
	id, err := jobs.Start(func(ctx context.Context) (TurnResult, error) {
		<-release
		return TurnResult{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.Start(func(context.Context) (TurnResult, error) { return TurnResult{}, nil }); !errors.As(err, &ErrBusy{}) {
		t.Errorf("expected ErrBusy, got %v", err)
	}
	close(release)
	waitJob(t, jobs, id)
	now := time.Now()
	jobs.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, ok := jobs.Get(id); ok {
		t.Error("finished job should expire after retention")
	}
}

func TestExtractReply(t *testing.T) {
	cases := map[string]string{
		"---\n## Skills\n- /path/SKILL.md\n<reply>\nHello there.\n</reply>": "Hello there.",
		"---\n\n## Skills\n- /a/SKILL.md\n\n## Prompts\n- /use-memory\n\n## Extensions\n- /x/jira.ts\n- https://github.com/o/r\n- [object Object]\nThe real answer.\n- with a bullet": "The real answer.\n- with a bullet",
		"- **Idea** - a diverge bullet\n- /not-a-banner": "- **Idea** - a diverge bullet\n- /not-a-banner",
		"plain answer  ":                                 "plain answer",
		"banner <reply>unterminated answer":              "unterminated answer",
		"<reply>first</reply> noise <reply>last</reply>": "last",
		"<reply>   </reply> fallback text":               "fallback text",
	}
	for in, want := range cases {
		if got := ExtractReply(in); got != want {
			t.Errorf("ExtractReply(%q) = %q, want %q", in, got, want)
		}
	}
}
