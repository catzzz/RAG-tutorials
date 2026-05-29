// Package tools holds the agent's callable tools. query_db is the first: a text-to-SQL
// tool the agent uses for numeric/aggregate questions.
//
// Design (locked): the AGENT writes the SQL inline (the schema is in its system prompt),
// and this tool just guards + runs it. That's one CLI call per step, a single visible
// trace, and self-correction lives in the main loop (a SQL error comes back as the
// observation, and the agent rewrites the query next turn).
package tools

import (
	"fmt"
	"strings"

	"github.com/catzzz/RAG-tutorials/fin-research-agent/internal/store"
)

// QueryDB wraps a read-only Store as the query_db tool.
type QueryDB struct {
	Store store.Store
}

// Name and Description feed the agent's tool registry (M5). The description is the
// routing signal — it tells the model WHEN to reach for SQL vs the other tools.
func (q *QueryDB) Name() string { return "query_db" }

func (q *QueryDB) Description() string {
	return "Run a single read-only SQL SELECT over the financials database for numeric or " +
		"aggregate questions (revenue, net income, EPS, margins, trends across quarters/years). " +
		"Input: a SQL SELECT string. Use this for anything that needs counting, summing, " +
		"averaging, filtering, or comparing numbers across periods."
}

// Run executes the SQL and returns a markdown table (or a clear error string the agent
// can read and correct on the next turn).
func (q *QueryDB) Run(sql string) string {
	res, err := q.Store.Query(sql)
	if err != nil {
		return "SQL error: " + err.Error() + "\nFix the query and try again."
	}
	if len(res.Rows) == 0 {
		return "Query ran successfully but returned 0 rows."
	}
	return markdownTable(res)
}

func markdownTable(r *store.Result) string {
	var b strings.Builder
	b.WriteString("| " + strings.Join(r.Columns, " | ") + " |\n")
	b.WriteString("|" + strings.Repeat(" --- |", len(r.Columns)) + "\n")
	limit := len(r.Rows)
	if limit > 100 { // keep tool output bounded for the prompt
		limit = 100
	}
	for _, row := range r.Rows[:limit] {
		b.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	if len(r.Rows) > limit {
		b.WriteString(fmt.Sprintf("\n(%d more rows truncated)\n", len(r.Rows)-limit))
	}
	return b.String()
}
