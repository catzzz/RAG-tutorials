// Command seed builds the SQLite database (data/research.db) from the committed
// data/seed.csv + data/schema.sql. ANYONE-run, offline — this is what makes the repo
// clone-and-run: the real data is already in the CSV, this just loads it into SQLite.
//
//	go run ./cmd/seed
package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	const dbPath = "data/research.db"
	_ = os.Remove(dbPath) // rebuild from scratch each time

	ddl, err := os.ReadFile("data/schema.sql")
	if err != nil {
		fatal(fmt.Errorf("read schema: %w", err))
	}
	csvFile, err := os.Open("data/seed.csv")
	if err != nil {
		fatal(fmt.Errorf("read seed: %w", err))
	}
	defer csvFile.Close()

	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(string(ddl)); err != nil {
		fatal(fmt.Errorf("create schema: %w", err))
	}

	r := csv.NewReader(csvFile)
	header, err := r.Read() // skip header
	if err != nil {
		fatal(fmt.Errorf("read header: %w", err))
	}
	if len(header) != 8 {
		fatal(fmt.Errorf("expected 8 columns, got %d", len(header)))
	}

	tx, _ := db.Begin()
	stmt, err := tx.Prepare(`INSERT INTO financials
		(ticker, period_end, period_type, fiscal_year, revenue, net_income, eps_diluted, net_margin)
		VALUES (?,?,?,?,?,?,?,?)`)
	if err != nil {
		fatal(err)
	}
	n := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fatal(fmt.Errorf("read row: %w", err))
		}
		// columns: ticker, period_end, period_type, fiscal_year, revenue, net_income, eps_diluted, net_margin
		if _, err := stmt.Exec(rec[0], rec[1], rec[2], rec[3], rec[4], rec[5], rec[6], rec[7]); err != nil {
			fatal(fmt.Errorf("insert %v: %w", rec[:4], err))
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "seeded %s with %d rows\n", dbPath, n)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "seed:", err)
	os.Exit(1)
}
