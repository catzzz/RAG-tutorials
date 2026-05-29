// Command query is a small debug harness for the M2 data layer: it opens the read-only
// store and runs a SQL string through the query_db tool, exactly as the agent will in M5.
//
//	go run ./cmd/query "SELECT ticker, period_end, net_margin FROM financials WHERE ticker='TSLA' AND period_type='Q' ORDER BY period_end DESC LIMIT 4"
//	go run ./cmd/query --schema
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/catzzz/RAG-tutorials/fin-research-agent/config"
	"github.com/catzzz/RAG-tutorials/fin-research-agent/internal/store"
	"github.com/catzzz/RAG-tutorials/fin-research-agent/internal/tools"
)

func main() {
	cfg := config.Load(".env")
	st, err := store.OpenSQLite(cfg.DBPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open store:", err)
		os.Exit(1)
	}
	defer st.Close()

	if len(os.Args) > 1 && os.Args[1] == "--schema" {
		txt, err := st.SchemaText()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Print(txt)
		return
	}
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, `usage: query "<SELECT ...>"  |  query --schema`)
		os.Exit(2)
	}

	tool := &tools.QueryDB{Store: st}
	fmt.Println(tool.Run(strings.Join(os.Args[1:], " ")))
}
