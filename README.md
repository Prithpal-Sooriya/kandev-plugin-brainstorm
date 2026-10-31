# kandev-plugin-brainstorm

**Brainstorm** is a [Kandev](https://github.com/kdlbs/kandev) plugin that gives
you an agent "brainstorming partner": chat through an idea, push it wide, poke
holes in it, then converge it onto a structured board and turn the results into
Kandev tasks with one click.

## What it does

### The `/brainstorm` page

A **Brainstorm** item in the sidebar opens a three-pane workspace:

| Left | Middle | Right |
| --- | --- | --- |
| Your saved brainstorm sessions (per user, per workspace) | Chat with the brainstorming partner | Live **Board** for the current session |

Sessions are saved through Kandev's `host.storage`, so they follow you across
browsers and restarts.

### The task panel

From a task's workspace **+** menu, add the **Brainstorm** panel. Each task gets
exactly one session, seeded with the task's title and description (read via a
`task.context` action). The panel uses compact **Chat / Board** tabs, and
**Create task** there creates a **subtask** of the current task.

### The partner's persona

- Stays at the altitude you ask for: product-level questions get product-level
  answers, not code.
- Concise; builds on *your* idea instead of replacing it.
- Asks at most one clarifying question per reply.
- Accepts corrections without arguing.
- When automation comes up, prefers options that are reversible and auditable.

### Modes

| Mode | How | What you get |
| --- | --- | --- |
| **Chat** | `Enter` | A normal conversational turn |
| **Diverge** | Diverge button | 5-8 genuinely different ideas |
| **Challenge** | Challenge button | Risks and blind spots, each with a mitigation |
| **Converge** | Converge button | The agent returns JSON that becomes the Board |

The **Board** holds: goal, stages, ideas (status `explore`, `parked` or
`chosen`), decisions, open questions, risks, and next actions.

### From board to tasks

Every next action and idea has a **Create task** button. It opens Kandev's
native create-task dialog prefilled with a title and a description containing
the item's detail plus context from the session (brainstorm title, goal,
decisions, risks). Items you've spun off show **✓ task**.

### Extras

- **Export** copies the whole session as Markdown (falls back to downloading a
  `.md` file if the clipboard is unavailable).
- **Dictation**: a mic button uses the browser Web Speech API when the webview
  supports it; it is hidden otherwise.

## Install

1. Download `kandev-plugin-brainstorm-<version>.tar.gz` from the
   [Releases](../../releases) page (or build it, see [Development](#development)).
2. In Kandev, go to **Settings → Plugins → Install from package** and pick the
   tarball.
3. Review and approve the requested capabilities:
   - `agent_invoke` — run one-shot agent turns for the brainstorming partner
   - `user_state` — save your sessions per user and workspace
   - `api_read: tasks` — read a task's title/description to seed the task panel
4. Open the plugin's settings and configure:
   - **Agent profile** (`agent_profile`) — which agent profile answers. Leave
     empty to use Kandev's default utility agent.
   - **Persona notes** (`persona_notes`) — optional extra instructions appended
     to the built-in persona (e.g. "We ship a mobile app; keep ideas
     offline-friendly").

**Privacy:** sessions are stored in Kandev's own database. Nothing is sent to a
third-party server beyond whatever the agent profile you choose talks to.

## Usage tips

- Start in **Chat** with a one-paragraph problem statement, then **Diverge**
  before you have an opinion; it's cheap to park ideas later.
- Use **Challenge** on the idea you like most, not the one you already doubt.
- **Converge** whenever you want a checkpoint; re-running it refreshes the
  Board from the whole conversation.
- If the partner drifts too low-level, just say so ("stay at the product
  level"); it will hold that altitude.
- Use the task panel for "how should I approach *this* task?" and the
  `/brainstorm` page for open-ended exploration.

## Development

Requirements: Go (version in `go.mod`), Node 24, and a sibling checkout of
Kandev at `../kandev-sdk`. `go.mod` has
`replace github.com/kandev/kandev => ../kandev-sdk/apps/backend`, and
`package.json` depends on
`@kandev/plugin-sdk: file:../kandev-sdk/apps/packages/plugin-sdk`.

```sh
git clone https://github.com/kdlbs/kandev ../kandev-sdk
git -C ../kandev-sdk checkout "$(cat .kandev-sdk-ref)"
```

`.kandev-sdk-ref` pins the exact SDK revision this plugin builds against; CI
checks out the same commit.

```sh
make deps          # npm install (once)
make test          # go test -race + vitest + tsc typecheck
make vet check-format
make package-host  # host-platform tarball, fastest to install locally
make package       # all 5 platforms (linux/darwin amd64+arm64, windows amd64)
```

The version lives in `manifest.yaml` (`version:`) and is mirrored in the
`Makefile` `VERSION` and `package.json`. To release, bump all three, merge, and
push a matching `vX.Y.Z` tag; the release workflow verifies the tag, builds
`make package`, and attaches the tarball plus `checksums.txt` to a GitHub
release.

## Architecture

```text
manifest.yaml               plugin id, capabilities, config schema, UI entry
server/
  plugin.go                 plugin wiring: config, actions, `turn` webhook
  brainstorm/types.go       session, message, and board types
  brainstorm/prompts.go     persona + per-mode prompts, transcript trimming
  brainstorm/board.go       parse/validate the Converge JSON into a board
  brainstorm/jobs.go        background turn jobs (concurrency, timeouts, TTL)
ui/
  src/register.tsx          registers the page, sidebar item, and task panel
  src/app.tsx               three-pane page + compact task-panel views
  src/model.ts              client-side session/board model
  src/api.ts                calls the `turn` webhook and host actions
  src/store.ts              session persistence via host.storage
  src/markdown.tsx          message rendering and Markdown export
  src/speech.ts             Web Speech API dictation
  src/runtime.ts            host/runtime helpers
  plugin.css                styles
  bundle.js                 esbuild output (generated)
tests/                      vitest suites for the UI half
```

### Why a webhook instead of a plain action?

Plugin actions are capped at 15 seconds, and an agent turn can take longer. So
the backend exposes an authenticated webhook, `turn`:

- `POST` starts a background job and returns `202 {"id": "..."}`.
- `GET ?id=<id>` polls it: `running`, `done` (with the reply), or `error`.

Each job calls `host.InvokeUtilityAgent` (a one-shot invocation) with the
persona, mode prompt, and transcript. Transcripts are trimmed to roughly 48k
characters, always keeping the opening message. Limits: a turn times out after
5 minutes, at most 4 run concurrently, and finished results are kept in memory
for 10 minutes.

## Roadmap

- Persistent, managed agent conversations with tools/MCPs (for example,
  read access to an issue tracker so the partner can reference real tickets).
- Auto-refresh the Board as the conversation evolves, without an explicit
  Converge.
- Multi-user sharing of brainstorm sessions.

## License

MIT, see [LICENSE](LICENSE).
