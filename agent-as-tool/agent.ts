import {
  LlmAgent,
  AgentTool,
  GOOGLE_SEARCH,
  URL_CONTEXT,
  setLogLevel,
  LogLevel,
} from "@google/adk";
import { z } from "zod";

setLogLevel(LogLevel.ERROR);

// --- Schemas ---

const FindingsSchema = z.object({
  findings: z
    .array(
      z.object({
        point: z
          .string()
          .describe("A concise finding that addresses the brief"),
        source: z.string().describe("The source URL backing this finding"),
      }),
    )
    .describe("4 to 8 findings covering the brief, each with a source URL"),
});

const CritiqueSchema = z.object({
  verdict: z
    .enum(["APPROVED", "NEEDS_REVISION"])
    .describe(
      "APPROVED if the findings fully cover the request, otherwise NEEDS_REVISION",
    ),
  gaps: z
    .array(z.string())
    .describe(
      'Specific, actionable gaps a researcher could act on directly (e.g. "missing the compliance deadline for general-purpose AI"). Empty array when APPROVED.',
    ),
});

const researcher = new LlmAgent({
  name: "researcher",
  model: "gemini-2.5-flash",
  description:
    "Researches a topic on the web. Accepts a brief and returns findings with sources. Can be re-called with a sharper brief to fill specific gaps.",
  instruction: `You are a researcher.

Given a brief, search the web and return 4 to 8 findings. Each finding must be a single concise point paired with the source URL it came from.

If the brief targets specific gaps (e.g. "find compliance deadlines"), focus only on those gaps. Do not repeat earlier ground.

The structured output schema enforces the shape — populate the findings array; do not add preamble.`,
  tools: [GOOGLE_SEARCH, URL_CONTEXT],
  outputSchema: FindingsSchema,
  outputKey: "findings",
});

const critic = new LlmAgent({
  name: "critic",
  model: "gemini-2.5-flash",
  description:
    "Reviews research findings for completeness. Returns a structured verdict (APPROVED or NEEDS_REVISION) with specific gaps.",
  instruction: `You are a research critic.

Read the findings against the original user request. Check for:
- Missing facts directly implied by the request
- Vague claims without sources
- Outdated or generic information where specifics are needed

Set verdict to "APPROVED" if the findings fully cover the request, otherwise "NEEDS_REVISION". When NEEDS_REVISION, list each gap in the gaps array — each gap must be concrete enough that a researcher could act on it directly. When APPROVED, leave gaps as an empty array.

Do not rewrite the findings. Only judge them.`,
  outputSchema: CritiqueSchema,
});

export const rootAgent = new LlmAgent({
  name: "research_root",
  model: "gemini-flash-latest",
  description:
    "Iteratively researches a topic with a critic-driven refinement loop.",
  instruction: `You orchestrate research with a critic.

Both tools return structured JSON:
- researcher returns { findings: [{ point, source }] }.
- critic returns { verdict: "APPROVED" | "NEEDS_REVISION", gaps: [string] }.

Process:
1. Call researcher with a brief derived from the user's request.
2. Call critic with the findings.
3. If critic's verdict is "APPROVED", write the final report and stop.
4. If critic's verdict is "NEEDS_REVISION", call researcher AGAIN with a brief targeting ONLY the entries in critic's gaps array. Do not re-research what you already have.
5. Call critic again with the combined findings.
6. Repeat steps 4 and 5 until the verdict is "APPROVED".

Rules:
- Always start by calling researcher. Never answer from memory.
- When re-calling researcher, the brief must reference the specific gaps from the critic.
- The final report should integrate all research passes, not just the last one.
- Cite the source URL from each finding in the final report.`,
  tools: [
    new AgentTool({ agent: researcher }),
    new AgentTool({ agent: critic }),
  ],
});
