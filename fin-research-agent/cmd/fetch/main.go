// Command fetch pulls real fundamentals from SEC EDGAR and writes a committed seed
// (data/seed.csv) plus the table DDL (data/schema.sql). MAINTAINER-run, occasionally —
// cloners don't need it; they run `cmd/seed` against the committed CSV.
//
//	go run ./cmd/fetch                 # default tickers, last ~3 years
//	go run ./cmd/fetch AAPL NVDA TSLA  # custom tickers
package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/catzzz/RAG-tutorials/fin-research-agent/internal/edgar"
)

var defaultTickers = []string{"AAPL", "MSFT", "NVDA", "TSLA"}

const schemaDDL = `CREATE TABLE financials (
  ticker        TEXT    NOT NULL,            -- stock symbol, e.g. 'AAPL'
  period_end    TEXT    NOT NULL,            -- period end date, YYYY-MM-DD (order by this)
  period_type   TEXT    NOT NULL,            -- 'Q' (quarter) or 'FY' (full year)
  fiscal_year   INTEGER NOT NULL,            -- calendar year the period ends
  revenue       INTEGER,                     -- total revenue, USD
  net_income    INTEGER,                     -- net income, USD
  eps_diluted   REAL,                        -- diluted earnings per share, USD
  net_margin    REAL,                        -- net_income / revenue (0..1)
  PRIMARY KEY (ticker, period_end, period_type)
);
`

func main() {
	tickers := defaultTickers
	if len(os.Args) > 1 {
		tickers = os.Args[1:]
	}
	sinceYear := time.Now().Year() - 3

	c := edgar.New()
	fmt.Fprintln(os.Stderr, "loading ticker→CIK directory…")
	if err := c.LoadTickers(); err != nil {
		fatal(err)
	}

	if err := os.MkdirAll("data", 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile("data/schema.sql", []byte(schemaDDL), 0o644); err != nil {
		fatal(err)
	}

	f, err := os.Create("data/seed.csv")
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	w.Write([]string{"ticker", "period_end", "period_type", "fiscal_year", "revenue", "net_income", "eps_diluted", "net_margin"})

	total := 0
	for _, t := range tickers {
		rows, err := c.Fetch(t, sinceYear)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", t, err)
			continue
		}
		for _, r := range rows {
			w.Write([]string{
				r.Ticker,
				r.PeriodEnd,
				r.PeriodType,
				strconv.Itoa(r.FiscalYear),
				strconv.FormatInt(r.Revenue, 10),
				strconv.FormatInt(r.NetIncome, 10),
				strconv.FormatFloat(r.EPSDiluted, 'f', 4, 64),
				strconv.FormatFloat(r.NetMargin, 'f', 4, 64),
			})
		}
		fmt.Fprintf(os.Stderr, "  %s: %d periods\n", t, len(rows))
		total += len(rows)
		time.Sleep(300 * time.Millisecond) // be polite to SEC
	}
	fmt.Fprintf(os.Stderr, "wrote data/seed.csv (%d rows) + data/schema.sql\n", total)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "fetch:", err)
	os.Exit(1)
}
