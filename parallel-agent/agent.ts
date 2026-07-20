import {
  FunctionTool,
  LlmAgent,
  ParallelAgent,
  SequentialAgent,
  setLogger,
} from "@google/adk";
import { z } from "zod";

setLogger(null);

// --- Translation Tool ---

const translate = new FunctionTool({
  name: "translate_text",
  description:
    "Translate text from English to a target language using the MyMemory translation API",
  parameters: z.object({
    text: z.string().describe("The English text to translate"),
    target_lang: z
      .string()
      .describe("The target language code (e.g. fr, ja, es)"),
  }),
  execute: async ({ text, target_lang }) => {
    const url = `https://api.mymemory.translated.net/get?q=${encodeURIComponent(text)}&langpair=en|${target_lang}`;
    const res = await fetch(url);
    const data = (await res.json()) as any;
    return { translation: data.responseData.translatedText };
  },
});

// --- Translation Sub-Agents (run in parallel) ---

const frenchAgent = new LlmAgent({
  name: "FrenchTranslator",
  model: "gemini-flash-latest",
  description: "Translates the user's text into French.",
  instruction: `You are a French translator. Whatever the user says, treat their entire message as text to translate. Immediately call the translate_text tool with the user's exact message as the text and target_lang "fr". Return only the translated text, nothing else.`,
  tools: [translate],
  outputKey: "french_translation",
});

const japaneseAgent = new LlmAgent({
  name: "JapaneseTranslator",
  model: "gemini-flash-latest",
  description: "Translates the user's text into Japanese.",
  instruction: `You are a Japanese translator. Whatever the user says, treat their entire message as text to translate. Immediately call the translate_text tool with the user's exact message as the text and target_lang "ja". Return only the translated text, nothing else.`,
  tools: [translate],
  outputKey: "japanese_translation",
});

const spanishAgent = new LlmAgent({
  name: "SpanishTranslator",
  model: "gemini-flash-latest",
  description: "Translates the user's text into Spanish.",
  instruction: `You are a Spanish translator. Whatever the user says, treat their entire message as text to translate. Immediately call the translate_text tool with the user's exact message as the text and target_lang "es". Return only the translated text, nothing else.`,
  tools: [translate],
  outputKey: "spanish_translation",
});

const italianAgent = new LlmAgent({
  name: "ItalianTranslator",
  model: "gemini-flash-latest",
  description: "Translates the user's text into italian.",
  instruction: `You are an italian translator. Whatever the user says, treat their entire message as text to translate. Immediately call the translate_text tool with the user's exact message as the text and target_lang "it". Return only the translated text, nothing else.`,
  tools: [translate],
  outputKey: "italian_translation",
});

// --- Parallel Agent ---

const parallelTranslator = new ParallelAgent({
  name: "ParallelTranslator",
  description:
    "Runs Italian, French, Japanese, and Spanish translators in parallel.",
  subAgents: [italianAgent, frenchAgent, japaneseAgent, spanishAgent],
});

// --- Aggregator Agent ---

const aggregatorAgent = new LlmAgent({
  name: "TranslationAggregator",
  model: "gemini-flash-latest",
  description:
    "Aggregates parallel translation results and provides linguistic insights.",
  instruction: `You are an aggregator in a parallel agent pipeline. You receive four translations of the same word or phrase. Present them clearly, then add a one-sentence note on any interesting linguistic differences between them.

Italian: {italian_translation}
French: {french_translation}
Japanese: {japanese_translation}
Spanish: {spanish_translation}`,
  tools: [],
  outputKey: "aggregated_result",
});

// --- Root Agent: Sequential(Parallel → Aggregator) ---

export const rootAgent = new SequentialAgent({
  name: "ParallelTranslationPipeline",
  description:
    "Translates text into 4 languages in parallel, then aggregates the results.",
  subAgents: [parallelTranslator, aggregatorAgent],
});
