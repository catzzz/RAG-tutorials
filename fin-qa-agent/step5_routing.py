"""
Step 5 — Reliability + Cost: MODEL ROUTING with cost/latency instrumentation.

You already built reliability (retry/backoff + throttle in step4). This step adds the
COST side, which is the core of "AI Infra":

    Don't send every query to the expensive model. Try a CHEAP model first; only
    ESCALATE to a strong model when the cheap one is not confident. Most queries are
    easy, so you pay the cheap price for them and the strong price only when needed.

We also INSTRUMENT every call — tokens, estimated cost, latency — because you can't
optimize (or put numbers in a README) what you don't measure.

Reuses step3 retrieval and step4's with_retry (reliability).

Run:
    python step5_routing.py
"""

import json
import time
from google import genai
from google.genai import types

from step3_rag import load_documents, build_index, retrieve
from step4_eval import with_retry   # reuse reliability: backoff + throttle

client = genai.Client()

CHEAP_MODEL = "gemini-2.5-flash-lite"   # handles the easy majority
STRONG_MODEL = "gemini-2.5-flash"       # escalate here when cheap isn't confident
CONFIDENCE_THRESHOLD = 0.7              # below this -> escalate

# Approximate prices (USD per 1M tokens). ILLUSTRATIVE — adjust to current pricing.
# The point is the mechanism + relative cost, not exact dollars.
PRICES = {
    CHEAP_MODEL:  {"in": 0.10, "out": 0.40},
    STRONG_MODEL: {"in": 0.30, "out": 2.50},
}


def estimate_cost(model, usage):
    p = PRICES[model]
    return (usage.prompt_token_count / 1e6) * p["in"] + (usage.candidates_token_count / 1e6) * p["out"]


def call_model(model, prompt, json_mode=False):
    """One model call, INSTRUMENTED: returns text + token usage + latency + cost."""
    cfg = types.GenerateContentConfig(response_mime_type="application/json") if json_mode else None
    t0 = time.perf_counter()
    resp = with_retry(client.models.generate_content, model=model, contents=prompt, config=cfg)
    latency = time.perf_counter() - t0
    usage = resp.usage_metadata
    return resp.text, {
        "model": model,
        "in_tokens": usage.prompt_token_count,
        "out_tokens": usage.candidates_token_count,
        "latency": latency,
        "cost": estimate_cost(model, usage),
    }


def build_context(hits):
    return "\n\n".join(f"[{c['source']}] {c['text']}" for _, c in hits)


def answer_with_routing(question, index):
    """Cheap-first, escalate-if-unsure. Returns (answer, path, [call records])."""
    context = build_context(retrieve(question, index))
    records = []

    # 1) CHEAP attempt — also ask it to self-report confidence the context supports the answer.
    cheap_prompt = (
        "Answer the question using ONLY the context. Cite sources in brackets. Then rate your "
        "confidence from 0 to 1 that the context fully supports your answer (0 if the answer is "
        "not in the context).\n\n"
        f"Context:\n{context}\n\nQuestion: {question}\n\n"
        'Respond as JSON: {"answer": "...", "confidence": 0.0}'
    )
    text, rec = call_model(CHEAP_MODEL, cheap_prompt, json_mode=True)
    records.append(rec)
    data = json.loads(text)
    conf = float(data.get("confidence", 0))

    if conf >= CONFIDENCE_THRESHOLD:
        return data["answer"], f"cheap (conf={conf:.2f})", records

    # 2) ESCALATE — strong model handles the hard / low-confidence case.
    strong_prompt = (
        "Answer the question using ONLY the context. Cite sources in brackets. If the answer is "
        "not in the context, say you don't know.\n\n"
        f"Context:\n{context}\n\nQuestion: {question}"
    )
    text2, rec2 = call_model(STRONG_MODEL, strong_prompt)
    records.append(rec2)
    return text2, f"escalated (cheap conf={conf:.2f})", records


def main():
    index = build_index(load_documents())
    questions = [
        "What was Apple's Q3 FY2026 revenue?",                    # easy -> cheap
        "What was NVIDIA's Data Center revenue?",                 # easy -> cheap
        "Compare Apple's and Tesla's gross margins this quarter.",# harder -> maybe escalate
        "What was Amazon's Q3 FY2026 revenue?",                   # not in corpus -> low conf -> escalate
    ]

    total_cost = strong_only_cost = 0.0
    escalations = 0
    print(f"📚 Index ready. Routing {len(questions)} queries "
          f"({CHEAP_MODEL} → {STRONG_MODEL}, escalate if conf < {CONFIDENCE_THRESHOLD})\n")

    for q in questions:
        ans, path, records = answer_with_routing(q, index)
        q_cost = sum(r["cost"] for r in records)
        q_latency = sum(r["latency"] for r in records)
        total_cost += q_cost
        # Counterfactual: what if we had sent this straight to the strong model?
        in_tok = records[0]["in_tokens"]; out_tok = records[0]["out_tokens"]
        strong_only_cost += (in_tok / 1e6) * PRICES[STRONG_MODEL]["in"] + (out_tok / 1e6) * PRICES[STRONG_MODEL]["out"]
        if "escalated" in path:
            escalations += 1
        print(f"🧑 {q}")
        print(f"   route: {path} | cost: ${q_cost:.6f} | latency: {q_latency:.2f}s")
        print(f"   answer: {ans[:100]}{'...' if len(ans) > 100 else ''}\n")

    n = len(questions)
    print("=" * 60)
    print("📊 ROUTING SUMMARY")
    print(f"  Escalation rate:     {escalations}/{n}")
    print(f"  Routed total cost:   ${total_cost:.6f}")
    print(f"  Strong-only cost:    ${strong_only_cost:.6f}  (if every query used {STRONG_MODEL})")
    if strong_only_cost > 0:
        print(f"  Savings:             {(1 - total_cost / strong_only_cost):.0%}")
    print("=" * 60)


if __name__ == "__main__":
    main()
