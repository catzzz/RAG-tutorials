"""
Step 1 — Tool-calling "hello world" (Gemini).

GOAL: understand the ONE mechanic that every agent is built on:
    model decides it needs a tool -> we run the tool -> we give the result back -> model answers.

There is no magic in "agents". An agent is just this loop, repeated. Step 2 will
turn the single round-trip below into a loop. For now we do exactly ONE tool call
so you can see every moving part.

The model NEVER runs your code. It only emits a *request* to call a function;
WE execute it and hand the result back. That boundary is the whole game.

Run:
    python step1_tool_calling.py
"""

import os
from dotenv import load_dotenv
from google import genai
from google.genai import types

load_dotenv()  # reads GEMINI_API_KEY from a local .env file

client = genai.Client()  # the SDK automatically picks up GEMINI_API_KEY / GOOGLE_API_KEY

MODEL = "gemini-2.5-flash"  # fast + cheap for dev. We'll add model routing later.


# ---------------------------------------------------------------------------
# 1) THE TOOL (just a normal Python function).
#    Right now it returns fake data. Later this could hit a real stock API.
#    The model NEVER runs this itself — it only *asks* us to run it.
# ---------------------------------------------------------------------------
def get_stock_price(ticker: str) -> str:
    fake_prices = {"AAPL": 212.45, "TSLA": 178.20, "NVDA": 1043.10, "MSFT": 441.58}
    price = fake_prices.get(ticker.upper())
    if price is None:
        return f"No price found for ticker '{ticker}'."
    return f"{ticker.upper()} is trading at ${price}."


# ---------------------------------------------------------------------------
# 2) THE TOOL SCHEMA ("function declaration").
#    This is how we DESCRIBE the tool to the model. The model reads this to
#    decide (a) whether to use the tool and (b) what arguments to pass.
#    A vague description here => the model misuses the tool. Tool design is a skill.
# ---------------------------------------------------------------------------
get_stock_price_declaration = {
    "name": "get_stock_price",
    "description": "Get the current trading price for a stock ticker symbol.",
    "parameters": {
        "type": "object",
        "properties": {
            "ticker": {
                "type": "string",
                "description": "The stock ticker symbol, e.g. 'AAPL' or 'TSLA'.",
            }
        },
        "required": ["ticker"],
    },
}

# Bundle our tool(s) into the config we pass on every request.
TOOLS = types.Tool(function_declarations=[get_stock_price_declaration])
CONFIG = types.GenerateContentConfig(tools=[TOOLS])


def main():
    question = "What is the current price of Apple stock?"
    print(f"\n🧑 User: {question}\n")

    # The conversation is a list of "contents". We append to it as we go.
    contents = [types.Content(role="user", parts=[types.Part(text=question)])]

    # --- TURN 1: ask the model. It will (probably) ask to use the tool. ---
    resp = client.models.generate_content(model=MODEL, contents=contents, config=CONFIG)

    # `resp.function_calls` is the control signal: a non-empty list means
    # "I need you to run these and come back." Empty means "I'm done."
    if resp.function_calls:
        # Save the model's turn (we must echo it back in the next request).
        contents.append(resp.candidates[0].content)

        # Run each requested function and collect the results.
        for fc in resp.function_calls:
            print(f"🤖 Gemini wants to call: {fc.name}({dict(fc.args)})")
            result = get_stock_price(**fc.args)        # <-- WE run the tool
            print(f"🔧 Tool returned: {result}")

            # Hand the result back as a function_response part.
            contents.append(
                types.Content(
                    role="user",
                    parts=[
                        types.Part.from_function_response(
                            name=fc.name,
                            response={"result": result},
                        )
                    ],
                )
            )

        # --- TURN 2: model now has the data and writes the final answer. ---
        final = client.models.generate_content(model=MODEL, contents=contents, config=CONFIG)
        print(f"\n✅ Gemini: {final.text}\n")
    else:
        # The model answered directly without needing a tool.
        print(f"\n✅ Gemini (no tool needed): {resp.text}\n")


if __name__ == "__main__":
    main()
