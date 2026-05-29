// Command fin-research-agent is a multi-tool financial research agent.
//
// M1 milestone: this only proves the backbone — read a question, make one Gemini CLI
// call, trace it, print the answer. The ReAct loop and tools (SQL, RAG, price) arrive
// in later milestones.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/catzzz/RAG-tutorials/fin-research-agent/config"
	"github.com/catzzz/RAG-tutorials/fin-research-agent/internal/llm"
	"github.com/catzzz/RAG-tutorials/fin-research-agent/internal/trace"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, `usage: fin-research-agent "<your research question>"`)
		os.Exit(2)
	}
	question := strings.Join(os.Args[1:], " ")

	cfg := config.Load(".env")

	tracer, err := trace.New(cfg.TraceDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "trace init:", err)
		os.Exit(1)
	}
	defer tracer.Close()

	client := llm.New(cfg.Model, cfg.LLMTimeout)

	resp, err := client.Generate(context.Background(), question, "")
	tracer.Emit("llm_call", map[string]any{
		"model":         cfg.Model,
		"prompt":        trace.Preview(question, 200),
		"output":        trace.Preview(resp.Content, 200),
		"input_tokens":  resp.InputTokens,
		"output_tokens": resp.OutputTokens,
		"latency_ms":    resp.LatencyMs,
		"error":         errString(err),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "generate:", err)
		os.Exit(1)
	}

	fmt.Println(resp.Content)
	fmt.Fprintf(os.Stderr, "\n[trace %s · in=%d out=%d · %dms]\n",
		tracer.RunID, resp.InputTokens, resp.OutputTokens, resp.LatencyMs)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
