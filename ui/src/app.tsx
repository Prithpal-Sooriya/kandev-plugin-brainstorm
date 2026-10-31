// The brainstorm workspace: session list, chat, and board. One component
// serves both surfaces — the full /brainstorm page (many sessions per
// workspace) and the compact task panel (one session per task).
import { runTurn, fetchTaskContext } from "./api";
import { Markdown } from "./markdown";
import {
  MODES,
  appendMessage,
  markSpunOff,
  newSession,
  sortSessions,
  taskDraft,
  toMarkdown,
  transcriptForAgent,
  withBoard,
  type Board,
  type Message,
  type Mode,
  type Session,
} from "./model";
import { h, Fragment, host, React } from "./runtime";
import { dictationSupported, startDictation, type Dictation } from "./speech";
import { deleteSession, loadSessions, saveSession, type SessionScope } from "./store";


export interface AppProps {
  scope: SessionScope;
  workspaceId: string | null;
  /** Set on the task panel: the session is about this task, spin-offs become subtasks. */
  taskId?: string;
  compact: boolean;
  writerId: string;
}

interface SpinOff {
  title: string;
  description: string;
  key: string;
}

function now(): string {
  return new Date().toISOString();
}

function useTaskCreation(workspaceId: string | null) {
  const [ctx, setCtx] = React.useState(() => (workspaceId ? host.context.getTaskCreationContext(workspaceId) : null));
  React.useEffect(() => {
    if (!workspaceId) return undefined;
    setCtx(host.context.getTaskCreationContext(workspaceId));
    return host.context.subscribeTaskCreationContext(workspaceId, setCtx);
  }, [workspaceId]);
  return ctx;
}

export function BrainstormApp(props: AppProps) {
  const { scope, workspaceId, taskId, compact, writerId } = props;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const ui = host.ui as any;
  const [sessions, setSessions] = React.useState<Session[]>([]);
  const [activeId, setActiveId] = React.useState<string | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [loadError, setLoadError] = React.useState<string | null>(null);
  const [draft, setDraft] = React.useState("");
  const [pending, setPending] = React.useState<Mode | null>(null);
  const [tab, setTab] = React.useState<"chat" | "board">("chat");
  const [spinOff, setSpinOff] = React.useState<SpinOff | null>(null);
  const [listening, setListening] = React.useState(false);
  const abortRef = React.useRef<AbortController | null>(null);
  const dictationRef = React.useRef<Dictation | null>(null);
  const draftBeforeDictation = React.useRef("");
  const scrollRef = React.useRef<HTMLDivElement | null>(null);
  const taskCreation = useTaskCreation(workspaceId);

  const active = sessions.find((s) => s.id === activeId) ?? null;

  // Load (and, for a task panel, lazily create) sessions.
  React.useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setLoadError(null);
    (async () => {
      try {
        let list = await loadSessions(host.storage, scope, controller.signal);
        if (scope.single && list.length === 0 && taskId) {
          const task = await fetchTaskContext(host, taskId, controller.signal);
          const context = task ? `Kandev task: ${task.title}\n\n${task.description}`.trim() : "";
          const created = newSession(host.utils.generateUUID(), now(), context);
          if (task?.title) created.title = task.title;
          await saveSession(host.storage, scope, created, writerId);
          list = [created];
        }
        if (controller.signal.aborted) return;
        setSessions(list);
        setActiveId((current) => current ?? list[0]?.id ?? null);
      } catch (err) {
        if (!controller.signal.aborted) setLoadError(err instanceof Error ? err.message : String(err));
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    })();
    return () => controller.abort();
  }, [scope.scope, scope.scopeId]);

  // Cancel in-flight work on unmount.
  React.useEffect(
    () => () => {
      abortRef.current?.abort();
      dictationRef.current?.stop();
    },
    [],
  );

  // Keep the newest message in view.
  React.useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [active?.messages.length, pending]);

  const persist = (next: Session) => {
    setSessions((list) => sortSessions([next, ...list.filter((s) => s.id !== next.id)]));
    saveSession(host.storage, scope, next, writerId).catch((err) =>
      host.toast.error(`Could not save brainstorm: ${err instanceof Error ? err.message : String(err)}`),
    );
  };

  const createSession = () => {
    const session = newSession(host.utils.generateUUID(), now());
    persist(session);
    setActiveId(session.id);
    setDraft("");
    setTab("chat");
  };

  const removeSession = (session: Session) => {
    if (!globalThis.confirm(`Delete “${session.title}”? This cannot be undone.`)) return;
    setSessions((list) => list.filter((s) => s.id !== session.id));
    if (activeId === session.id) setActiveId(sessions.find((s) => s.id !== session.id)?.id ?? null);
    deleteSession(host.storage, scope, session.id, writerId).catch(() => host.toast.error("Could not delete brainstorm"));
  };

  const send = async (mode: Mode) => {
    if (!active || pending) return;
    dictationRef.current?.stop();
    const text = draft.trim();
    if (mode === "chat" && !text) return;
    let session = active;
    if (text) {
      const userMessage: Message = { id: host.utils.generateUUID(), role: "user", content: text, mode, createdAt: now() };
      session = appendMessage(session, userMessage);
      persist(session);
      setDraft("");
    }
    if (!session.messages.length) return;
    const controller = new AbortController();
    abortRef.current = controller;
    setPending(mode);
    try {
      const out = await runTurn(
        host.api.fetch,
        {
          mode,
          topic: session.title,
          context: session.context,
          messages: transcriptForAgent(session),
          board: session.board,
        },
        controller.signal,
      );
      const reply: Message = { id: host.utils.generateUUID(), role: "assistant", content: out.reply, mode, createdAt: now() };
      session = appendMessage(session, reply);
      if (out.board) {
        session = withBoard(session, out.board as Board, now());
        if (compact) setTab("board");
      }
      persist(session);
    } catch (err) {
      if (controller.signal.aborted) return;
      const message: Message = {
        id: host.utils.generateUUID(),
        role: "assistant",
        content: err instanceof Error ? err.message : String(err),
        mode,
        createdAt: now(),
        error: true,
      };
      persist(appendMessage(session, message));
    } finally {
      if (abortRef.current === controller) abortRef.current = null;
      setPending(null);
    }
  };

  const stop = () => {
    abortRef.current?.abort();
    abortRef.current = null;
    setPending(null);
  };

  const toggleDictation = () => {
    if (dictationRef.current) {
      dictationRef.current.stop();
      return;
    }
    draftBeforeDictation.current = draft ? `${draft.trimEnd()} ` : "";
    const dictation = startDictation({
      onText: (spoken) => setDraft(draftBeforeDictation.current + spoken),
      onError: (message) => host.toast.error(message),
      onEnd: () => {
        dictationRef.current = null;
        setListening(false);
      },
    });
    if (dictation) {
      dictationRef.current = dictation;
      setListening(true);
    }
  };

  const exportMarkdown = async () => {
    if (!active) return;
    const md = toMarkdown(active);
    try {
      await navigator.clipboard.writeText(md);
      host.toast.success("Copied brainstorm as Markdown");
    } catch {
      const blob = new Blob([md], { type: "text/markdown" });
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = `${active.title.replace(/[^\w-]+/g, "-").slice(0, 60) || "brainstorm"}.md`;
      a.click();
      URL.revokeObjectURL(a.href);
    }
  };

  const openSpinOff = (item: { title: string; detail: string }) => {
    if (!active) return;
    const draftTask = taskDraft(active, item);
    if (!taskCreation) {
      navigator.clipboard?.writeText(`${draftTask.title}\n\n${draftTask.description}`).catch(() => undefined);
      host.toast.info("Task creation isn't available here yet; copied the task text instead.");
      return;
    }
    setSpinOff({ ...draftTask, key: item.title });
  };

  const rename = (title: string) => {
    if (!active) return;
    const clean = title.trim();
    if (!clean || clean === active.title) return;
    persist({ ...active, title: clean, updatedAt: now() });
  };

  if (loading) {
    return (
      <div className="kbs-empty">
        <ui.Spinner />
      </div>
    );
  }
  if (loadError) {
    return <div className="kbs-empty kbs-error">Could not load brainstorms: {loadError}</div>;
  }

  // Narrow page: a one-line session switcher instead of the sessions column.
  const sessionBar = !scope.single && compact && (
    <div className="kbs-session-bar">
      <select
        className="kbs-session-select"
        value={activeId ?? ""}
        onChange={(e: Event) => setActiveId((e.target as HTMLSelectElement).value || null)}
        aria-label="Brainstorm session"
      >
        {sessions.length === 0 && <option value="">No brainstorms yet</option>}
        {sessions.map((s) => (
          <option key={s.id} value={s.id}>
            {s.title}
          </option>
        ))}
      </select>
      <ui.Button size="sm" variant="outline" onClick={createSession}>
        + New
      </ui.Button>
    </div>
  );

  const sessionList = sessionBar || (!scope.single && (
    <aside className="kbs-sessions">
      <ui.Button size="sm" onClick={createSession} className="kbs-new">
        + New brainstorm
      </ui.Button>
      <div className="kbs-session-list">
        {sessions.length === 0 && <p className="kbs-muted kbs-pad">No brainstorms yet.</p>}
        {sessions.map((s) => (
          <div
            key={s.id}
            className={`kbs-session${s.id === activeId ? " kbs-session--active" : ""}`}
            onClick={() => {
              setActiveId(s.id);
              setTab("chat");
            }}
            role="button"
            tabIndex={0}
          >
            <span className="kbs-session-title">{s.title}</span>
            <span className="kbs-session-meta">
              {s.messages.length} msgs · {host.utils.formatRelativeTime(s.updatedAt)}
            </span>
            <button
              className="kbs-icon-btn kbs-session-delete"
              title="Delete brainstorm"
              onClick={(e: MouseEvent) => {
                e.stopPropagation();
                removeSession(s);
              }}
            >
              ×
            </button>
          </div>
        ))}
      </div>
    </aside>
  ));

  if (!active) {
    return (
      <div className={`kbs-root${compact ? " kbs-root--compact" : ""}`}>
        {sessionList}
        <div className="kbs-empty kbs-welcome">
          <h3>Think out loud with an agent</h3>
          <p className="kbs-muted">
            Talk loosely, let the partner ask one sharp question at a time, then Converge to turn the conversation into
            stages, decisions, and next actions you can launch as Kandev tasks.
          </p>
          <ui.Button onClick={createSession}>Start a brainstorm</ui.Button>
        </div>
      </div>
    );
  }

  const chat = (
    <section className="kbs-chat">
      {!compact && (
        <header className="kbs-chat-header">
          <input
            className="kbs-title-input"
            key={active.id}
            defaultValue={active.title}
            onBlur={(e: FocusEvent) => rename((e.target as HTMLInputElement).value)}
            onKeyDown={(e: KeyboardEvent) => {
              if (e.key === "Enter") (e.target as HTMLInputElement).blur();
            }}
            aria-label="Brainstorm title"
          />
          <ui.Button size="sm" variant="ghost" onClick={exportMarkdown} title="Copy as Markdown">
            Export
          </ui.Button>
        </header>
      )}
      <div className="kbs-messages" ref={scrollRef}>
        {active.messages.length === 0 && (
          <div className="kbs-hint">
            <p>
              <strong>Start anywhere.</strong> Dump the messy version of the problem — the partner will reflect it back,
              build on it, and ask at most one question.
            </p>
            <p className="kbs-muted">Enter sends · Shift+Enter for a new line{dictationSupported() ? " · 🎙 to dictate" : ""}</p>
          </div>
        )}
        {active.messages.map((m) => (
          <div key={m.id} className={`kbs-msg kbs-msg--${m.role}${m.error ? " kbs-msg--error" : ""}`}>
            {m.role === "assistant" && m.mode && m.mode !== "chat" && (
              <span className={`kbs-mode-tag kbs-mode-tag--${m.mode}`}>{m.mode}</span>
            )}
            {m.role === "assistant" && !m.error ? <Markdown text={m.content} /> : <p className="kbs-plain">{m.content}</p>}
          </div>
        ))}
        {pending && (
          <div className="kbs-msg kbs-msg--assistant kbs-msg--pending">
            <ui.Spinner />
            <span className="kbs-muted">
              {pending === "converge" ? "Converging into the board…" : pending === "chat" ? "Thinking…" : `${pending}…`}
            </span>
            <button className="kbs-link" onClick={stop}>
              Stop
            </button>
          </div>
        )}
      </div>
      <footer className="kbs-composer">
        <ui.Textarea
          value={draft}
          placeholder={listening ? "Listening…" : "Think out loud…"}
          onChange={(e: Event) => setDraft((e.target as HTMLTextAreaElement).value)}
          onKeyDown={(e: KeyboardEvent) => {
            if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
              e.preventDefault();
              void send("chat");
            }
          }}
          rows={compact ? 2 : 3}
          className="kbs-textarea"
        />
        <div className="kbs-composer-row">
          <div className="kbs-modes">
            {MODES.map((m) => (
              <ui.Button
                key={m.mode}
                size="sm"
                variant="outline"
                title={m.hint}
                disabled={!!pending || (!active.messages.length && !draft.trim())}
                onClick={() => void send(m.mode)}
              >
                {m.label}
              </ui.Button>
            ))}
          </div>
          <div className="kbs-send">
            {dictationSupported() && (
              <ui.Button
                size="sm"
                variant={listening ? "destructive" : "ghost"}
                onClick={toggleDictation}
                title={listening ? "Stop dictation" : "Dictate"}
                aria-pressed={listening}
              >
                {listening ? "■ Stop" : "🎙"}
              </ui.Button>
            )}
            <ui.Button size="sm" disabled={!!pending || !draft.trim()} onClick={() => void send("chat")}>
              Send
            </ui.Button>
          </div>
        </div>
      </footer>
    </section>
  );

  const board = (
    <BoardView
      session={active}
      onSpinOff={openSpinOff}
      onConverge={() => void send("converge")}
      busy={!!pending}
      onExport={compact ? exportMarkdown : undefined}
    />
  );

  return (
    <div className={`kbs-root${compact ? " kbs-root--compact" : ""}`}>
      {sessionList}
      {compact ? (
        <div className="kbs-compact">
          <div className="kbs-tabs" role="tablist">
            <button role="tab" aria-selected={tab === "chat"} className={tab === "chat" ? "kbs-tab kbs-tab--on" : "kbs-tab"} onClick={() => setTab("chat")}>
              Chat
            </button>
            <button role="tab" aria-selected={tab === "board"} className={tab === "board" ? "kbs-tab kbs-tab--on" : "kbs-tab"} onClick={() => setTab("board")}>
              Board{active.board ? ` · ${active.board.nextActions.length}` : ""}
            </button>
          </div>
          {tab === "chat" ? chat : board}
        </div>
      ) : (
        <Fragment>
          {chat}
          {board}
        </Fragment>
      )}
      {spinOff && taskCreation && (
        <ui.TaskCreateDialog
          open
          onOpenChange={(open: boolean) => {
            if (!open) setSpinOff(null);
          }}
          mode="create"
          workspaceId={taskCreation.workspaceId}
          workflowId={taskCreation.workflowId}
          defaultStepId={taskCreation.defaultStepId}
          steps={taskCreation.steps}
          parentTaskId={taskId}
          initialValues={{ title: spinOff.title, description: spinOff.description }}
          onSuccess={() => {
            persist(markSpunOff(active, spinOff.key));
            setSpinOff(null);
            host.toast.success("Task created from brainstorm");
          }}
        />
      )}
    </div>
  );
}

function BoardView(props: {
  session: Session;
  busy: boolean;
  onSpinOff(item: { title: string; detail: string }): void;
  onConverge(): void;
  onExport?: () => void;
}) {
  const { session, busy, onSpinOff, onConverge, onExport } = props;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const ui = host.ui as any;
  const board = session.board;
  const spun = new Set(session.spunOff);

  const spinButton = (item: { title: string; detail: string }) =>
    spun.has(item.title) ? (
      <span className="kbs-done" title="A task was created from this item">
        ✓ task
      </span>
    ) : (
      <button className="kbs-link" onClick={() => onSpinOff(item)} title="Create a Kandev task from this">
        Create task
      </button>
    );

  const list = (title: string, items: string[]) =>
    items.length > 0 && (
      <div className="kbs-board-section">
        <h5>{title}</h5>
        <ul>
          {items.map((item, i) => (
            <li key={i}>{item}</li>
          ))}
        </ul>
      </div>
    );

  return (
    <aside className="kbs-board">
      <header className="kbs-board-header">
        <h4>Board</h4>
        <div className="kbs-board-actions">
          {onExport && (
            <ui.Button size="sm" variant="ghost" onClick={onExport}>
              Export
            </ui.Button>
          )}
          <ui.Button size="sm" variant="outline" disabled={busy || session.messages.length === 0} onClick={onConverge}>
            {board ? "Refresh" : "Converge"}
          </ui.Button>
        </div>
      </header>
      {!board ? (
        <p className="kbs-muted kbs-pad">
          Nothing here yet. After a few turns, press <strong>Converge</strong> and the partner will distil the conversation
          into a goal, stages, ideas, decisions, and next actions.
        </p>
      ) : (
        <div className="kbs-board-body">
          {session.boardUpdatedAt && (
            <p className="kbs-muted kbs-small">Updated {host.utils.formatRelativeTime(session.boardUpdatedAt)}</p>
          )}
          {board.goal && (
            <div className="kbs-board-section">
              <h5>Goal</h5>
              <p>{board.goal}</p>
            </div>
          )}
          {board.stages.length > 0 && (
            <div className="kbs-board-section">
              <h5>Stages</h5>
              <ol className="kbs-stages">
                {board.stages.map((s, i) => (
                  <li key={i}>
                    <strong>{s.title}</strong>
                    <span>{s.summary}</span>
                  </li>
                ))}
              </ol>
            </div>
          )}
          {board.nextActions.length > 0 && (
            <div className="kbs-board-section">
              <h5>Next actions</h5>
              <ul className="kbs-items">
                {board.nextActions.map((a, i) => (
                  <li key={i} className="kbs-item">
                    <div>
                      <strong>{a.title}</strong>
                      {a.detail && <span className="kbs-muted">{a.detail}</span>}
                    </div>
                    {spinButton(a)}
                  </li>
                ))}
              </ul>
            </div>
          )}
          {board.ideas.length > 0 && (
            <div className="kbs-board-section">
              <h5>Ideas</h5>
              <ul className="kbs-items">
                {board.ideas.map((idea, i) => (
                  <li key={i} className="kbs-item">
                    <div>
                      <span className={`kbs-status kbs-status--${idea.status}`}>{idea.status}</span>
                      <strong>{idea.title}</strong>
                      {idea.detail && <span className="kbs-muted">{idea.detail}</span>}
                    </div>
                    {spinButton(idea)}
                  </li>
                ))}
              </ul>
            </div>
          )}
          {list("Decisions", board.decisions)}
          {list("Open questions", board.questions)}
          {list("Risks", board.risks)}
        </div>
      )}
    </aside>
  );
}
