// Package config loads runtime settings from a .env file (and the environment),
// keeping paths and model names out of the code so they're swappable.
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds everything the agent needs to run. Defaults are filled in by Load
// so the program works even with an empty .env.
type Config struct {
	Model      string        // Gemini CLI model name (the -m flag)
	DBPath     string        // SQLite file (used from M2 on)
	MaxSteps   int           // ReAct loop guard (used from M5 on)
	TraceDir   string        // where JSONL traces are written
	OllamaURL  string        // local Ollama endpoint (used from M3 on)
	EmbedModel string        // embedding model name (used from M3 on)
	LLMTimeout time.Duration // per Gemini CLI call timeout
}

// Load reads key=value pairs from .env (if present) into the process environment,
// then builds a Config with sensible defaults. .env never overrides a value that
// is already set in the real environment.
func Load(envPath string) Config {
	loadDotEnv(envPath)
	return Config{
		Model:      getenv("MODEL", "gemini-2.5-flash"),
		DBPath:     getenv("DB_PATH", "./data/research.db"),
		MaxSteps:   getenvInt("MAX_STEPS", 8),
		TraceDir:   getenv("TRACE_DIR", "./traces"),
		OllamaURL:  getenv("OLLAMA_URL", "http://localhost:11434"),
		EmbedModel: getenv("EMBED_MODEL", "nomic-embed-text"),
		LLMTimeout: time.Duration(getenvInt("LLM_TIMEOUT_SEC", 120)) * time.Second,
	}
}

// loadDotEnv parses a minimal KEY=VALUE .env file. Missing file is not an error.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // no .env is fine — defaults + real env take over
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`) // strip optional quotes
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return fallback
}
