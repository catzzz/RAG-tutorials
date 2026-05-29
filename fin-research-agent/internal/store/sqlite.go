package store

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no cgo → single static binary)
)

// SQLite is a read-only Store over a SQLite file.
type SQLite struct {
	db *sql.DB
}

// OpenSQLite opens path read-only. Two layers of protection against a malicious or
// buggy LLM-generated query: (1) the connection itself is mode=ro (the real guard),
// and (2) Query() validates the statement is a lone SELECT (clean errors + blocks
// PRAGMA/ATTACH that mode=ro alone would still allow).
func OpenSQLite(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("open %s read-only: %w", path, err)
	}
	return &SQLite{db: db}, nil
}

var selectOnly = regexp.MustCompile(`(?is)^\s*(select|with)\b`)
var forbidden = regexp.MustCompile(`(?is)\b(insert|update|delete|drop|alter|create|replace|attach|detach|pragma|vacuum|reindex)\b`)

// validate enforces that sql is a single read-only SELECT/CTE.
func validate(query string) error {
	q := strings.TrimSpace(query)
	q = strings.TrimSuffix(q, ";") // allow one trailing semicolon
	if strings.Contains(q, ";") {
		return fmt.Errorf("only a single statement is allowed")
	}
	if !selectOnly.MatchString(q) {
		return fmt.Errorf("only SELECT/WITH queries are allowed")
	}
	if forbidden.MatchString(q) {
		return fmt.Errorf("query contains a forbidden keyword (writes/DDL/PRAGMA are not allowed)")
	}
	return nil
}

// Query runs a validated read-only SELECT.
func (s *SQLite) Query(query string) (*Result, error) {
	if err := validate(query); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	res := &Result{Columns: cols}
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		out := make([]string, len(cols))
		for i, c := range cells {
			out[i] = render(c)
		}
		res.Rows = append(res.Rows, out)
	}
	return res, rows.Err()
}

// SchemaText returns the CREATE TABLE statements — compact, accurate schema for the
// system prompt. Runs internally (not through the agent-facing validate()).
func (s *SQLite) SchemaText() (string, error) {
	rows, err := s.db.Query("SELECT sql FROM sqlite_master WHERE type='table' AND sql IS NOT NULL")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var ddl string
		if err := rows.Scan(&ddl); err != nil {
			return "", err
		}
		b.WriteString(strings.TrimSpace(ddl))
		b.WriteString(";\n")
	}
	return b.String(), rows.Err()
}

func (s *SQLite) Close() error { return s.db.Close() }

func render(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return string(x)
	default:
		return fmt.Sprintf("%v", x)
	}
}
