import { LlmAgent, GOOGLE_SEARCH, URL_CONTEXT, setLogger } from "@google/adk";

// setLogger(null);

export const rootAgent = new LlmAgent({
  name: "research_assistant",
  model: "gemini-3-flash-preview",
  description: "Searches the web and reads pages to answer questions.",
  instruction: `You are a research assistant.

Process:
1. Call google_search with a focused query.
2. Pick the most relevant URL from the snippets.
3. Use url_context to read that page in full.
4. Decide whether you have enough. If not, fetch another URL or run a sharper search.
5. When you have enough, write a concise answer with citations.

Rules:
- Do not answer from memory. Always search first.
- Prefer primary sources over aggregators.
- Stop calling tools as soon as the answer is solid.`,
  tools: [GOOGLE_SEARCH, URL_CONTEXT],
});
