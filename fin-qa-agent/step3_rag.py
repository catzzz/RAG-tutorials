"""
Step 3 — RAG (Retrieval-Augmented Generation).

PROBLEM: the model doesn't know your private/fresh data (our Q3 earnings docs are
synthetic — they're nowhere in its training). If you just ask, it will guess or refuse.

SOLUTION (RAG): before answering, RETRIEVE the most relevant chunks of YOUR documents
and put them in the prompt as context. The model then answers FROM that context.

The pipeline (memorize this shape):
    LOAD docs -> CHUNK them -> EMBED each chunk -> store vectors
    then per question:  EMBED the question -> find nearest chunks (cosine sim) -> stuff
    them into the prompt -> GENERATE an answer.

We build the vector search BY HAND with numpy so you can see there's no magic:
a "vector DB" just stores these vectors and does this nearest-neighbor search faster.

Run:
    python step3_rag.py
"""

import os
import glob
import numpy as np
from dotenv import load_dotenv
from google import genai
from google.genai import types

load_dotenv()
client = genai.Client()

GEN_MODEL = "gemini-2.5-flash"
EMBED_MODEL = "gemini-embedding-001"   # turns text -> a vector of numbers (3072 dims)


# --- 1. LOAD ---------------------------------------------------------------
def load_documents(folder="data"):
    docs = []
    for path in sorted(glob.glob(os.path.join(folder, "*.txt"))):
        with open(path) as f:
            docs.append({"source": os.path.basename(path), "text": f.read()})
    return docs


# --- 2. CHUNK --------------------------------------------------------------
# Why chunk? Embeddings capture a *limited* amount of meaning per vector, and we want to
# retrieve only the relevant passage, not a whole document. Overlap avoids cutting a fact
# in half at a boundary.
def chunk_text(text, size=70, overlap=15):
    words = text.split()
    chunks, start = [], 0
    while start < len(words):
        chunks.append(" ".join(words[start : start + size]))
        start += size - overlap
    return chunks


# --- 3. EMBED --------------------------------------------------------------
# An embedding maps text -> a vector of numbers such that similar MEANINGS land close
# together in space. Note the asymmetric task_type: documents and queries are embedded
# slightly differently, which improves retrieval quality.
def embed(texts, task_type):
    resp = client.models.embed_content(
        model=EMBED_MODEL,
        contents=texts,
        config=types.EmbedContentConfig(task_type=task_type),
    )
    return [np.array(e.values) for e in resp.embeddings]


def build_index(docs):
    """Flatten docs into chunks and attach an embedding vector to each. This in-memory
    list IS our 'vector store'. A real vector DB (pgvector/Qdrant) stores the same thing."""
    index = []
    for doc in docs:
        for chunk in chunk_text(doc["text"]):
            index.append({"source": doc["source"], "text": chunk})
    vectors = embed([c["text"] for c in index], task_type="RETRIEVAL_DOCUMENT")
    for c, v in zip(index, vectors):
        c["vector"] = v
    return index


# --- 4. RETRIEVE -----------------------------------------------------------
def cosine(a, b):
    # similarity = how aligned two vectors are. 1.0 = identical meaning, 0 = unrelated.
    return float(np.dot(a, b) / (np.linalg.norm(a) * np.linalg.norm(b)))


def retrieve(query, index, k=3):
    qvec = embed([query], task_type="RETRIEVAL_QUERY")[0]
    scored = [(cosine(qvec, c["vector"]), c) for c in index]
    scored.sort(key=lambda x: x[0], reverse=True)   # highest similarity first
    return scored[:k]   # top-k nearest chunks


# --- 5. GENERATE (the "G" in RAG) -----------------------------------------
def answer(query, index, k=3):
    hits = retrieve(query, index, k)

    context = "\n\n".join(f"[{c['source']}] {c['text']}" for _, c in hits)
    prompt = (
        "You are a financial analyst assistant. Answer the question using ONLY the context "
        "below. Cite the source file in brackets, e.g. [aapl_q3_2026.txt]. If the answer is "
        "not in the context, say you don't have that information.\n\n"
        f"Context:\n{context}\n\nQuestion: {query}"
    )
    resp = client.models.generate_content(model=GEN_MODEL, contents=prompt)
    return resp.text, hits


if __name__ == "__main__":
    docs = load_documents()
    index = build_index(docs)
    print(f"📚 Loaded {len(docs)} documents -> {len(index)} chunks (each embedded to a vector)\n")

    question = "What was Apple's Q3 revenue and what drove the growth?"
    print(f"🧑 User: {question}\n")

    ans, hits = answer(question, index)

    print("🔎 Retrieved chunks (by similarity):")
    for score, c in hits:
        print(f"  {score:.3f}  [{c['source']}]  {c['text'][:70]}...")
    print(f"\n✅ Answer:\n{ans}\n")
