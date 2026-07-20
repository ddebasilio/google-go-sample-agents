import {
  FunctionTool,
  LlmAgent,
  SequentialAgent,
  setLogger,
} from "@google/adk";
import { z } from "zod";

setLogger(null);

// --- Tools ---

const getRandomQuote = new FunctionTool({
  name: "get_random_quote",
  description: "Fetch a random inspirational quote with its author",
  parameters: z.object({}),
  execute: async () => {
    const res = await fetch("https://zenquotes.io/api/random");
    const data = (await res.json()) as any;
    return { quote: data[0].q, author: data[0].a };
  },
});

const searchWikipedia = new FunctionTool({
  name: "search_wikipedia",
  description: "Search Wikipedia for biographical information about a person",
  parameters: z.object({
    query: z.string().describe("The person to search for on Wikipedia"),
  }),
  execute: async ({ query }) => {
    const searchUrl = `https://en.wikipedia.org/w/api.php?action=query&list=search&srsearch=${encodeURIComponent(query)}&format=json`;
    const searchRes = await fetch(searchUrl);
    const searchData = (await searchRes.json()) as any;
    if (!searchData.query?.search?.length)
      return { result: "No results found" };
    const title = searchData.query.search[0].title;
    const summaryRes = await fetch(
      `https://en.wikipedia.org/api/rest_v1/page/summary/${encodeURIComponent(title)}`,
    );
    const summaryData = (await summaryRes.json()) as any;
    return { result: summaryData.extract || "No information found" };
  },
});

// --- Schemas ---

const QuoteSchema = z.object({
  quote: z.string().describe("The text of the quote"),
  author: z.string().describe("The author of the quote"),
});

// --- Sub-Agents ---

const quoteFetcherAgent = new LlmAgent({
  name: "QuoteFetcherAgent",
  model: "gemini-3-flash-preview",
  description: "Fetches a random inspirational quote using the quote tool.",
  instruction: `You fetch random quotes using the provided tool. Call the get_random_quote tool, then report the quote text and its author. The structured output schema enforces the response shape — fill in the quote and author fields from the tool result.`,
  tools: [getRandomQuote],
  outputSchema: QuoteSchema,
  outputKey: "quote_result",
});

const wikipediaResearcherAgent = new LlmAgent({
  name: "WikipediaResearcherAgent",
  model: "gemini-3-flash-preview",
  description: "Researches a person on Wikipedia and returns a concise bio.",
  instruction: `You research people on Wikipedia. The previous agent fetched a quote — here is its structured output (a JSON object with "quote" and "author" fields):

{quote_result}

Take the "author" field and search for that person using the Wikipedia tool. Return a concise 2-3 sentence bio about who they are and why they are notable.`,
  tools: [searchWikipedia],
  outputKey: "author_bio",
});

const inspirationCardAgent = new LlmAgent({
  name: "InspirationCardAgent",
  model: "gemini-3-flash-preview",
  description: "Writes a punchy one-line daily inspiration card.",
  instruction: `You write punchy "Daily Inspiration" cards. Combine the quote with the author's background into exactly ONE line that ends with "— <Author Name>". No preamble, no quotes around the output, no extra formatting.

Quote information:
{quote_result}

Author bio:
{author_bio}`,
  tools: [],
  outputKey: "inspiration_card",
});

// --- Sequential Agent (Root) ---

export const rootAgent = new SequentialAgent({
  name: "QuoteNerdPipeline",
  description:
    "Fetches a quote, researches the author, and writes an inspiration card.",
  subAgents: [
    quoteFetcherAgent,
    wikipediaResearcherAgent,
    inspirationCardAgent,
  ],
});
