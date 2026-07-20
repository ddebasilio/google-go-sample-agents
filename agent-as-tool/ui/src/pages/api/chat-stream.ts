import type { APIRoute } from "astro";
import {
  createSession,
  streamMessage,
  type AdkEvent,
} from "~/lib/adk";

export const prerender = false;

type ChatBody = {
  sessionId?: string;
  message?: string;
};

type Rendered =
  | { kind: "text"; author: string; text: string }
  | { kind: "call"; author: string; name: string; args: unknown }
  | { kind: "result"; author: string; name: string; response: unknown }
  | { kind: "error"; message: string };

function renderEvent(ev: AdkEvent): Rendered[] {
  if (ev.errorMessage) return [{ kind: "error", message: ev.errorMessage }];
  const author = ev.author ?? "agent";
  const out: Rendered[] = [];
  for (const part of ev.content?.parts ?? []) {
    if (part.text) {
      out.push({ kind: "text", author, text: part.text });
    } else if (part.functionCall) {
      out.push({
        kind: "call",
        author,
        name: part.functionCall.name,
        args: part.functionCall.args ?? {},
      });
    } else if (part.functionResponse) {
      out.push({
        kind: "result",
        author,
        name: part.functionResponse.name,
        response: part.functionResponse.response ?? null,
      });
    }
  }
  return out;
}

function sse(event: string, data: unknown): Uint8Array {
  return new TextEncoder().encode(
    `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`,
  );
}

export const POST: APIRoute = async ({ request }) => {
  let body: ChatBody;
  try {
    body = (await request.json()) as ChatBody;
  } catch {
    return new Response(JSON.stringify({ error: "invalid JSON" }), {
      status: 400,
    });
  }
  const message = body.message?.trim();
  if (!message) {
    return new Response(JSON.stringify({ error: "message required" }), {
      status: 400,
    });
  }

  const stream = new ReadableStream({
    async start(controller) {
      try {
        const sessionId = body.sessionId ?? (await createSession());
        controller.enqueue(sse("session", { sessionId }));
        for await (const ev of streamMessage(sessionId, message)) {
          for (const r of renderEvent(ev)) {
            controller.enqueue(sse(r.kind, r));
          }
        }
        controller.enqueue(sse("done", {}));
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        controller.enqueue(sse("error", { message: msg }));
      } finally {
        controller.close();
      }
    },
  });

  return new Response(stream, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache, no-transform",
      Connection: "keep-alive",
      "X-Accel-Buffering": "no",
    },
  });
};
