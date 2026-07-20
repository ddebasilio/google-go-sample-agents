import type { APIRoute } from "astro";
import { createSession } from "~/lib/adk";

export const prerender = false;

export const POST: APIRoute = async () => {
  try {
    const sessionId = await createSession();
    return new Response(JSON.stringify({ sessionId }), {
      headers: { "Content-Type": "application/json" },
    });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return new Response(JSON.stringify({ error: message }), {
      status: 500,
      headers: { "Content-Type": "application/json" },
    });
  }
};
