package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

// fakeHost implements only what the plugin calls; the embedded nil Host
// panics if any other method is reached, which keeps the test honest.
type fakeHost struct {
	pluginsdk.Host
	config     map[string]any
	reply      string
	err        error
	gotPrompt  string
	gotProfile string
	task       *pluginsdk.Task
}

func (f *fakeHost) GetConfig(context.Context) (map[string]any, error) { return f.config, nil }

func (f *fakeHost) InvokeUtilityAgent(_ context.Context, prompt string, opts ...pluginsdk.UtilityAgentOptions) (string, error) {
	f.gotPrompt = prompt
	if len(opts) > 0 {
		f.gotProfile = opts[0].ProfileID
	}
	return f.reply, f.err
}

func (f *fakeHost) Tasks() pluginsdk.TaskReader { return fakeTasks{f: f} }

type fakeTasks struct {
	pluginsdk.TaskReader
	f *fakeHost
}

func (t fakeTasks) Get(_ context.Context, id string) (*pluginsdk.Task, error) {
	if t.f.task == nil || t.f.task.ID != id {
		return nil, errors.New("not found")
	}
	return t.f.task, nil
}
func newTestPlugin(h *fakeHost) *brainstormPlugin {
	p := newPlugin()
	p.host = func() pluginsdk.Host { return h }
	return p
}

func pollTurn(t *testing.T, p *brainstormPlugin, id string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, _ := p.HandleWebhook(context.Background(), &pluginsdk.WebhookRequest{WebhookKey: "turn", Method: "GET", Query: "id=" + id})
		if resp.Status != 200 {
			t.Fatalf("poll status %d: %s", resp.Status, resp.Body)
		}
		var job map[string]any
		_ = json.Unmarshal(resp.Body, &job)
		if job["status"] != "running" {
			return job
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("turn did not finish")
	return nil
}

func startTurn(t *testing.T, p *brainstormPlugin, body string) string {
	t.Helper()
	resp, _ := p.HandleWebhook(context.Background(), &pluginsdk.WebhookRequest{WebhookKey: "turn", Method: "POST", Body: []byte(body)})
	if resp.Status != 202 {
		t.Fatalf("start status %d: %s", resp.Status, resp.Body)
	}
	var out map[string]string
	_ = json.Unmarshal(resp.Body, &out)
	return out["id"]
}

func TestChatTurnUsesConfiguredProfile(t *testing.T) {
	h := &fakeHost{config: map[string]any{"agent_profile": " prof-1 ", "persona_notes": "Be brief."}, reply: "  Nice idea.  "}
	p := newTestPlugin(h)
	id := startTurn(t, p, `{"mode":"chat","messages":[{"role":"user","content":"hello"}]}`)
	job := pollTurn(t, p, id)
	if job["status"] != "done" {
		t.Fatalf("job = %v", job)
	}
	if reply := job["result"].(map[string]any)["reply"]; reply != "Nice idea." {
		t.Errorf("reply = %v", reply)
	}
	if h.gotProfile != "prof-1" || !strings.Contains(h.gotPrompt, "Be brief.") {
		t.Errorf("profile=%q prompt has notes=%v", h.gotProfile, strings.Contains(h.gotPrompt, "Be brief."))
	}
}

func TestConvergeTurnReturnsBoardOrFallsBack(t *testing.T) {
	h := &fakeHost{reply: `{"reply":"recap","board":{"goal":"g"}}`}
	p := newTestPlugin(h)
	job := pollTurn(t, p, startTurn(t, p, `{"mode":"converge","messages":[{"role":"user","content":"x"}]}`))
	result := job["result"].(map[string]any)
	if result["board"].(map[string]any)["goal"] != "g" {
		t.Errorf("board = %v", result["board"])
	}

	h.reply = "I could not produce JSON"
	job = pollTurn(t, p, startTurn(t, p, `{"mode":"converge","messages":[{"role":"user","content":"x"}]}`))
	result = job["result"].(map[string]any)
	if _, has := result["board"]; has || !strings.Contains(result["reply"].(string), "could not be updated") {
		t.Errorf("fallback result = %v", result)
	}
}

func TestAgentErrorSurfacesOnJob(t *testing.T) {
	h := &fakeHost{err: errors.New("rpc error: code = FailedPrecondition desc = bad profile")}
	p := newTestPlugin(h)
	job := pollTurn(t, p, startTurn(t, p, `{"messages":[{"role":"user","content":"x"}]}`))
	if job["status"] != "error" || !strings.Contains(job["error"].(string), "agent profile") {
		t.Errorf("job = %v", job)
	}
}

func TestTurnValidation(t *testing.T) {
	p := newTestPlugin(&fakeHost{})
	cases := map[string]int32{
		`not json`:                        400,
		`{"mode":"rant","messages":[{}]}`: 400,
		`{"mode":"chat","messages":[]}`:   400,
	}
	for body, want := range cases {
		resp, _ := p.HandleWebhook(context.Background(), &pluginsdk.WebhookRequest{WebhookKey: "turn", Method: "POST", Body: []byte(body)})
		if resp.Status != want {
			t.Errorf("%s: status %d, want %d", body, resp.Status, want)
		}
	}
	resp, _ := p.HandleWebhook(context.Background(), &pluginsdk.WebhookRequest{WebhookKey: "other", Method: "POST"})
	if resp.Status != 404 {
		t.Errorf("unknown key status %d", resp.Status)
	}
	resp, _ = p.HandleWebhook(context.Background(), &pluginsdk.WebhookRequest{WebhookKey: "turn", Method: "GET", Query: "id=nope"})
	if resp.Status != 404 {
		t.Errorf("unknown job status %d", resp.Status)
	}
}

func TestTaskContextAction(t *testing.T) {
	h := &fakeHost{task: &pluginsdk.Task{ID: "t1", WorkspaceID: "w1", Title: "Groom backlog", Description: strings.Repeat("d", maxContextChars+10)}}
	p := newTestPlugin(h)
	resp, _ := p.HandleAction(context.Background(), &pluginsdk.PluginActionRequest{ActionKey: "task.context", Context: pluginsdk.VerifiedActionContext{TaskID: "t1"}})
	var out map[string]string
	_ = json.Unmarshal(resp.Body, &out)
	if resp.Status != 200 || out["title"] != "Groom backlog" || out["workspaceId"] != "w1" {
		t.Fatalf("resp %d %v", resp.Status, out)
	}
	if !strings.HasSuffix(out["description"], "…") {
		t.Error("long description should be truncated")
	}
	resp, _ = p.HandleAction(context.Background(), &pluginsdk.PluginActionRequest{ActionKey: "task.context", Context: pluginsdk.VerifiedActionContext{TaskID: "missing"}})
	if resp.Status != 404 {
		t.Errorf("missing task status %d", resp.Status)
	}
}
