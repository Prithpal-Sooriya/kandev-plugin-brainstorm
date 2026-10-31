// Persistence via host.storage (per-user, authenticated, kept in Kandev's own
// database). Workspace sessions live under ("workspace", id) with one key
// per session; a task panel keeps exactly one session under ("task", id).
import type { PluginHostApi, PluginStorageScope } from "@kandev/plugin-sdk";
import { parseSession, sortSessions, type Session } from "./model";

export interface SessionScope {
  scope: PluginStorageScope;
  scopeId: string;
  /** Task panels hold a single session; the workspace page holds many. */
  single: boolean;
}

const PREFIX = "session:";

export function keyFor(scope: SessionScope, sessionId: string): string {
  return scope.single ? "session" : `${PREFIX}${sessionId}`;
}

export async function loadSessions(
  storage: PluginHostApi["storage"],
  scope: SessionScope,
  signal?: AbortSignal,
): Promise<Session[]> {
  const entries = await storage.list(scope.scope, scope.scopeId, { signal });
  const sessions: Session[] = [];
  for (const entry of entries) {
    if (scope.single ? entry.key !== "session" : !entry.key.startsWith(PREFIX)) continue;
    const session = parseSession(entry.value);
    if (session) sessions.push(session);
  }
  return sortSessions(sessions);
}

export async function saveSession(
  storage: PluginHostApi["storage"],
  scope: SessionScope,
  session: Session,
  writerId: string,
): Promise<void> {
  await storage.set(scope.scope, scope.scopeId, keyFor(scope, session.id), session, { writerId });
}

export async function deleteSession(
  storage: PluginHostApi["storage"],
  scope: SessionScope,
  sessionId: string,
  writerId: string,
): Promise<void> {
  await storage.delete(scope.scope, scope.scopeId, keyFor(scope, sessionId), { writerId });
}
