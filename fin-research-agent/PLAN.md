# 🔬 Project 2 — Financial Research Agent (PLAN)

> Status: **planning only** — no code yet. We build this step-by-step next session.
> Predecessor: `../fin-qa-agent` (Financial Q&A Agent — done). This project is the
> **agentic** escalation that fills the gaps Project 1 left.

---

## What it does

Given a research question — e.g. *"Should I be worried about Tesla's margins this quarter?"* —
the agent **plans and executes a multi-step investigation** across heterogeneous tools, then
writes a **cited research memo**:

- 📊 **SQL tool** → query a structured metrics/prices database (text-to-SQL)
- 📰 **Search tool** → retrieve from filings/news (RAG — reused from Project 1)
- 🌐 **Price tool** → live/latest quote (API-style tool)
- ✍️ **Synthesis** → a structured, cited answer

The agent decides *which* tools to use, *in what order*, chains them, recovers from errors,
and grounds the final memo in what it found.

---

## Why this project (what's NEW vs Project 1)

Project 1 was a **measured RAG pipeline** — read-only, single-step. This project proves the
skills it didn't:

| New skill | Where it shows up |
|---|---|
| **Multi-step planning** | Agent decomposes a question → sequences tool calls → synthesizes |
| **Structured data via SQL tool (text-to-SQL)** | The SQL tool — embeddings can't aggregate/count; this can |
| **Heterogeneous tool orchestration** | SQL + RAG + price API in one agent loop |
| **Self-correction** | Agent retries when a tool/SQL call fails (feeds the error back) |
| **Data-source routing** | Numeric/aggregate → SQL; prose → RAG (the routing we discussed but never built) |
| **Agentic RAG** | Retrieval exposed as a tool the agent *chooses* to call |
| **(Stretch) real vector DB** | pgvector/Qdrant — persistence + scale |

This is squarely what **Agent Engineer** roles hire for — and it's the project where an
**agent runtime / the Gemini CLI** finally earns its keep (genuinely complex, multi-tool).

---

## Architecture (target)

```
  research question
        │
        ▼
   ┌─────────────────── AGENT LOOP (plan → act → observe → repeat) ───────────────────┐
   │   the model picks tools and sequences them until it can answer:                    │
   │                                                                                    │
   │     ┌── SQL tool ──────► SQLite metrics DB   (revenue, margins, prices...)         │
   │     ├── search tool ───► RAG over filings/news  (reuses Project 1 retrieval)       │
   │     └── price tool ────► latest quote (API-style)                                  │
   │                                                                                    │
   │   cross-cutting: retry/backoff (reuse) · self-correction on tool errors            │
   └────────────────────────────────────────────────────────────────────────────────────┘
        │
        ▼
   cited research memo  (which numbers came from SQL, which facts from which doc)

  evaluated by: eval harness (final answer + optionally the tool-use trajectory)
  observed by:  per-step trace (which tools, in what order, tokens/latency/cost) — reuse Project 1
```

---

## Milestone plan (we'll do these step-by-step)

- [ ] **M1 — Scaffold + data layer.** venv, Gemini SDK, `.env`. Create a small **SQLite** DB (`prices`/`fundamentals` tables, synthetic) + reuse Project 1's document corpus.
- [ ] **M2 — SQL tool (text-to-SQL).** Tool that takes a NL request → generates SQL (schema in prompt) → runs **read-only** against SQLite → returns rows. Validate; **self-correct** by feeding SQL errors back to the model.
- [ ] **M3 — Search tool (agentic RAG).** Wrap Project 1's retrieval as a `search_documents` tool the agent can call. (Port/import from `fin-qa-agent`.)
- [ ] **M4 — Price tool.** Simple API-style tool (mock first, real later).
- [ ] **M5 — The agent loop / planner.** Multi-tool loop (reuse the Project 1 loop pattern): model chooses among SQL / search / price, chains them, `max_steps` guard, reliability wrapper.
- [ ] **M6 — Synthesis + citations.** Final memo grounded in tool outputs, citing where each fact came from (SQL row vs document).
- [ ] **M7 — Eval harness.** Labeled research questions with expected facts; score the final answer (keyword + LLM-judge). Stretch: also score the **trajectory** (did it use the right tools?).
- [ ] **M8 — Observability.** Trace the full multi-step trajectory (tool order, tokens, latency, cost) — extend Project 1's tracer.
- [ ] **M9 — README writeup** with architecture, metrics, and "what failed."
- [ ] **(Stretch)** real vector DB (pgvector/Qdrant) · local model (Ollama) variant · MCP server exposing these tools.

---

## Stack (target)
- **Python**, `google-genai` SDK (same as Project 1)
- **Structured data:** SQLite (zero-setup; upgrade to Postgres/pgvector as a stretch)
- **Retrieval:** reuse Project 1's RAG (in-memory → vector DB later)
- **Reuse from Project 1:** `with_retry` (reliability), the agent-loop pattern, the tracer

## Interview angle (why this project lands)
> *"My second project is a multi-tool research agent: it plans across a SQL tool for structured
> metrics, a RAG tool for documents, and a price API, and synthesizes a cited memo. It routes
> numeric questions to SQL and prose questions to retrieval, self-corrects on tool failures, and
> I trace the full tool-use trajectory. It shows the agentic planning and structured-data handling
> my first RAG project didn't."*

---

## ▶️ Next session
Say **"let's build Project 2, M1"** and we'll scaffold the data layer + SQLite DB, then go
milestone by milestone — same rhythm as Project 1 (build → explain *why* → interview soundbite → check understanding). This time we let the agent get genuinely complex.
