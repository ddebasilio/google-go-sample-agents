import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

export const ADK_URL =
  process.env.ADK_URL ??
  "https://adk-default-service-name-841572325613.europe-west3.run.app";
export const ADK_APP = process.env.ADK_APP ?? "agent";
export const ADK_USER = process.env.ADK_USER ?? "u1";

let cachedToken: { value: string; expires: number } | null = null;

async function mintToken(): Promise<string> {
  const { stdout } = await execFileAsync("gcloud", [
    "auth",
    "print-identity-token",
  ]);
  return stdout.trim();
}

export async function getToken(force = false): Promise<string> {
  const now = Date.now();
  if (!force && cachedToken && cachedToken.expires > now) {
    return cachedToken.value;
  }
  const value = await mintToken();
  cachedToken = { value, expires: now + 50 * 60 * 1000 };
  return value;
}

async function authedFetch(
  path: string,
  init: RequestInit = {},
): Promise<Response> {
  const doFetch = async (token: string) =>
    fetch(`${ADK_URL}${path}`, {
      ...init,
      headers: {
        ...(init.headers ?? {}),
        Authorization: `Bearer ${token}`,
      },
    });

  let res = await doFetch(await getToken());
  if (res.status === 401 || res.status === 403) {
    res = await doFetch(await getToken(true));
  }
  return res;
}

export async function createSession(): Promise<string> {
  const sessionId = `s${Date.now()}`;
  const res = await authedFetch(
    `/apps/${ADK_APP}/users/${ADK_USER}/sessions/${sessionId}`,
    { method: "POST" },
  );
  if (!res.ok) {
    throw new Error(`createSession failed: HTTP ${res.status}`);
  }
  return sessionId;
}

export type AdkEvent = {
  author?: string;
  errorMessage?: string;
  content?: {
    parts?: Array<{
      text?: string;
      functionCall?: { name: string; args?: unknown };
      functionResponse?: { name: string; response?: unknown };
    }>;
  };
};

export async function runMessage(
  sessionId: string,
  message: string,
): Promise<AdkEvent[]> {
  const res = await authedFetch("/run", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      appName: ADK_APP,
      userId: ADK_USER,
      sessionId,
      newMessage: { role: "user", parts: [{ text: message }] },
    }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`run failed: HTTP ${res.status} — ${text}`);
  }
  return (await res.json()) as AdkEvent[];
}

export async function* streamMessage(
  sessionId: string,
  message: string,
): AsyncGenerator<AdkEvent> {
  const res = await authedFetch("/run_sse", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      appName: ADK_APP,
      userId: ADK_USER,
      sessionId,
      newMessage: { role: "user", parts: [{ text: message }] },
    }),
  });
  if (!res.ok || !res.body) {
    const text = res.body ? await res.text() : "";
    throw new Error(`run_sse failed: HTTP ${res.status} — ${text}`);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    let sep: number;
    while ((sep = buffer.indexOf("\n\n")) !== -1) {
      const frame = buffer.slice(0, sep);
      buffer = buffer.slice(sep + 2);
      const dataLines = frame
        .split("\n")
        .filter((l) => l.startsWith("data:"))
        .map((l) => l.slice(5).replace(/^ /, ""));
      if (dataLines.length === 0) continue;
      const json = dataLines.join("\n");
      try {
        yield JSON.parse(json) as AdkEvent;
      } catch {
        // ignore malformed frame
      }
    }
  }
}
