"""
Step 4 — The Eval Harness.  (The highest-signal part of this whole project.)

THE PROBLEM: "it seems to work" is not evidence. LLM systems are non-deterministic —
you cannot unit-test them like normal code. So how do you KNOW your RAG agent is correct,
and how do you know a change (new chunk size, new model) made it better, not worse?

THE ANSWER: an eval harness.
    1. a LABELED DATASET    — questions with known-correct reference answers
    2. SCORING              — grade each answer. We use TWO methods:
         a) keyword check   — cheap, deterministic: does the answer contain the key fact?
         b) LLM-as-judge    — a model grades correctness vs the reference (catches paraphrases)
    3. a SCOREBOARD         — accuracy + per-question results, so you can track regressions

We reuse the RAG pipeline from step3 (good: one source of truth for the system under test).

Run:
    python step4_eval.py
"""

import json
import time
from google import genai
from google.genai import types, errors

# Reuse the system under test — the actual RAG pipeline from step 3.
from step3_rag import load_documents, build_index, answer

client = genai.Client()
JUDGE_MODEL = "gemini-2.5-flash"


# ---------------------------------------------------------------------------
# RELIABILITY: retry on rate limits (429). Production systems must tolerate
# transient failures instead of crashing. We back off and retry — respecting
# that the free tier allows only ~5 requests/minute. (This is a taste of Step 5.)
# ---------------------------------------------------------------------------
# Retryable = transient: 429 rate limit + 5xx server errors. NOT 400/401/403/404
# (client mistakes won't fix themselves by retrying).
RETRYABLE_CODES = {429, 500, 502, 503, 504}

# PROACTIVE rate limiting: stay UNDER the free-tier 5 req/min by spacing calls out.
# 13s between calls => ~4.6/min, safely under the limit, so we rarely hit a 429 at all.
MIN_INTERVAL_SEC = 13
_last_call_time = [0.0]


def _throttle():
    now = time.monotonic()
    wait = MIN_INTERVAL_SEC - (now - _last_call_time[0])
    if wait > 0:
        time.sleep(wait)
    _last_call_time[0] = time.monotonic()


def with_retry(fn, *args, max_retries=6, **kwargs):
    delay = 20
    for attempt in range(max_retries):
        try:
            _throttle()                  # proactive: pace requests under the quota
            return fn(*args, **kwargs)
        except errors.APIError as e:
            if getattr(e, "code", None) in RETRYABLE_CODES and attempt < max_retries - 1:
                print(f"   ⏳ transient error {e.code} — waiting {delay}s, then retrying...")
                time.sleep(delay)
                delay = min(int(delay * 1.5), 65)   # exponential backoff, capped
                continue
            raise   # non-retryable, or out of retries → let it surface


# ---------------------------------------------------------------------------
# 1) THE LABELED DATASET.
#    Each item: the question, a reference answer, and the key fact(s) that MUST appear.
#    Note the last one is a NEGATIVE test: the data isn't in our corpus, so a correct
#    system should SAY IT DOESN'T KNOW rather than hallucinate. Testing refusal is as
#    important as testing recall.
# ---------------------------------------------------------------------------
EVAL_SET = [
    {
        "question": "What was Apple's Q3 FY2026 revenue?",
        "reference": "Apple's Q3 FY2026 revenue was $94.8 billion, up 7% year over year.",
        "must_include": ["94.8"],
    },
    {
        "question": "What drove Apple's revenue growth in Q3?",
        "reference": "Growth was driven primarily by the Services segment.",
        "must_include": ["Services"],
    },
    {
        "question": "What was Tesla's automotive gross margin excluding regulatory credits?",
        "reference": "Tesla's automotive gross margin excluding credits was 14.2%.",
        "must_include": ["14.2"],
    },
    {
        "question": "How much energy storage did Tesla deploy in the quarter?",
        "reference": "Tesla deployed a record 9.1 GWh of energy storage.",
        "must_include": ["9.1"],
    },
    {
        "question": "What was NVIDIA's Data Center revenue?",
        "reference": "NVIDIA's Data Center revenue was $43.7 billion.",
        "must_include": ["43.7"],
    },
    {
        "question": "What did the Federal Reserve do at its September meeting?",
        "reference": "The Fed held interest rates steady and signaled one possible cut before year end.",
        "must_include": ["steady"],
    },
    {
        # NEGATIVE TEST — Amazon is NOT in our corpus. Correct behavior = say it doesn't know.
        "question": "What was Amazon's Q3 FY2026 revenue?",
        "reference": "This information is not in the provided documents; the system should say it doesn't know.",
        "must_include": [],            # nothing required...
        "must_not_hallucinate": True,  # ...and it must NOT invent a dollar figure
    },
]


# ---------------------------------------------------------------------------
# 2a) KEYWORD SCORING — cheap, deterministic, no API call.
# ---------------------------------------------------------------------------
def keyword_score(prediction, item):
    pred = prediction.lower()
    if item.get("must_not_hallucinate"):
        # Pass only if it admits ignorance and doesn't fabricate a "$<number> billion".
        import re
        admitted = any(p in pred for p in ["don't", "do not", "not in", "no information", "cannot", "unable"])
        fabricated = bool(re.search(r"\$\s?\d", pred))
        return admitted and not fabricated
    return all(kw.lower() in pred for kw in item["must_include"])


# ---------------------------------------------------------------------------
# 2b) LLM-AS-JUDGE — a model grades the answer against the reference.
#     We force JSON output so the verdict is machine-readable. Note: judges have
#     biases (verbosity, position) — we keep the prompt strict and pair it with the
#     cheap keyword check rather than trusting the judge alone.
# ---------------------------------------------------------------------------
def llm_judge(question, reference, prediction):
    prompt = (
        "You are grading a financial Q&A system. Decide if the SYSTEM ANSWER is factually "
        "correct and consistent with the REFERENCE ANSWER. Ignore wording differences; judge "
        "facts. If the reference says the info is unavailable, the system should also decline.\n\n"
        f"QUESTION: {question}\n"
        f"REFERENCE ANSWER: {reference}\n"
        f"SYSTEM ANSWER: {prediction}\n\n"
        'Respond ONLY as JSON: {"verdict": "PASS" or "FAIL", "reason": "<one short sentence>"}'
    )
    resp = client.models.generate_content(
        model=JUDGE_MODEL,
        contents=prompt,
        config=types.GenerateContentConfig(response_mime_type="application/json"),
    )
    data = json.loads(resp.text)
    return data["verdict"].upper() == "PASS", data["reason"]


def main():
    index = build_index(load_documents())
    print(f"📚 Index ready ({len(index)} chunks). Running {len(EVAL_SET)} eval cases...\n")

    kw_pass = judge_pass = 0
    rows = []

    for i, item in enumerate(EVAL_SET, 1):
        prediction, _ = with_retry(answer, item["question"], index)
        kw_ok = keyword_score(prediction, item)
        judge_ok, reason = with_retry(llm_judge, item["question"], item["reference"], prediction)
        kw_pass += kw_ok
        judge_pass += judge_ok
        rows.append((i, kw_ok, judge_ok, item["question"], reason))
        print(f"[{i}] keyword={'✅' if kw_ok else '❌'}  judge={'✅' if judge_ok else '❌'}  {item['question']}")
        print(f"     ↳ judge: {reason}")

    n = len(EVAL_SET)
    print("\n" + "=" * 60)
    print("📊 SCOREBOARD")
    print(f"  Keyword accuracy:     {kw_pass}/{n}  ({kw_pass / n:.0%})")
    print(f"  LLM-judge accuracy:   {judge_pass}/{n}  ({judge_pass / n:.0%})")
    print("=" * 60)
    # Disagreements between the two metrics are the interesting cases to inspect.
    disagree = [r for r in rows if r[1] != r[2]]
    if disagree:
        print("\n⚠️  Metrics disagreed on these (worth a look):")
        for i, kw_ok, judge_ok, q, _ in disagree:
            print(f"   [{i}] keyword={kw_ok} judge={judge_ok}  {q}")


if __name__ == "__main__":
    main()
