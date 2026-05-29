// Package edgar fetches real company fundamentals from the SEC EDGAR XBRL API.
//
// Why EDGAR: it's the authoritative source for filed financials (revenue, net income,
// EPS), it's keyless and free (only a descriptive User-Agent is required), and it keeps
// the project all-Go and reproducible — we fetch once into a committed seed, so anyone
// cloning the repo can rebuild the same SQLite DB.
//
// We use the *companyfacts* endpoint (one call per company returns every concept) rather
// than per-concept *companyconcept* calls: it's fewer requests AND it dodges a real quirk
// where companyconcept returned 0 points for some tickers (e.g. NVDA) whose data is
// present in companyfacts.
//
// Real-world messiness handled here:
//   - companies tag revenue differently across eras → we try several concept names;
//   - EDGAR returns quarterly, year-to-date, AND annual durations mixed together →
//     we bucket by period length (≈quarter vs ≈year) and drop the cumulative ones;
//   - the same period is re-reported in later/amended filings → we keep the latest-filed;
//   - EDGAR's fy/fp tag a datapoint with the *filing's* fiscal year, so we derive the
//     period instead from its END date + duration (the reliable source of truth).
package edgar

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// SEC requires a descriptive User-Agent identifying the caller.
const userAgent = "fin-research-agent educational portfolio (chuneleu@gmail.com)"

// Revenue is tagged inconsistently across filers/eras; try these in order.
var revenueConcepts = []string{
	"RevenueFromContractWithCustomerExcludingAssessedTax",
	"Revenues",
	"SalesRevenueNet",
}

// FinancialRow is one assembled period for one company. fiscal_year/type are derived
// from the period-END date and its duration, NOT from EDGAR's (unreliable) fy/fp fields.
type FinancialRow struct {
	Ticker     string
	PeriodEnd  string // YYYY-MM-DD (the period's end date — the source of truth)
	PeriodType string // "Q" (quarter) or "FY" (full year)
	FiscalYear int    // calendar year the period ends
	Revenue    int64  // USD
	NetIncome  int64  // USD
	EPSDiluted float64
	NetMargin  float64 // NetIncome / Revenue
}

// Client talks to EDGAR and caches the ticker→CIK map.
type Client struct {
	http        *http.Client
	cikByTicker map[string]int
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: 60 * time.Second}}
}

func (c *Client) getJSON(url string, v any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent) // do NOT set Accept-Encoding: let Go auto-gzip
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// LoadTickers fetches the public ticker→CIK directory. Required before Fetch.
func (c *Client) LoadTickers() error {
	var raw map[string]struct {
		CIK    int    `json:"cik_str"`
		Ticker string `json:"ticker"`
	}
	if err := c.getJSON("https://www.sec.gov/files/company_tickers.json", &raw); err != nil {
		return fmt.Errorf("load tickers: %w", err)
	}
	c.cikByTicker = make(map[string]int, len(raw))
	for _, e := range raw {
		c.cikByTicker[strings.ToUpper(e.Ticker)] = e.CIK
	}
	return nil
}

// factsResponse is the shape of /companyfacts/CIK##########.json
type factsResponse struct {
	Facts struct {
		GAAP map[string]conceptData `json:"us-gaap"`
	} `json:"facts"`
}

type conceptData struct {
	Units map[string][]rawPoint `json:"units"`
}

type rawPoint struct {
	Start string  `json:"start"`
	End   string  `json:"end"`
	Val   float64 `json:"val"`
	Form  string  `json:"form"`
	Filed string  `json:"filed"`
}

// point is one normalized data point with its period length in days.
type point struct {
	end   string
	days  int
	val   float64
	filed string
}

// periodType classifies a point by its duration. "" means "ignore" (cumulative YTD, etc.).
func periodType(days int) string {
	switch {
	case days >= 80 && days <= 100:
		return "Q"
	case days >= 350 && days <= 380:
		return "FY"
	default:
		return ""
	}
}

func (c *Client) fetchFacts(cik int) (map[string]conceptData, error) {
	url := fmt.Sprintf("https://data.sec.gov/api/xbrl/companyfacts/CIK%010d.json", cik)
	var resp factsResponse
	if err := c.getJSON(url, &resp); err != nil {
		return nil, err
	}
	return resp.Facts.GAAP, nil
}

// conceptPoints buckets one concept's Q/FY points by "<end>|<type>", keeping the
// latest-filed value for each period.
func conceptPoints(gaap map[string]conceptData, concept string) map[string]point {
	cd, ok := gaap[concept]
	if !ok {
		return nil
	}
	var rows []rawPoint
	if u, ok := cd.Units["USD"]; ok {
		rows = u
	} else {
		for _, u := range cd.Units { // e.g. "USD/shares" for EPS
			rows = u
			break
		}
	}
	out := make(map[string]point)
	for _, r := range rows {
		if r.Start == "" || r.End == "" {
			continue // instantaneous (balance-sheet) item — not a flow we want
		}
		days := dayDiff(r.Start, r.End)
		if periodType(days) == "" {
			continue
		}
		key := r.End + "|" + periodType(days)
		if prev, ok := out[key]; ok && prev.filed >= r.Filed {
			continue
		}
		out[key] = point{end: r.End, days: days, val: r.Val, filed: r.Filed}
	}
	return out
}

// Fetch assembles revenue + net income + EPS for one ticker into FinancialRows.
// sinceYear keeps the seed small (only periods ending in/after that calendar year).
func (c *Client) Fetch(ticker string, sinceYear int) ([]FinancialRow, error) {
	cik, ok := c.cikByTicker[strings.ToUpper(ticker)]
	if !ok {
		return nil, fmt.Errorf("unknown ticker %q (LoadTickers first?)", ticker)
	}
	gaap, err := c.fetchFacts(cik)
	if err != nil {
		return nil, fmt.Errorf("%s facts: %w", ticker, err)
	}

	// Pick the revenue concept with the MOST in-range periods, not just the first
	// non-empty one: some filers (e.g. NVDA) carry old data under one tag
	// (RevenueFromContract…) but report recent revenue under another (Revenues), so
	// "first non-empty" can select a tag whose points are all filtered out by sinceYear.
	var revenue map[string]point
	best := 0
	for _, concept := range revenueConcepts {
		m := conceptPoints(gaap, concept)
		cnt := 0
		for _, p := range m {
			if yearOf(p.end) >= sinceYear {
				cnt++
			}
		}
		if cnt > best {
			best, revenue = cnt, m
		}
	}
	if len(revenue) == 0 {
		return nil, fmt.Errorf("%s: no revenue concept with recent data in facts", ticker)
	}
	netIncome := conceptPoints(gaap, "NetIncomeLoss")
	eps := conceptPoints(gaap, "EarningsPerShareDiluted")

	var rows []FinancialRow
	for key, rev := range revenue {
		if yearOf(rev.end) < sinceYear {
			continue
		}
		ni, hasNI := netIncome[key]
		ep := eps[key] // EPS may be absent for some periods; zero is acceptable
		row := FinancialRow{
			Ticker:     strings.ToUpper(ticker),
			PeriodEnd:  rev.end,
			PeriodType: periodType(rev.days),
			FiscalYear: yearOf(rev.end),
			Revenue:    int64(rev.val),
			EPSDiluted: ep.val,
		}
		if hasNI {
			row.NetIncome = int64(ni.val)
			if rev.val != 0 {
				row.NetMargin = ni.val / rev.val
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].PeriodEnd < rows[j].PeriodEnd })
	return rows, nil
}

func dayDiff(start, end string) int {
	const layout = "2006-01-02"
	s, err1 := time.Parse(layout, start)
	e, err2 := time.Parse(layout, end)
	if err1 != nil || err2 != nil {
		return -1
	}
	return int(e.Sub(s).Hours() / 24)
}

func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	var y int
	fmt.Sscanf(date[:4], "%d", &y)
	return y
}
