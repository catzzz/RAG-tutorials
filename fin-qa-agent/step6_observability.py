"""
Step 6 — Observability.

THE PROBLEM: when an LLM system gives a weird answer, "it's non-deterministic" is not a
debugging strategy. You need to see INSIDE a single run: what was retrieved, what prompt
was sent, what the model returned, how many tokens, how long, how much it cost — per step.

THE ANSWER: structured tracing. Every step emits a structured event (a dict / JSON line),
tied together by a run_id. You can then read the trace to answer "why did it do that?",
break down latency/cost, and (in production) ship these events to a tracing tool.

Why STRUCTURED (JSON) instead of print()? Because structured logs are machine-readable:
you can query them ("show all calls over 2s"), aggregate them (avg cost/run), and feed
them to dashboards (Langfuse / LangSmith / OpenTelemetry). print() gives you none of that.

Reuses step3 retrieval and step4 reliability.

Run:
    python step6_observability.py
"""

import os
import json
import time
import uuid
from google import genai

from step3_rag import load_documents, build_index, retrieve
from step4_eval import with_retry

client = genai.Client()
MODEL = "gemini-2.5-flash-lite"


# ---------------------------------------------------------------------------
# THE TRACER: collects structured events for one run, prints them, saves JSONL.
# ---------------------------------------------------------------------------
class Tracer:
    def __init__(self):
        self.run_id = str(uuid.uuid4())[:8]   # ties all steps of THIS run together
        self.events = []

    def log(self, step_type, **fields):
        event = {"run_id": self.run_id, "ts": time.time(), "step": step_type, **fields}
        self.events.append(event)
        return event

    def save(self, folder="traces"):
        os.makedirs(folder, exist_ok=True)
        path = os.path.join(folder, f"run-{self.run_id}.jsonl")
        with open(path, "w") as f:
            for e in self.events:
                f.write(json.dumps(e) + "\n")   # one JSON object per line = JSONL
        return path

    def print_trace(self):
        print(f"\n🔍 TRACE  run_id={self.run_id}  ({len(self.events)} steps)")
        total_cost = total_latency = 0.0
        for e in self.events:
            lat = e.get("latency_s", 0)
            total_latency += lat
            if e["step"] == "retrieval":
                hits = ", ".join(f"{h['source']}({h['score']})" for h in e["hits"])
                print(f"  • retrieval   {lat:.2f}s  query='{e['query'][:40]}'  → {hits}")
            elif e["step"] == "llm_call":
                total_cost += e.get("cost", 0)
                print(f"  • llm_call    {lat:.2f}s  {e['model']}  "
                      f"in={e['in_tokens']} out={e['out_tokens']} ${e.get('cost', 0):.6f}")
                print(f"                prompt='{e['prompt_preview']}'")
                print(f"                output='{e['output_preview']}'")
        print(f"  └─ totals: {total_latency:.2f}s, ${total_cost:.6f}")


# ---------------------------------------------------------------------------
# Traced building blocks — each wraps a real operation and logs an event.
# ---------------------------------------------------------------------------
def traced_retrieve(tracer, query, index, k=3):
    t0 = time.perf_counter()
    hits = retrieve(query, index, k)
    tracer.log(
        "retrieval",
        query=query,
        latency_s=round(time.perf_counter() - t0, 2),
        hits=[{"source": c["source"], "score": round(s, 3)} for s, c in hits],
    )
    return hits


def traced_generate(tracer, prompt, step_name):
    t0 = time.perf_counter()
    resp = with_retry(client.models.generate_content, model=MODEL, contents=prompt)
    latency = time.perf_counter() - t0
    u = resp.usage_metadata
    cost = (u.prompt_token_count / 1e6) * 0.10 + (u.candidates_token_count / 1e6) * 0.40
    tracer.log(
        "llm_call",
        name=step_name,
        model=MODEL,
        prompt_preview=prompt[:80].replace("\n", " "),
        output_preview=resp.text[:80].replace("\n", " "),
        in_tokens=u.prompt_token_count,
        out_tokens=u.candidates_token_count,
        latency_s=round(latency, 2),
        cost=round(cost, 6),
    )
    return resp.text


def answer_traced(question, index):
    tracer = Tracer()
    hits = traced_retrieve(tracer, question, index)
    context = "\n\n".join(f"[{c['source']}] {c['text']}" for _, c in hits)
    prompt = (
        "Answer using ONLY the context. Cite sources in brackets. If not present, say you don't know.\n\n"
        f"Context:\n{context}\n\nQuestion: {question}"
    )
    answer = traced_generate(tracer, prompt, "answer")
    return answer, tracer


def main():
    index = build_index(load_documents())
    question = "What was NVIDIA's Data Center revenue and what is driving demand?"
    print(f"🧑 User: {question}")

    answer, tracer = answer_traced(question, index)
    print(f"\n✅ Answer: {answer}")

    tracer.print_trace()
    path = tracer.save()
    print(f"\n💾 Full structured trace saved to: {path}")
    print("   (each line is one JSON event — queryable, aggregatable, dashboard-ready)")


if __name__ == "__main__":
    main()
