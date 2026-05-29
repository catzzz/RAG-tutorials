"""
Step 2 — The Agent Loop.

This is THE concept. An "agent" = the Step 1 mechanic wrapped in a `while` loop.

In Step 1 we did exactly ONE tool call. But real questions need several:
    "Compare Apple and Tesla's price, then give me news on whichever is cheaper."
That needs: price(AAPL), price(TSLA), reasoning, then news(...) — multiple rounds.

The loop:
    1. ask the model
    2. did it request tool(s)?
         yes -> run them, add results, GO BACK TO 1
         no  -> it produced a final answer -> DONE
We also add a max_steps guard so a confused model can't loop forever.

Run:
    python step2_agent_loop.py
"""

import os
from dotenv import load_dotenv
from google import genai
from google.genai import types

load_dotenv()
client = genai.Client()
MODEL = "gemini-2.5-flash"


# ---------------------------------------------------------------------------
# TOOLS — plain functions. (Still fake data; could be DB/API calls.)
# ---------------------------------------------------------------------------
def get_stock_price(ticker: str) -> str:
    prices = {"AAPL": 212.45, "TSLA": 178.20, "NVDA": 1043.10, "MSFT": 441.58}
    p = prices.get(ticker.upper())
    return f"{ticker.upper()} is trading at ${p}." if p else f"No price for '{ticker}'."


def get_company_news(ticker: str) -> str:
    news = {
        "AAPL": "Apple unveils new AI features for iPhone; analysts bullish.",
        "TSLA": "Tesla cuts prices in China amid rising EV competition.",
        "NVDA": "Nvidia announces next-gen GPU; demand outpaces supply.",
        "MSFT": "Microsoft expands Copilot across its enterprise suite.",
    }
    return news.get(ticker.upper(), f"No recent news for '{ticker}'.")


# A REGISTRY mapping tool name -> the actual function. The loop uses this to
# dispatch whatever the model asks for. Adding a tool = add a function + schema + entry.
TOOL_FUNCTIONS = {
    "get_stock_price": get_stock_price,
    "get_company_news": get_company_news,
}

# Schemas (what the MODEL sees — the model-facing contract from our last discussion).
TOOLS = types.Tool(
    function_declarations=[
        {
            "name": "get_stock_price",
            "description": "Get the current trading price for a stock ticker symbol.",
            "parameters": {
                "type": "object",
                "properties": {
                    "ticker": {"type": "string", "description": "Ticker, e.g. 'AAPL'."}
                },
                "required": ["ticker"],
            },
        },
        {
            "name": "get_company_news",
            "description": "Get the latest news headline for a company by ticker symbol.",
            "parameters": {
                "type": "object",
                "properties": {
                    "ticker": {"type": "string", "description": "Ticker, e.g. 'TSLA'."}
                },
                "required": ["ticker"],
            },
        },
    ]
)
CONFIG = types.GenerateContentConfig(tools=[TOOLS])


def run_agent(question: str, max_steps: int = 6) -> str:
    """The whole agent. Returns the final text answer."""
    print(f"\n🧑 User: {question}\n")
    contents = [types.Content(role="user", parts=[types.Part(text=question)])]

    for step in range(1, max_steps + 1):
        resp = client.models.generate_content(model=MODEL, contents=contents, config=CONFIG)

        # No tool request => the model is done. Return its answer.
        if not resp.function_calls:
            print(f"✅ Final answer (after {step - 1} tool round(s)):\n{resp.text}\n")
            return resp.text

        # Otherwise: echo the model's turn, run each requested tool, feed results back.
        print(f"--- step {step}: model requested {len(resp.function_calls)} tool call(s) ---")
        contents.append(resp.candidates[0].content)

        for fc in resp.function_calls:
            fn = TOOL_FUNCTIONS.get(fc.name)
            if fn is None:
                result = f"ERROR: unknown tool '{fc.name}'."   # never trust the model blindly
            else:
                try:
                    result = fn(**fc.args)
                except Exception as e:
                    result = f"ERROR running {fc.name}: {e}"    # errors go BACK to the model
            print(f"  🔧 {fc.name}({dict(fc.args)}) -> {result}")
            contents.append(
                types.Content(
                    role="user",
                    parts=[types.Part.from_function_response(name=fc.name, response={"result": result})],
                )
            )
        # loop continues: the model now sees the results and decides the next move

    # Safety net: model kept asking for tools past the limit (likely stuck).
    return "⚠️ Stopped: reached max_steps without a final answer."


if __name__ == "__main__":
    # This one question forces a multi-step plan: 2 price lookups, then reasoning,
    # then a news lookup on the cheaper stock. Watch the steps print out.
    run_agent("Compare the price of Apple and Tesla, then give me the latest news on whichever is cheaper.")
