# 📈 Financial Q&A Agent

A retrieval-augmented agent that answers questions over financial documents (earnings
summaries, market news) and returns **cited** answers. Built to be **reliable, measurable,
and cost-aware** — not a demo. The emphasis is the engineering *around* the model: retrieval,
evaluation, reliability, cost routing, and observability.

> Portfolio project for AI Infra / Agent Engineer roles. The model is intentionally treated as
> a swappable component; the value is the harness around it.

---

## What it does

Ask a question like *"What drove Apple's Q3 revenue, and how did Tesla's margins compare?"* and the system:
1. **Retrieves** the most relevant document chunks by semantic similarity,
2. **Generates** a grounded, cited answer (or says *"I don't know"* if the answer isn't in the corpus),
3. does this **reliably** (retries transient failures), **cheaply** (routes easy queries to a small model), and **observably** (every step is traced).

---

## Architecture

```
                    ┌─────────────────────────── INDEXING (once) ───────────────────────────┐
                    │  data/*.txt ─▶ chunk (70w/15 overlap) ─▶ embed ─▶ in-memory vector index │
                    └────────────────────────────────────────────────────────────────────────┘

  question
     │
     ▼
  embed query ──▶ cosine search ──▶ top-k chunks ─┐
  (gemini-embedding-001)                          │
                                                   ▼
                           ┌──────────────── ROUTING ────────────────┐
                           │  cheap model (flash-lite) + confidence    │
                           │      conf ≥ 0.7 ─▶ use answer             │
                           │      conf <  0.7 ─▶ escalate to flash     │
                           └───────────────────────────────────────────┘
                                                   │
                                                   ▼
                                          grounded, cited answer

  cross-cutting:  reliability (retry/backoff + rate-limit throttle)  ·  observability (per-step JSONL trace)
  separately:     eval harness scores answers vs a labeled dataset (keyword + LLM-as-judge)
```

---

## What each piece demonstrates

| File | Capability | Skill signal |
|---|---|---|
| `step1_tool_calling.py` | Tool/function calling | The core agent mechanic |
| `step2_agent_loop.py` | Agent loop (multi-tool, `max_steps` guard) | "An agent is an LLM in a loop" |
| `step3_rag.py` | Pipeline RAG (chunk → embed → cosine search) | Retrieval grounding, hallucination control |
| `step4_eval.py` | Eval harness (keyword + LLM-as-judge + scoreboard) | **Making non-determinism measurable** |
| `step5_routing.py` | Cheap→strong model routing + cost/latency instrumentation | Cost engineering (AI Infra) |
| `step6_observability.py` | Structured per-step tracing (JSONL) | Debuggability / production readiness |

---

## Metrics

Measured on the free tier (small synthetic corpus). Reproduce with the commands below.

| Metric | Value | Source |
|---|---|---|
| Eval — answerable cases (keyword + LLM-judge agreement) | 5/5 passed* | `step4_eval.py` |
| Refusal test (out-of-corpus question → "I don't know") | designed-in, grounded prompt | `step4_eval.py` |
| Cost per query (cheap model) | ~**$0.00005**/query | `step5_routing.py` |
| Routing — easy queries kept on cheap model | 3/3 at confidence 1.0 | `step5_routing.py` |
| Retrieval latency | ~0.35s | `step6_observability.py` trace |
| Generation latency (cheap model) | ~1.0s | `step6_observability.py` trace |

\* Full 7-case eval (incl. the refusal case) requires a clean run; on the free tier (5 req/min)
this is rate-limited — see *What I learned* below. Run `python step4_eval.py` to populate the
complete scoreboard. **To make the numbers comparable, the eval is the regression gate: change a
parameter (chunk size, model, threshold) and re-run to prove the change helped.**

---

## What I learned (debugging notes)

The interesting part wasn't the happy path — it was making it robust. Real issues hit and fixed:

- **`404` on `text-embedding-004`.** The embedding model name from a tutorial wasn't available on my API version. **Fix:** queried `client.models.list()` to see what the key *actually* supports, switched to `gemini-embedding-001`. *Lesson: verify against the API, don't trust model names from docs/tutorials — they drift.*

- **`429 RESOURCE_EXHAUSTED`.** The eval fires ~2 model calls per question and blew past the free-tier 5 req/min limit. **Fix:** wrapped calls in **retry with exponential backoff**.

- **`503 UNAVAILABLE` slipped through the retry.** My first retry only caught `429`. **Fix:** broadened to catch the base `APIError` and retry a *set* of transient codes (`429, 500, 502, 503, 504`) while letting non-retryable `4xx` (400/401/404) surface immediately. *Lesson: knowing which errors are retryable is the skill.*

- **429s persisted even with backoff.** Reactive retry alone is always one step behind a rolling rate limit. **Fix:** added a **proactive client-side throttle** to stay *under* quota by design (proactive limiting + reactive backoff together).

- **"But my Gemini account is paid!"** The 429 said `free_tier`. **Root cause:** a consumer Gemini subscription ≠ API billing — API quota is tied to the Cloud project's billing, not the app subscription. *Lesson: the API and the consumer product are separately billed.*

- **Considered using the Gemini CLI as the backend to dodge limits.** Probed it: the CLI *does* expose token usage (`-o json`) and honor `-m`, but injects ~9K tokens of agent scaffolding per call and runs its own tool loop. *Conclusion: an agent CLI is a runtime, not a raw endpoint — great for complex agentic tasks, wrong for a measured pipeline where clean cost attribution matters.*

---

## Tech stack

- **Python**, `google-genai` SDK
- **Generation:** `gemini-2.5-flash-lite` (cheap) / `gemini-2.5-flash` (escalation)
- **Embeddings:** `gemini-embedding-001`
- **Vector search:** in-memory cosine (NumPy) — by hand, to show the mechanism a vector DB indexes

## Setup

```bash
cd fin-qa-agent
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
cp .env.example .env        # paste your GEMINI_API_KEY (from https://aistudio.google.com/)
```

## Run

```bash
python step1_tool_calling.py     # tool calling
python step2_agent_loop.py       # agent loop (multi-tool chaining)
python step3_rag.py              # RAG over financial docs
python step4_eval.py             # eval harness + scoreboard
python step5_routing.py          # cheap→strong routing + cost/latency
python step6_observability.py    # structured per-step trace (writes traces/*.jsonl)
```

## What I'd do next for production

- **Vector DB** (pgvector/Qdrant): persist embeddings (embed once, reuse), ANN indexing for scale, metadata filtering.
- **Hybrid search** (BM25 + vector) + a **reranker** for better recall/precision.
- **Agentic RAG**: expose retrieval as a `search_documents` tool so the agent decides when to search.
- **Route structured data to SQL**, not embeddings (embeddings can't aggregate/count).
- **Eval in CI**: run the harness on every change as a regression gate; track per-category slices.
- **Local model variant** (Ollama) for a fully-offline, $0 deployment.
