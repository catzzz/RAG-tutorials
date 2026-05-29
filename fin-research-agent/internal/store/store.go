// Package store is the agent's data layer behind a small interface, so the backing
// database is swappable. Today there's one implementation (read-only SQLite over the
// committed seed); a live read-only Postgres impl (pointed at a real market-data
// warehouse) can be added later without the agent loop or the query_db tool changing.
package store

// Result is a generic query result: column names + string-rendered rows.
// Stringly-typed on purpose — it's headed straight into an LLM prompt as text.
type Result struct {
	Columns []string
	Rows    [][]string
}

// Store is a read-only, query-only view of structured data.
type Store interface {
	// Query runs a single read-only SELECT and returns rows. It MUST reject anything
	// that isn't a lone SELECT/CTE so an LLM-generated string can't mutate data.
	Query(sql string) (*Result, error)
	// SchemaText returns the schema as text to ground text-to-SQL in the system prompt.
	SchemaText() (string, error)
	Close() error
}
