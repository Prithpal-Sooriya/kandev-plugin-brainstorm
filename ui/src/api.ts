// Browser side of the "turn" webhook: start a job, then poll until it settles.
import type { PluginHostApi } from "@kandev/plugin-sdk";
import type { Board, Mode } from "./model";

export interface TurnInput {
  mode: Mode;
  topic: string;
  context: string;
  messages: Array<{ role: "user" | "assistant"; content: string }>;
  board: Board | null;
}

export interface TurnOutput {
  reply: string;
  board?: Board;
}

type Fetch = PluginHostApi["api"]["fetch"];

const POLL_MS = 1200;

async function readError(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string };
    if (body?.error) return body.error;
  } catch {
    // fall through
  }
  return `Request failed (${res.status})`;
}

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(resolve, ms);
    signal.addEventListener(
      "abort",
      () => {
        clearTimeout(timer);
        reject(signal.reason ?? new DOMException("Aborted", "AbortError"));
      },
      { once: true },
    );
  });
}

export async function runTurn(fetch: Fetch, input: TurnInput, signal: AbortSignal): Promise<TurnOutput> {
  const start = await fetch("webhooks/turn", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ...input, board: input.board ?? undefined }),
    signal,
  });
  if (!start.ok) throw new Error(await readError(start));
  const { id } = (await start.json()) as { id: string };
  for (;;) {
    await sleep(POLL_MS, signal);
    const res = await fetch(`webhooks/turn?id=${encodeURIComponent(id)}`, { method: "GET", signal });
    if (!res.ok) throw new Error(await readError(res));
    const job = (await res.json()) as { status: string; result?: TurnOutput; error?: string };
    if (job.status === "done" && job.result) return job.result;
    if (job.status === "error") throw new Error(job.error || "The agent turn failed");
  }
}

export interface TaskContext {
  title: string;
  description: string;
  workspaceId: string;
}

export async function fetchTaskContext(
  host: PluginHostApi,
  taskId: string,
  signal: AbortSignal,
): Promise<TaskContext | null> {
  try {
    return await host.api.invokeAction<TaskContext>("task.context", { taskId }, { signal });
  } catch {
    return null;
  }
}
