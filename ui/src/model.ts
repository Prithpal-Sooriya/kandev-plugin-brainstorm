// Pure data model for brainstorm sessions. Nothing here touches the host, so
// it is unit-tested directly (tests/model.test.ts).

export type Mode = "chat" | "diverge" | "challenge" | "converge";

export interface Message {
  id: string;
  role: "user" | "assistant";
  content: string;
  mode?: Mode;
  createdAt: string;
  error?: boolean;
}

export interface Stage {
  title: string;
  summary: string;
}

export interface Idea {
  title: string;
  detail: string;
  status: "explore" | "parked" | "chosen";
}

export interface NextAction {
  title: string;
  detail: string;
}

export interface Board {
  goal: string;
  stages: Stage[];
  ideas: Idea[];
  questions: string[];
  decisions: string[];
  risks: string[];
  nextActions: NextAction[];
}

export interface Session {
  version: 1;
  id: string;
  title: string;
  /** Extra background the agent always sees (e.g. the attached task). */
  context: string;
  messages: Message[];
  board: Board | null;
  boardUpdatedAt: string | null;
  /** Titles of board items already turned into Kandev tasks. */
  spunOff: string[];
  createdAt: string;
  updatedAt: string;
}

export const MODES: ReadonlyArray<{ mode: Mode; label: string; hint: string }> = [
  { mode: "diverge", label: "Diverge", hint: "Generate more, different ideas" },
  { mode: "challenge", label: "Challenge", hint: "Find risks and blind spots" },
  { mode: "converge", label: "Converge", hint: "Summarize everything into the board" },
];

export const UNTITLED = "Untitled brainstorm";

export function newSession(id: string, now: string, context = ""): Session {
  return {
    version: 1,
    id,
    title: UNTITLED,
    context,
    messages: [],
    board: null,
    boardUpdatedAt: null,
    spunOff: [],
    createdAt: now,
    updatedAt: now,
  };
}

/** Derive a short title from the first user message (first sentence, ≤ 60 chars). */
export function deriveTitle(text: string): string {
  const clean = text.replace(/\s+/g, " ").trim();
  if (!clean) return UNTITLED;
  const sentence = clean.split(/(?<=[.!?])\s/)[0] ?? clean;
  if (sentence.length <= 60) return sentence;
  const cut = sentence.slice(0, 60);
  const lastSpace = cut.lastIndexOf(" ");
  return `${(lastSpace > 30 ? cut.slice(0, lastSpace) : cut).trim()}…`;
}

export function appendMessage(session: Session, message: Message): Session {
  const isFirstUser = message.role === "user" && !session.messages.some((m) => m.role === "user");
  return {
    ...session,
    title: isFirstUser && session.title === UNTITLED ? deriveTitle(message.content) : session.title,
    messages: [...session.messages, message],
    updatedAt: message.createdAt,
  };
}

/** Messages the agent should see: errors are UI-only. */
export function transcriptForAgent(session: Session): Array<{ role: Message["role"]; content: string }> {
  return session.messages.filter((m) => !m.error).map((m) => ({ role: m.role, content: m.content }));
}

export function withBoard(session: Session, board: Board, now: string): Session {
  return { ...session, board, boardUpdatedAt: now, updatedAt: now };
}

export function markSpunOff(session: Session, title: string): Session {
  if (session.spunOff.includes(title)) return session;
  return { ...session, spunOff: [...session.spunOff, title] };
}

/** Prefill for Kandev's create-task dialog from a board item. */
export function taskDraft(
  session: Session,
  item: { title: string; detail: string },
): { title: string; description: string } {
  const lines: string[] = [];
  if (item.detail.trim()) lines.push(item.detail.trim(), "");
  lines.push(`_From the brainstorm “${session.title}”._`);
  const board = session.board;
  if (board?.goal) lines.push("", `**Goal:** ${board.goal}`);
  if (board && board.decisions.length) {
    lines.push("", "**Decisions so far:**", ...board.decisions.map((d) => `- ${d}`));
  }
  if (board && board.risks.length) {
    lines.push("", "**Known risks:**", ...board.risks.map((r) => `- ${r}`));
  }
  return { title: item.title.trim().slice(0, 200), description: lines.join("\n") };
}

export function toMarkdown(session: Session): string {
  const out: string[] = [`# ${session.title}`, ""];
  const board = session.board;
  if (board) {
    if (board.goal) out.push(`**Goal:** ${board.goal}`, "");
    const section = (title: string, items: string[]) => {
      if (!items.length) return;
      out.push(`## ${title}`, "", ...items, "");
    };
    section(
      "Stages",
      board.stages.map((s) => `- **${s.title}** — ${s.summary}`),
    );
    section(
      "Ideas",
      board.ideas.map((i) => `- [${i.status}] **${i.title}**${i.detail ? ` — ${i.detail}` : ""}`),
    );
    section(
      "Decisions",
      board.decisions.map((d) => `- ${d}`),
    );
    section(
      "Open questions",
      board.questions.map((q) => `- ${q}`),
    );
    section(
      "Risks",
      board.risks.map((r) => `- ${r}`),
    );
    section(
      "Next actions",
      board.nextActions.map((a) => `- [ ] **${a.title}**${a.detail ? ` — ${a.detail}` : ""}`),
    );
  }
  const transcript = session.messages.filter((m) => !m.error);
  if (transcript.length) {
    out.push("## Transcript", "");
    for (const m of transcript) {
      out.push(`**${m.role === "user" ? "You" : "Partner"}:** ${m.content}`, "");
    }
  }
  return `${out.join("\n").trim()}\n`;
}

/** Accept only well-formed sessions from storage (defends against old/garbled values). */
export function parseSession(value: unknown): Session | null {
  if (!value || typeof value !== "object") return null;
  const v = value as Partial<Session>;
  if (v.version !== 1 || typeof v.id !== "string" || !Array.isArray(v.messages)) return null;
  return {
    ...newSession(v.id, typeof v.createdAt === "string" ? v.createdAt : new Date(0).toISOString()),
    ...v,
    spunOff: Array.isArray(v.spunOff) ? v.spunOff : [],
  } as Session;
}

export function sortSessions(sessions: Session[]): Session[] {
  return [...sessions].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
}
