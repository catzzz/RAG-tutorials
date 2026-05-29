// Package trace records one JSONL event per agent step, so a non-deterministic
// multi-step run can be inspected after the fact. It's wired in from M1 ("trace-first")
// because you can't debug an agent loop you can't see.
package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Event is one line in the trace file. Fields carries step-specific data
// (token counts, previews, tool names, ...) so the schema can grow without churn.
type Event struct {
	RunID  string         `json:"run_id"`
	Time   string         `json:"time"`
	Type   string         `json:"type"` // e.g. "llm_call", "tool_call", "final"
	Fields map[string]any `json:"fields,omitempty"`
}

// Tracer appends events to traces/<run_id>.jsonl.
type Tracer struct {
	RunID string
	f     *os.File
}

// New creates a tracer writing to dir/<run_id>.jsonl, creating dir if needed.
func New(dir string) (*Tracer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create trace dir: %w", err)
	}
	runID := "run_" + time.Now().Format("20060102_150405")
	path := filepath.Join(dir, runID+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open trace file: %w", err)
	}
	return &Tracer{RunID: runID, f: f}, nil
}

// Emit writes one event. Errors are intentionally swallowed: tracing must never
// break the agent run.
func (t *Tracer) Emit(eventType string, fields map[string]any) {
	if t == nil || t.f == nil {
		return
	}
	ev := Event{
		RunID:  t.RunID,
		Time:   time.Now().Format(time.RFC3339),
		Type:   eventType,
		Fields: fields,
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	t.f.Write(append(b, '\n'))
}

// Close flushes and closes the trace file.
func (t *Tracer) Close() error {
	if t == nil || t.f == nil {
		return nil
	}
	return t.f.Close()
}

// Preview trims s to n runes for compact logging of prompts/outputs.
func Preview(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
