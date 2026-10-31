package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/kandev/kandev/pkg/pluginsdk"

	"kandev-plugin-brainstorm/server/brainstorm"
)

const (
	webhookTurn       = "turn"
	actionTaskContext = "task.context"

	turnTimeout   = 5 * time.Minute
	turnRetention = 10 * time.Minute
	maxActive     = 4

	// maxContextChars caps the task description replayed into prompts.
	maxContextChars = 8000
)

// brainstormPlugin is the backend half: it turns browser requests into
// one-shot utility-agent calls and hands task context to the task panel.
type brainstormPlugin struct {
	pluginsdk.UnimplementedPlugin
	jobs *brainstorm.Jobs
	// host overrides UnimplementedPlugin.Host() in tests.
	host func() pluginsdk.Host
}

var _ pluginsdk.Plugin = (*brainstormPlugin)(nil)
var _ pluginsdk.ActionHandler = (*brainstormPlugin)(nil)

func newPlugin() *brainstormPlugin {
	p := &brainstormPlugin{jobs: brainstorm.NewJobs(turnTimeout, turnRetention, maxActive)}
	p.host = p.UnimplementedPlugin.Host
	return p
}

// HandleWebhook serves the authenticated "turn" webhook:
//
//	POST webhooks/turn        body: TurnRequest  -> 202 {"id": "..."}
//	GET  webhooks/turn?id=... -> 200 Job (running | done | error)
//
// Agent turns can outlast any reasonable HTTP request, so POST only starts
// the work and the browser polls GET until the job settles.
func (p *brainstormPlugin) HandleWebhook(ctx context.Context, req *pluginsdk.WebhookRequest) (*pluginsdk.WebhookResponse, error) {
	if req == nil || req.WebhookKey != webhookTurn {
		return jsonResponse(404, map[string]string{"error": "unknown webhook"}), nil
	}
	switch strings.ToUpper(req.Method) {
	case "POST":
		return p.startTurn(ctx, req.Body), nil
	case "GET":
		query, _ := url.ParseQuery(req.Query)
		job, ok := p.jobs.Get(query.Get("id"))
		if !ok {
			return jsonResponse(404, map[string]string{"error": "turn not found (it may have expired or the plugin restarted)"}), nil
		}
		return jsonResponse(200, job), nil
	default:
		return jsonResponse(405, map[string]string{"error": "method not allowed"}), nil
	}
}

func (p *brainstormPlugin) startTurn(ctx context.Context, body []byte) *pluginsdk.WebhookResponse {
	var turn brainstorm.TurnRequest
	if err := json.Unmarshal(body, &turn); err != nil {
		return jsonResponse(400, map[string]string{"error": "invalid JSON body"})
	}
	if turn.Mode == "" {
		turn.Mode = brainstorm.ModeChat
	}
	if !turn.Mode.Valid() {
		return jsonResponse(400, map[string]string{"error": fmt.Sprintf("unknown mode %q", turn.Mode)})
	}
	if len(turn.Messages) == 0 {
		return jsonResponse(400, map[string]string{"error": "messages must not be empty"})
	}
	host := p.host()
	if host == nil {
		return jsonResponse(503, map[string]string{"error": "plugin host is not connected yet"})
	}
	cfg := readConfig(ctx, host)
	id, err := p.jobs.Start(func(runCtx context.Context) (brainstorm.TurnResult, error) {
		return runTurn(runCtx, host, cfg, turn)
	})
	if err != nil {
		var busy brainstorm.ErrBusy
		if errors.As(err, &busy) {
			return jsonResponse(429, map[string]string{"error": err.Error()})
		}
		return jsonResponse(500, map[string]string{"error": err.Error()})
	}
	return jsonResponse(202, map[string]string{"id": id})
}

type config struct {
	ProfileID    string
	PersonaNotes string
}

func readConfig(ctx context.Context, host pluginsdk.Host) config {
	raw, err := host.GetConfig(ctx)
	if err != nil {
		return config{}
	}
	str := func(key string) string {
		v, _ := raw[key].(string)
		return strings.TrimSpace(v)
	}
	return config{ProfileID: str("agent_profile"), PersonaNotes: str("persona_notes")}
}

func runTurn(ctx context.Context, host pluginsdk.Host, cfg config, turn brainstorm.TurnRequest) (brainstorm.TurnResult, error) {
	prompt := brainstorm.BuildPrompt(turn, cfg.PersonaNotes)
	var opts []pluginsdk.UtilityAgentOptions
	if cfg.ProfileID != "" {
		opts = append(opts, pluginsdk.UtilityAgentOptions{ProfileID: cfg.ProfileID})
	}
	text, err := host.InvokeUtilityAgent(ctx, prompt, opts...)
	if err != nil {
		return brainstorm.TurnResult{}, friendlyAgentError(err)
	}
	if turn.Mode == brainstorm.ModeConverge {
		result, perr := brainstorm.ParseConverge(text)
		if perr != nil {
			// Keep the agent's words rather than failing the turn outright.
			return brainstorm.TurnResult{Reply: brainstorm.ExtractReply(text) +
				"\n\n_(The board could not be updated: the agent did not return valid JSON. Try Converge again.)_"}, nil
		}
		return result, nil
	}
	return brainstorm.TurnResult{Reply: brainstorm.ExtractReply(text)}, nil
}

func friendlyAgentError(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "FailedPrecondition"):
		return fmt.Errorf("the configured agent profile cannot run utility prompts; pick another under Settings > Plugins > Brainstorm (%s)", msg)
	case strings.Contains(msg, "PermissionDenied"):
		return fmt.Errorf("the plugin is not allowed to invoke agents; approve the agent_invoke capability (%s)", msg)
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("the agent took longer than 5 minutes; try a shorter turn")
	}
	return fmt.Errorf("agent call failed: %s", msg)
}

// HandleAction serves "task.context": the title and description of the task
// a panel is attached to, so the partner knows what the session is about.
// The host has already verified the caller can see this task.
func (p *brainstormPlugin) HandleAction(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	if req == nil || req.ActionKey != actionTaskContext {
		return actionJSON(404, map[string]string{"error": "unknown action"}), nil
	}
	host := p.host()
	if host == nil {
		return actionJSON(503, map[string]string{"error": "plugin host is not connected yet"}), nil
	}
	task, err := host.Tasks().Get(ctx, req.Context.TaskID)
	if err != nil || task == nil {
		return actionJSON(404, map[string]string{"error": "task not found"}), nil
	}
	description := task.Description
	if len(description) > maxContextChars {
		description = description[:maxContextChars] + "…"
	}
	return actionJSON(200, map[string]string{
		"title":       task.Title,
		"description": description,
		"workspaceId": task.WorkspaceID,
	}), nil
}

func jsonResponse(status int32, v any) *pluginsdk.WebhookResponse {
	body, _ := json.Marshal(v)
	return &pluginsdk.WebhookResponse{
		Status:  status,
		Headers: map[string]string{"Content-Type": "application/json", "Cache-Control": "no-store"},
		Body:    body,
	}
}

func actionJSON(status int, v any) *pluginsdk.PluginActionResponse {
	body, _ := json.Marshal(v)
	return &pluginsdk.PluginActionResponse{
		Status:  status,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    body,
	}
}
