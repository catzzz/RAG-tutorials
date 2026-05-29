// Package llm wraps the local `gemini` CLI as the agent's text-generation backend.
//
// Why the CLI instead of the API: it reuses the user's already-authenticated gemini
// session, so generation is free and needs no API key. The trade-offs are handled here:
//   - the CLI has no --system-prompt flag, so we inline the system prompt with markers;
//   - large prompts are piped over stdin to dodge the OS argv length limit (~262KB on macOS);
//   - we parse `--output-format json` to recover token counts for the tracer.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Response is one generation result, with the token/latency stats the tracer records.
type Response struct {
	Content      string
	InputTokens  int
	OutputTokens int
	LatencyMs    int64 // model-reported latency if available, else wall-clock
}

// Client drives the gemini CLI.
type Client struct {
	Binary  string        // CLI binary name/path (default "gemini")
	Model   string        // -m flag
	Timeout time.Duration // hard cap per call
}

// New builds a Client with defaults applied.
func New(model string, timeout time.Duration) *Client {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &Client{Binary: "gemini", Model: model, Timeout: timeout}
}

// argLimit: prompts longer than this are sent via stdin instead of as an argv value.
const argLimit = 100_000

// geminiEnvelope mirrors the JSON the gemini CLI emits with --output-format json.
// Tokens/latency are reported per underlying model (a request may use a router model
// plus the main model), so we sum across all of them.
type geminiEnvelope struct {
	Response string `json:"response"`
	Stats    struct {
		Models map[string]struct {
			API struct {
				TotalLatencyMs int64 `json:"totalLatencyMs"`
			} `json:"api"`
			Tokens struct {
				Input      int `json:"input"`
				Candidates int `json:"candidates"`
			} `json:"tokens"`
		} `json:"models"`
	} `json:"stats"`
}

// Generate runs one prompt through the CLI. system is optional; when set it's inlined
// with loud markers because the CLI can't take a separate system prompt.
func (c *Client) Generate(ctx context.Context, prompt, system string) (Response, error) {
	if system != "" {
		prompt = "=== SYSTEM INSTRUCTIONS (follow these exactly) ===\n\n" +
			system +
			"\n\n=== END SYSTEM INSTRUCTIONS ===\n\n=== USER REQUEST ===\n\n" + prompt
	}

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	useStdin := len(prompt) > argLimit
	// --skip-trust: run headless without prompting to "trust" the working directory.
	// We only use the CLI for `-p` generation, so we don't need it to load any
	// workspace/project config — skipping trust keeps the call clean and scriptable.
	args := []string{"-m", c.Model, "--output-format", "json", "--skip-trust"}
	if useStdin {
		args = append(args, "-p", "-") // CLI reads the prompt from stdin
	} else {
		args = append(args, "-p", prompt)
	}

	cmd := exec.CommandContext(ctx, c.Binary, args...)
	if useStdin {
		cmd.Stdin = strings.NewReader(prompt)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	start := time.Now()
	err := cmd.Run()
	wallMs := time.Since(start).Milliseconds()
	if err != nil {
		return Response{}, fmt.Errorf("gemini CLI failed: %v: %s", err, cleanStderr(stderr.String()))
	}

	// The CLI may print noise (e.g. "Loaded cached credentials.") before the JSON.
	// Skip everything up to the first line that starts a JSON object.
	jsonStr := extractJSON(stdout.String())

	var env geminiEnvelope
	if err := json.Unmarshal([]byte(jsonStr), &env); err != nil {
		// Fallback: not JSON (e.g. CLI changed format) — return the raw text.
		return Response{Content: strings.TrimSpace(stdout.String()), LatencyMs: wallMs}, nil
	}

	var in, out int
	var apiMs int64
	for _, m := range env.Stats.Models {
		in += m.Tokens.Input
		out += m.Tokens.Candidates
		apiMs += m.API.TotalLatencyMs
	}
	if apiMs == 0 {
		apiMs = wallMs
	}
	return Response{Content: env.Response, InputTokens: in, OutputTokens: out, LatencyMs: apiMs}, nil
}

// extractJSON returns the substring starting at the first line that begins with '{'.
func extractJSON(out string) string {
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "{") {
			return strings.Join(lines[i:], "\n")
		}
	}
	return out
}

// cleanStderr drops the known-noisy "Loaded cached credentials." line.
func cleanStderr(s string) string {
	var keep []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "Loaded cached credentials") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "; ")
}
