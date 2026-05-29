CREATE TABLE financials (
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
