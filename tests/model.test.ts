import { describe, expect, it } from "vitest";
import {
  UNTITLED,
  appendMessage,
  deriveTitle,
  markSpunOff,
  newSession,
  parseSession,
  sortSessions,
  taskDraft,
  toMarkdown,
  transcriptForAgent,
  withBoard,
  type Board,
} from "../ui/src/model";
import { parseBlocks } from "../ui/src/markdown";
import { keyFor } from "../ui/src/store";

const T0 = "2026-10-31T10:00:00.000Z";
const T1 = "2026-10-31T11:00:00.000Z";

const board: Board = {
  goal: "Cut backlog noise",
  stages: [{ title: "Stage 1", summary: "Reports" }],
  ideas: [{ title: "Weekly digest", detail: "Post a summary", status: "chosen" }],
  questions: ["Who owns triage?"],
  decisions: ["Never delete, only cancel with a reason"],
  risks: ["Over-eager closing"],
  nextActions: [{ title: "Draft a triage report", detail: "Done when shared" }],
};

describe("deriveTitle", () => {
  it("uses the first sentence", () => {
    expect(deriveTitle("Our backlog is noisy. Lots more here.")).toBe("Our backlog is noisy.");
  });
  it("truncates on a word boundary", () => {
    const title = deriveTitle("word ".repeat(40));
    expect(title.length).toBeLessThanOrEqual(61);
    expect(title.endsWith("…")).toBe(true);
  });
  it("falls back for empty text", () => {
    expect(deriveTitle("   ")).toBe(UNTITLED);
  });
});

describe("session model", () => {
  it("titles a session from its first user message only", () => {
    let s = newSession("s1", T0);
    s = appendMessage(s, { id: "1", role: "user", content: "Backlog cleanup ideas", createdAt: T1 });
    s = appendMessage(s, { id: "2", role: "user", content: "Second thought", createdAt: T1 });
    expect(s.title).toBe("Backlog cleanup ideas");
    expect(s.updatedAt).toBe(T1);
  });

  it("excludes error messages from the agent transcript", () => {
    let s = newSession("s1", T0);
    s = appendMessage(s, { id: "1", role: "user", content: "hi", createdAt: T0 });
    s = appendMessage(s, { id: "2", role: "assistant", content: "boom", createdAt: T0, error: true });
    expect(transcriptForAgent(s)).toEqual([{ role: "user", content: "hi" }]);
  });

  it("marks spin-offs idempotently", () => {
    const s = markSpunOff(markSpunOff(newSession("s1", T0), "A"), "A");
    expect(s.spunOff).toEqual(["A"]);
  });

  it("builds a task draft with brainstorm context", () => {
    const s = withBoard({ ...newSession("s1", T0), title: "Backlog" }, board, T1);
    const draft = taskDraft(s, board.nextActions[0]);
    expect(draft.title).toBe("Draft a triage report");
    expect(draft.description).toContain("Done when shared");
    expect(draft.description).toContain("“Backlog”");
    expect(draft.description).toContain("- Never delete, only cancel with a reason");
  });

  it("exports Markdown with board and transcript", () => {
    let s = withBoard({ ...newSession("s1", T0), title: "Backlog" }, board, T1);
    s = appendMessage(s, { id: "1", role: "user", content: "hello", createdAt: T1 });
    const md = toMarkdown(s);
    expect(md).toContain("# Backlog");
    expect(md).toContain("## Next actions");
    expect(md).toContain("- [ ] **Draft a triage report** — Done when shared");
    expect(md).toContain("**You:** hello");
  });

  it("parses only well-formed stored sessions", () => {
    expect(parseSession(null)).toBeNull();
    expect(parseSession({ version: 2, id: "x", messages: [] })).toBeNull();
    const parsed = parseSession({ version: 1, id: "x", messages: [], updatedAt: T1 });
    expect(parsed?.spunOff).toEqual([]);
    expect(parsed?.title).toBe(UNTITLED);
  });

  it("sorts newest first", () => {
    expect(sortSessions([newSession("a", T0), newSession("b", T1)]).map((s) => s.id)).toEqual(["b", "a"]);
  });
});

describe("storage keys", () => {
  it("uses one key for task panels and per-session keys for workspaces", () => {
    expect(keyFor({ scope: "task", scopeId: "t", single: true }, "abc")).toBe("session");
    expect(keyFor({ scope: "workspace", scopeId: "w", single: false }, "abc")).toBe("session:abc");
  });
});

describe("markdown blocks", () => {
  it("groups headings, lists and paragraphs", () => {
    expect(parseBlocks("### Ideas\n- one\n- two\n\nA para\ncontinues\n1. first\n2. second")).toEqual([
      { kind: "heading", text: "Ideas" },
      { kind: "ul", items: ["one", "two"] },
      { kind: "p", text: "A para continues" },
      { kind: "ol", items: ["first", "second"] },
    ]);
  });
});
