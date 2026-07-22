package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/agent/workflowagents/loopagent"
	"google.golang.org/adk/agent/workflowagents/sequentialagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/exitlooptool"
	"google.golang.org/adk/tool/functiontool"
)

type VerifySongArgs struct {
	Song   string `json:"song"`
	Artist string `json:"artist"`
}

type TrackInfo struct {
	Title  string `json:"title,omitempty"`
	Artist string `json:"artist,omitempty"`
	Album  string `json:"album,omitempty"`
	Date   string `json:"date,omitempty"`
	Score  int    `json:"score,omitempty"`
}

type VerifySongResult struct {
	Found  bool       `json:"found"`
	Error  string     `json:"error,omitempty"`
	Reason string     `json:"reason,omitempty"`
	Track  *TrackInfo `json:"track,omitempty"`
}

func verifySong(ctx agent.ToolContext, args VerifySongArgs) (VerifySongResult, error) {
	queryStr := fmt.Sprintf(`recording:"%s" AND artist:"%s"`, args.Song, args.Artist)
	mbURL := fmt.Sprintf("https://musicbrainz.org/ws/2/recording?query=%s&limit=1&fmt=json", url.QueryEscape(queryStr))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mbURL, nil)
	if err != nil {
		return VerifySongResult{Found: false, Error: err.Error()}, nil
	}
	req.Header.Set("User-Agent", "PlaylistCuratorAgent/1.0 (playlist-curator-agent)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return VerifySongResult{Found: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return VerifySongResult{Found: false, Error: fmt.Sprintf("MusicBrainz API error: %d", resp.StatusCode)}, nil
	}

	var data struct {
		Recordings []struct {
			Title string `json:"title"`
			Score int    `json:"score"`
			ArtistCredit []struct {
				Name string `json:"name"`
			} `json:"artist-credit"`
			Releases []struct {
				Title string `json:"title"`
			} `json:"releases"`
			FirstReleaseDate string `json:"first-release-date"`
		} `json:"recordings"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return VerifySongResult{Found: false, Error: err.Error()}, nil
	}

	if len(data.Recordings) == 0 {
		return VerifySongResult{Found: false}, nil
	}

	rec := data.Recordings[0]
	if rec.Score < 80 {
		return VerifySongResult{Found: false, Reason: fmt.Sprintf("Low match score: %d", rec.Score)}, nil
	}

	artistName := ""
	if len(rec.ArtistCredit) > 0 {
		artistName = rec.ArtistCredit[0].Name
	}
	albumTitle := ""
	if len(rec.Releases) > 0 {
		albumTitle = rec.Releases[0].Title
	}

	return VerifySongResult{
		Found: true,
		Track: &TrackInfo{
			Title:  rec.Title,
			Artist: artistName,
			Album:  albumTitle,
			Date:   rec.FirstReleaseDate,
			Score:  rec.Score,
		},
	}, nil
}

func main() {
	ctx := context.Background()

	modelName := os.Getenv("OLLAMA_MODEL")
	if modelName == "" {
		modelName = "gemma4:e4b"
	}
	model := newOllamaModel(modelName)

	verifySongTool, err := functiontool.New(
		functiontool.Config{
			Name:        "verify_song",
			Description: "Verify that a song exists by searching the MusicBrainz database. Returns whether the song was found and its track info.",
		},
		verifySong,
	)
	if err != nil {
		panic(err)
	}

	exitLoopTool, err := exitlooptool.New()
	if err != nil {
		panic(err)
	}

	songSchema := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"title":      {Type: genai.TypeString, Description: "The song title"},
			"artist":     {Type: genai.TypeString, Description: "The artist name"},
			"year":       {Type: genai.TypeInteger, Description: "The release year"},
			"why":        {Type: genai.TypeString, Description: "One-line explanation of why this song fits the mood"},
			"youtubeUrl": {Type: genai.TypeString, Description: "YouTube search URL"},
		},
		Required: []string{"title", "artist", "year", "why", "youtubeUrl"},
	}

	playlistSchema := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"songs": {
				Type:        genai.TypeArray,
				Items:       songSchema,
				Description: "The list of songs in the playlist",
			},
		},
		Required: []string{"songs"},
	}

	critiqueSchema := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"verdict": {
				Type:        genai.TypeString,
				Enum:        []string{"PASS", "FAIL"},
				Description: "Whether the playlist passes all criteria",
			},
			"issues": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeString,
				},
				Description: "Description of each issue found",
			},
		},
		Required: []string{"verdict", "issues"},
	}

	generatorAgent, err := llmagent.New(llmagent.Config{
		Name:        "GeneratorAgent",
		Model:       model,
		Description: "Generates an initial 10-song playlist for a given mood.",
		Instruction: `You are a music expert and playlist curator. The user will provide a mood or vibe.

Generate a playlist of exactly 10 songs that match that mood. Choose real, well-known songs that actually exist.

Rules:
- Exactly 10 songs
- No artist should appear more than twice
- No two songs should be from the same year
- Each song must genuinely fit the mood/vibe
- Include a variety of genres and eras when possible
- For each song, set youtubeUrl to "https://www.youtube.com/results?search_query=<artist>+<title>" with spaces replaced by + signs (e.g. "https://www.youtube.com/results?search_query=Norah+Jones+Come+Away+With+Me")`,
		OutputSchema: playlistSchema,
		OutputKey:    "playlist",
	})
	if err != nil {
		panic(err)
	}

	criticAgent, err := llmagent.New(llmagent.Config{
		Name:        "CriticAgent",
		Model:       model,
		Description: "Evaluates the playlist against all criteria and verifies songs via MusicBrainz.",
		Instruction: `You are a strict playlist critic. Your job is to evaluate the current playlist against ALL of the following criteria:

1. Exactly 10 songs
2. No artist appears more than twice
3. No two songs share the same year
4. Mood/vibe coherence — every song should genuinely fit the original mood
5. Each song must be verified as real via the verify_song tool (MusicBrainz) — call it for EVERY song
6. Each entry must include: song title, artist, year, and a one-line "why it fits"

Here is the current playlist:
{playlist?}

Steps:
1. Parse the playlist
2. Check criteria 1-4 and 6 by inspecting the data
3. Call verify_song for each song to check criterion 5
4. Compile your findings

Be thorough and strict. Only output PASS if ALL criteria are met.`,
		Tools:        []tool.Tool{verifySongTool},
		OutputSchema: critiqueSchema,
		OutputKey:    "critique",
	})
	if err != nil {
		panic(err)
	}

	refinerAgent, err := llmagent.New(llmagent.Config{
		Name:        "RefinerAgent",
		Model:       model,
		Description: "If critique passes, exits the loop. If it fails, refines the playlist based on feedback.",
		Instruction: `You are a playlist refiner. You receive a playlist and its critique.

Current playlist:
{playlist?}

Critique:
{critique?}

If the critique verdict is "PASS", call the exit_loop tool immediately to end the review cycle. Do not output a new playlist in that case.

If the critique verdict is "FAIL", fix ALL issues mentioned:
- Replace any songs that could not be verified on MusicBrainz with real, well-known songs
- Fix duplicate years by swapping songs for ones from different years
- Fix artist over-representation by replacing excess songs from the same artist
- Replace any songs that don't fit the mood
- Ensure all 6 criteria will be satisfied`,
		Tools:        []tool.Tool{exitLoopTool},
		OutputSchema: playlistSchema,
		OutputKey:    "playlist",
	})
	if err != nil {
		panic(err)
	}

	critiqueLoop, err := loopagent.New(loopagent.Config{
		MaxIterations: 3,
		AgentConfig: agent.Config{
			Name:        "critique_loop",
			Description: "Review and refine playlist in a loop",
			SubAgents:   []agent.Agent{criticAgent, refinerAgent},
		},
	})
	if err != nil {
		panic(err)
	}

	rootAgent, err := sequentialagent.New(sequentialagent.Config{
		AgentConfig: agent.Config{
			Name:        "playlist_curator",
			Description: "Generates a mood-based playlist and iteratively refines it through critique loops with Spotify verification.",
			SubAgents:   []agent.Agent{generatorAgent, critiqueLoop},
		},
	})
	if err != nil {
		panic(err)
	}

	cfg := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(rootAgent),
	}

	l := full.NewLauncher()
	if err := l.Execute(ctx, cfg, os.Args[1:]); err != nil {
		panic(err)
	}
}
