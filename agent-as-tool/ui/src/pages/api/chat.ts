import type { APIRoute } from "astro";
import { createSession, runMessage, type AdkEvent } from "~/lib/adk";

export const prerender = false;

type ChatBody = {
  sessionId?: string;
  message?: string;
};

type RenderedEvent =
  | { kind: "text"; author: string; text: string }
  | { kind: "call"; author: string; name: string }
  | { kind: "result"; author: string; name: string }
  | { kind: "error"; message: string };

function render(events: AdkEvent[]): RenderedEvent[] {
  const out: RenderedEvent[] = [];
  for (const ev of events) {
    if (ev.errorMessage) {
      out.push({ kind: "error", message: ev.errorMessage });
      continue;
    }
    const author = ev.author ?? "agent";
    for (const part of ev.content?.parts ?? []) {
      if (part.text) {
        out.push({ kind: "text", author, text: part.text });
      } else if (part.functionCall) {
        out.push({ kind: "call", author, name: part.functionCall.name });
      } else if (part.functionResponse) {
        out.push({ kind: "result", author, name: part.functionResponse.name });
      }
    }
  }
  return out;
}

export const POST: APIRoute = async ({ request }) => {
  let body: ChatBody;
  try {
    body = (await request.json()) as ChatBody;
  } catch {
    return new Response(JSON.stringify({ error: "invalid JSON" }), {
      status: 400,
      headers: { "Content-Type": "application/json" },
    });
  }

  const message = body.message?.trim();
  if (!message) {
    return new Response(JSON.stringify({ error: "message is required" }), {
      status: 400,
      headers: { "Content-Type": "application/json" },
    });
  }

  try {
    const sessionId = body.sessionId ?? (await createSession());
    const events = await runMessage(sessionId, message);
    return new Response(
      JSON.stringify({ sessionId, events: render(events) }),
      { headers: { "Content-Type": "application/json" } },
    );
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return new Response(JSON.stringify({ error: message }), {
      status: 500,
      headers: { "Content-Type": "application/json" },
    });
  }
};
