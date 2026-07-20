import {
  FunctionTool,
  LlmAgent,
  SequentialAgent,
  LoopAgent,
  EXIT_LOOP,
  setLogger,
} from "@google/adk";
import { z } from "zod";

setLogger(null);

// --- MusicBrainz Verify Song Tool ---

const verifySong = new FunctionTool({
  name: "verify_song",
  description:
    "Verify that a song exists by searching the MusicBrainz database. Returns whether the song was found and its track info.",
  parameters: z.object({
    song: z.string().describe("The song title to search for"),
    artist: z.string().describe("The artist name"),
  }),
  execute: async ({ song, artist }) => {
    try {
      const query = encodeURIComponent(
        `recording:"${song}" AND artist:"${artist}"`,
      );
      const res = await fetch(
        `https://musicbrainz.org/ws/2/recording?query=${query}&limit=1&fmt=json`,
        {
          headers: {
            "User-Agent": "PlaylistCuratorAgent/1.0 (playlist-curator-agent)",
          },
        },
      );

      if (!res.ok) {
        return { found: false, error: `MusicBrainz API error: ${res.status}` };
      }

      const data = (await res.json()) as any;
      const recordings = data?.recordings;

      if (!recordings || recordings.length === 0) {
        return { found: false, track: null };
      }

      const recording = recordings[0];
      const score = recording.score ?? 0;

      if (score < 80) {
        return {
          found: false,
          track: null,
          reason: `Low match score: ${score}`,
        };
      }

      return {
        found: true,
        track: {
          title: recording.title,
          artist: recording["artist-credit"]?.[0]?.name,
          album: recording.releases?.[0]?.title,
          date: recording["first-release-date"],
          score,
        },
      };
    } catch (err: any) {
      return { found: false, error: err.message };
    }
  },
});

// --- Schemas ---

const SongSchema = z.object({
  title: z.string().describe("The song title"),
  artist: z.string().describe("The artist name"),
  year: z.number().describe("The release year"),
  why: z
    .string()
    .describe("One-line explanation of why this song fits the mood"),
  youtubeUrl: z
    .string()
    .describe(
      "YouTube search URL: https://www.youtube.com/results?search_query=<artist>+<title> with spaces replaced by + signs",
    ),
});

const PlaylistSchema = z.object({
  songs: z.array(SongSchema).describe("The list of songs in the playlist"),
});

const CritiqueSchema = z.object({
  verdict: z
    .enum(["PASS", "FAIL"])
    .describe("Whether the playlist passes all criteria"),
  issues: z
    .array(z.string())
    .describe("Description of each issue found (empty array if PASS)"),
  songVerifications: z
    .array(
      z.object({
        title: z.string().describe("The song title"),
        artist: z.string().describe("The artist name"),
        verified: z
          .boolean()
          .describe("Whether the song was verified on MusicBrainz"),
      }),
    )
    .describe("Verification results for each song"),
});

// --- Agents ---

const generatorAgent = new LlmAgent({
  name: "GeneratorAgent",
  model: "gemini-flash-latest",
  description: "Generates an initial 10-song playlist for a given mood.",
  instruction: `You are a music expert and playlist curator. The user will provide a mood or vibe.

Generate a playlist of exactly 10 songs that match that mood. Choose real, well-known songs that actually exist.

Rules:
- Exactly 10 songs
- No artist should appear more than twice
- No two songs should be from the same year
- Each song must genuinely fit the mood/vibe
- Include a variety of genres and eras when possible
- For each song, set youtubeUrl to "https://www.youtube.com/results?search_query=<artist>+<title>" with spaces replaced by + signs (e.g. "https://www.youtube.com/results?search_query=Norah+Jones+Come+Away+With+Me")`,
  tools: [],
  outputSchema: PlaylistSchema,
  outputKey: "playlist",
});

const criticAgent = new LlmAgent({
  name: "CriticAgent",
  model: "gemini-flash-latest",
  description:
    "Evaluates the playlist against all criteria and verifies songs via MusicBrainz.",
  instruction: (context) => {
    const playlist = context.state.get("playlist") ?? "No playlist yet.";
    return `You are a strict playlist critic. Your job is to evaluate the current playlist against ALL of the following criteria:

1. Exactly 10 songs
2. No artist appears more than twice
3. No two songs share the same year
4. Mood/vibe coherence — every song should genuinely fit the original mood
5. Each song must be verified as real via the verify_song tool (MusicBrainz) — call it for EVERY song
6. Each entry must include: song title, artist, year, and a one-line "why it fits"

Here is the current playlist:
${JSON.stringify(playlist)}

Steps:
1. Parse the playlist
2. Check criteria 1-4 and 6 by inspecting the data
3. Call verify_song for each song to check criterion 5
4. Compile your findings

Be thorough and strict. Only output PASS if ALL criteria are met.`;
  },
  tools: [verifySong],
  outputSchema: CritiqueSchema,
  outputKey: "critique",
});

const refinerAgent = new LlmAgent({
  name: "RefinerAgent",
  model: "gemini-flash-latest",
  description:
    "If critique passes, exits the loop. If it fails, refines the playlist based on feedback.",
  instruction: (context) => {
    const playlist = context.state.get("playlist") ?? "No playlist yet.";
    const critique = context.state.get("critique") ?? "No critique yet.";
    return `You are a playlist refiner. You receive a playlist and its critique.

Current playlist:
${JSON.stringify(playlist)}

Critique:
${JSON.stringify(critique)}

If the critique verdict is "PASS", call the exit_loop tool immediately to end the review cycle. Do not output a new playlist in that case.

If the critique verdict is "FAIL", fix ALL issues mentioned:
- Replace any songs that could not be verified on MusicBrainz with real, well-known songs
- Fix duplicate years by swapping songs for ones from different years
- Fix artist over-representation by replacing excess songs from the same artist
- Replace any songs that don't fit the mood
- Ensure all 6 criteria will be satisfied`;
  },
  tools: [EXIT_LOOP],
  outputSchema: PlaylistSchema,
  outputKey: "playlist",
});

// --- Loop & Root ---

const critiqueLoop = new LoopAgent({
  name: "critique_loop",
  subAgents: [criticAgent, refinerAgent],
  maxIterations: 3,
});

export const rootAgent = new SequentialAgent({
  name: "playlist_curator",
  description:
    "Generates a mood-based playlist and iteratively refines it through critique loops with Spotify verification.",
  subAgents: [generatorAgent, critiqueLoop],
});
