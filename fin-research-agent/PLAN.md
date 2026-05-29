# 🔬 Project 2 — Financial Research Agent (PLAN — LOCKED)

> Status: **plan locked 2026-05-29** — no code yet. We build step-by-step next, milestone by milestone.
> Predecessor: `../fin-qa-agent` (Financial Q&A Agent, Python — done). This is the **agentic** escalation
> that fills the gaps Project 1 left — and it's written in **Go** to round out a polyglot portfolio.

---

## What it does

Given a research question — e.g. *"Should I be worried about Tesla's margins this quarter?"* —
the agent **plans and executes a multi-step investigation** across heterogeneous tools, then writes
a **cited research memo**:

- 📊 **`query_db`** → text-to-SQL over a structured metrics/prices database
- 📰 **`search_docs`** → RAG retrieval over filings/news (vector similarity)
- 🌐 **`price`** → latest quote (mock "live" API, distinct from historical prices)
- ✍️ **Synthesis** → a structured, cited answer

The agent decides *which* tools to use, *in what order*, chains them, recovers from errors, and grounds
the final memo in what it found.

---

## Why this project (what's NEW vs Project 1)

Project 1 was a **measured RAG pipeline** — read-only, single-step, Python. This proves the skills it didn't:

| New skill | Where it shows up |
|---|---|
| **Multi-step planning** | ReAct agent decomposes a question → sequences tool calls → synthesizes |
| **Structured data via text-to-SQL** | `query_db` — embeddings can't aggregate/count; SQL can |
| **Heterogeneous tool orchestration** | SQL + RAG + price in one agent loop |
| **Self-correction** | Agent feeds tool/SQL errors back into the loop and retries |
| **Data-source routing** | Numeric → SQL, prose → RAG — *emergent from tool descriptions, not a classifier* |
| **Agentic RAG** | Retrieval exposed as a tool the agent *chooses* to call |
| **Agent infra in Go** | Single static binary, goroutines for parallel tools, typed tool contracts |
| **Polyglot portfolio** | Python P1 (ML/RAG) + Go P2 (agent infra) → speaks to both target roles |

This is squarely what **Agent Engineer / AI Infra Engineer** roles hire for.

---

## ✅ Decisions locked

| Area | Decision | Why |
|---|---|---|
| **Language** | **Go** | Reuse real Go wrapper; infra signal; polyglot portfolio (P1 = Python) |
| **Generation** | **Gemini CLI** (`gemini -m … --output-format json`) | Free, large-context, zero-API-key (uses host's authenticated session) |
| **Embeddings** | **Ollama `nomic-embed-text`** (local, ~274 MB) | Free, local, no key — CLI has no embeddings endpoint |
| **Control loop** | **ReAct + JSON action protocol** | CLI has no native function-calling → hand-rolled `{thought, action, action_input}` |
| **SQL tool** | **Agent writes SQL inline** (schema in system prompt) | Fewer CLI calls, one clean trace, visible self-correction |
| **Structured data** | **Hand-built SQLite seed** (3 tickers × 8 quarters) | Zero deps, reproducible; shows the skill, not data engineering |
| **SQLite driver** | **`modernc.org/sqlite`** (pure Go, no cgo) | Single static binary — *is* the infra story |
| **Read-only safety** | DSN `?mode=ro` **+** SELECT-only validation | Belt + suspenders; clean error to feed back for self-correction |
| **Vector storage** | SQLite BLOB; **cosine in Go** | "Index once & persist"; retrieval is *our* code, not the LLM |
| **Deployment** | **Native** (no Docker for the build) | CLI auth is host-bound; Ollama-in-Docker on macOS is CPU-only. Docker = stretch |
| **Observability** | **Trace-first** (tracer wired from M1) | Can't debug a non-deterministic multi-step loop blind |
| **Public-repo safety** | **gitleaks pre-commit hook** + `.gitignore` | Repo is public — block secrets before they ever commit |
| **Reuse policy** | **Copy** patterns from `ai-stock-agent`, never import/modify it | Keep portfolio repo small, legible, self-contained |

---

## Architecture (target)

```
  research question
        │
        ▼
  ┌───────────────── ReAct AGENT LOOP (plan → act → observe → repeat) ─────────────────┐
  │  generation backend: Gemini CLI (free)   ·   JSON action protocol (hand-rolled)     │
  │  routes by tool description; max_steps guard; self-correct on tool/SQL errors        │
  │                                                                                       │
  │    ┌── query_db(sql)    ──► SQLite (read-only)   revenue / eps / margins / prices     │
  │    ├── search_docs(q)   ──► vector search: embed via OLLAMA → cosine in Go            │
  │    └── price(ticker)    ──► mock "live" quote                                         │
  └───────────────────────────────────────────────────────────────────────────────────┘
        │
        ▼
   cited research memo   (each fact tagged: SQL row · doc id · live quote)

  cost split:   generation = Gemini CLI (free)   ·   embeddings = Ollama (free, local)
  observed by:  JSONL tracer — tool order, tokens (from CLI JSON), latency, per step
```

RAG indexing sub-flow (one-time, `search_docs` setup):
```
  docs ─► chunk ─► embed each chunk (Ollama nomic-embed-text) ─► store {chunk, source, vector BLOB} in SQLite
  query time:  embed query ─► load vectors ─► cosine top-k in Go ─► return chunks + source ids
```

---

## Milestone plan (Go — build step-by-step)

- [x] **M1 — Scaffold + safety + LLM backend.** ✅ commit `c0faa65`
      Go module + `config/` `.env` loader; `internal/llm/gemini.go` (CLI wrapper, `--skip-trust` for headless,
      stdin for large prompts, JSON token/latency parse); `internal/trace` tracer (trace-first); `main.go`;
      gitleaks pre-commit hook via `core.hooksPath .githooks/`; `.gitignore` for Go/db.
      *Verified:* real CLI call (in=5659 out=31, 1910ms); gitleaks hook blocks a format-valid GitHub PAT; `.env` untracked.
      *Empirical:* CLI injects **~5.7K scaffolding tokens/call** (one-sentence Q → 5659 input tokens) — confirms the "CLI pollutes token measurement" caveat.
      *Gotchas fixed:* CLI needs `--skip-trust` to run headless; hook uses `gitleaks git --staged` (documented pre-commit cmd; `protect` is undocumented in 8.30.1). Token field is `input` (parses fine).
- [x] **M2 — Data layer + SQL tool.** ✅
      **Real data from SEC EDGAR** (keyless): `cmd/fetch` (maintainer) pulls revenue/net-income/EPS via the
      *companyfacts* API → commits `data/seed.csv` + `data/schema.sql`; `cmd/seed` (anyone, offline) builds
      `data/research.db`. **53 real rows** — AAPL/MSFT/TSLA × 13 + NVDA × 14 (`financials` table; prices are
      the mock `price` tool in M4). `internal/store` = pluggable **Store interface** (SQLite read-only via
      `?mode=ro` + SELECT-only/single-statement guard; `SchemaText()` for the prompt) — live Postgres impl
      can be added later. `internal/tools/sql.go` = `query_db(sql)` → markdown, returns correctable error text.
      `cmd/query` = debug harness.
      *Verified:* 52 rows; guard rejects `DELETE` ("only SELECT/WITH") and `SELECT 1; DROP` ("single statement");
      real numbers cross-checked (AAPL FY rev 383B/391B/416B; TSLA margins ~9%→2-5%; NVDA ~52%).
      *EDGAR gotchas fixed:* `fy`/`fp` tag the *filing's* year → derive period from END-date+duration; quarterly vs
      YTD vs annual mixed → bucket by day-count; use companyfacts (1 call/co); **NVDA reports recent revenue under
      `Revenues`, not `RevenueFromContract…` → pick the revenue tag with the MOST in-range periods, not the first non-empty**.
- [ ] **M3 — RAG `search_docs` tool (Ollama embeddings).**
      `internal/llm/ollama.go` `Embedder`; ingest: chunk corpus → embed → store vectors (BLOB) in SQLite;
      query: embed query → cosine top-k in Go; `tools/search.go` returns top-k chunks + source ids.
- [ ] **M4 — Price tool.** `tools/price.go` `price(ticker)` — mock live quote (distinct from historical SQL prices).
- [ ] **M5 — ReAct loop / planner.** `agent/loop.go` JSON protocol (`{thought, action, action_input}` / `{final_answer}`),
      `max_steps` guard, registry routing via tool descriptions, each tool call wrapped (errors → observation = self-correction);
      `agent/registry.go`; system prompt with tool contract + DB schema + citation rules.
- [ ] **M6 — Synthesis + citations.** Final memo grounded in tool outputs; every fact tagged with its source (SQL row / doc id / quote).
- [ ] **M7 — Eval harness.** `eval/cases.json` (research Qs + expected facts); keyword + LLM-judge scoring. *Stretch:* trajectory eval (right tools?).
- [ ] **M8 — Observability.** Tracer already live; add a trace **summary** (tools used, order, tokens, latency, per run).
- [ ] **M9 — README.** Architecture, metrics, "what failed", setup/run.
- [ ] **(Stretch)** Docker (multi-stage build; swaps CLI→Gemini API since CLI can't auth in a container) · full Ollama generation variant · pgvector/Qdrant · MCP server exposing these tools.

---

## Stack
- **Go** (single static binary)
- **Generation:** Gemini CLI via subprocess (`internal/llm/gemini.go`, mirrored from `ai-stock-agent`)
- **Embeddings:** Ollama `nomic-embed-text` (local, `http://localhost:11434`)
- **Structured data:** SQLite via `modernc.org/sqlite` (pure Go)
- **Vectors:** SQLite BLOB + cosine in Go
- **Config:** `.env` → `MODEL`, `DB_PATH`, `MAX_STEPS`, `TRACE_DIR`, `OLLAMA_URL`, `EMBED_MODEL` *(no API key in the pure CLI+Ollama path)*

## 🔐 Secrets & public-repo safety
- **gitleaks pre-commit hook** (added in M1) blocks accidental secret commits.
- `.gitignore` covers `.env`, `traces/`, `*.db`, `bin/`.
- The default CLI+Ollama path needs **no API key**; only the optional Docker/API stretch variant introduces one.

## Interview angle
> *"My second project is a Go agent runtime: a ReAct loop over three tools — text-to-SQL, RAG, and a price API.
> Generation runs through the free Gemini CLI and embeddings through local Ollama, so the whole agent is zero-cost
> and mostly local. There's no routing classifier — the model routes by reading tool descriptions. It self-corrects
> on SQL/tool errors, traces the full tool-use trajectory, and ships as a single static binary. My first project
> proved measured RAG in Python; this one proves agentic orchestration and structured-data handling in Go."*

---

## ▶️ Next session
Say **"build M1"** → scaffold the Go module, add the gitleaks hook + `.gitignore` + `.env`, copy/trim the
`gemini.go` wrapper, and wire the tracer. Then milestone by milestone — same rhythm as Project 1
(build → explain *why* → interview soundbite → check understanding). This time the agent gets genuinely complex.
