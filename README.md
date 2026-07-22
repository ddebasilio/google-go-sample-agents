 # GDG Event - Google ADK (Agent Development Kit) in Go

This repository contains 8 AI agent applications built with Google's **Agent Development Kit (ADK) for Go** (`google.golang.org/adk`). All projects run locally using **Ollama** and the **`gemma4:12b`** model without requiring cloud API keys or external quotas.

---

## 🚀 Prerequisites & Installing Ollama

To run these AI agents locally, you need [Go 1.22+](https://go.dev/) and [Ollama](https://ollama.com/).

### 1. Install Ollama

#### Linux & macOS
Run the official installation script in your terminal:
```bash
curl -fsSL https://ollama.com/install.sh | sh
```

#### Windows
Download and run the installer from the official website:
👉 [https://ollama.com/download/windows](https://ollama.com/download/windows)

---

### 2. Pull the `gemma4:e4b` Model

Once Ollama is installed, make sure the Ollama service is running:
```bash
ollama serve
```

In a new terminal window, pull the `gemma4:e4b` model:
```bash
ollama pull gemma4:e4b
```

Verify that the model is installed:
```bash
ollama list
```

---

## 📁 Projects in this Repository

| Project | Description | Architecture / Components |
| :--- | :--- | :--- |
| **[`single-agent`](./single-agent)** | Star Wars character lookup assistant | Single `LlmAgent` + SWAPI Go function tool |
| **[`sequential-agent`](./sequential-agent)** | Quote & bio inspiration generator | `SequentialAgent` (QuoteFetcher → WikiResearcher → InspirationCard) |
| **[`parallel-agent`](./parallel-agent)** | Multi-language translation pipeline | `ParallelAgent` (FR, JA, ES, IT) + Sequential `TranslationAggregator` |
| **[`routing-agent`](./routing-agent)** | Ollivanders wand shop coordinator | Root routing agent delegating to `WandSpecialist` & `MagicalTechnician` |
| **[`loop-agent`](./loop-agent)** | Playlist curator with review loop | `SequentialAgent` + `LoopAgent` (Critic + Refiner with MusicBrainz verification) |
| **[`agent-as-tool`](./agent-as-tool)** | Research orchestrator | Sub-agents (`researcher`, `critic`) wrapped as tools (`AgentTool`) |
| **[`coordinator`](./coordinator)** | Deep research web assistant | Web search agent with URL reading capabilities |
| **[`managed-agent`](./managed-agent)** | Managed interaction client | Stateful remote interaction runner |

---

## 💻 How to Run the Applications

1. Start your local Ollama server (if not running as a daemon):
   ```bash
   ollama serve
   ```

2. Navigate into any project directory and run it with `go run .`:
   ```bash
   cd single-agent
   go run .
   ```

3. (Optional) Pass a custom Ollama model tag via environment variable:
   ```bash
   OLLAMA_MODEL="gemma4:12b" go run .
   ```

4. **Launch the ADK Web UI locally:**
   ```bash
   go run . web api webui
   ```
   Then open your browser at **[http://localhost:8080/ui/](http://localhost:8080/ui/)**!

5. (Optional) Run on a custom port or model:
   ```bash
   OLLAMA_MODEL="gemma4:12b" go run . web -port 8085 api webui
   ```

---

## ⚙️ How the Local Ollama Adapter Works

Each project contains a lightweight, self-contained model driver (`ollama.go`) implementing `google.golang.org/adk/model.LLM`. It connects directly to Ollama's local API (`http://localhost:11434/v1/chat/completions`), enabling full support for multi-agent workflows, state management, and function tools locally.
