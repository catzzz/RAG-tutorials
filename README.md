# RAG-tutorials

A collection of AI agent / RAG projects, each in its own folder. Built to learn and
demonstrate the engineering *around* LLMs — retrieval, evaluation, reliability, cost, and
observability — with the model treated as a swappable component.

## Projects

| Folder | What it is | Status |
|---|---|---|
| [`fin-qa-agent/`](fin-qa-agent/) | Financial Q&A Agent — RAG pipeline with an eval harness, model routing, retry/backoff reliability, and structured tracing | ✅ Complete |
| [`fin-research-agent/`](fin-research-agent/) | Financial Research Agent — multi-tool agentic system (SQL + RAG + price tools, planning, self-correction) | 📋 Planned |

Each project folder has its own README/PLAN with architecture, setup, and run instructions.

## Setup note
API keys live in each project's local `.env` (git-ignored). Copy the `.env.example` in a
project folder to `.env` and add your own key — never commit real keys.
