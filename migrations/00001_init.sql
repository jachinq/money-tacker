-- +goose Up

CREATE TABLE user (
  id            INTEGER PRIMARY KEY,
  account       TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT NOT NULL,
  created_at    TEXT NOT NULL
);

CREATE TABLE session (
  token      TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES user(id),
  expires_at TEXT NOT NULL
);

CREATE TABLE product (
  code           TEXT PRIMARY KEY,
  name           TEXT NOT NULL,
  issuer         TEXT,
  last_seen_page INTEGER,
  last_seen_at   TEXT,
  listed         INTEGER NOT NULL DEFAULT 1,
  updated_at     TEXT NOT NULL
);

CREATE TABLE nav_snapshot (
  product_code      TEXT NOT NULL REFERENCES product(code),
  nav_date          TEXT NOT NULL,
  unit_nav          INTEGER,
  acc_nav           INTEGER,
  daily_return_bp   INTEGER,
  extra_json        TEXT,
  fetched_at        TEXT NOT NULL,
  PRIMARY KEY (product_code, nav_date)
);

CREATE TABLE crawl_run (
  id            INTEGER PRIMARY KEY,
  started_at    TEXT NOT NULL,
  finished_at   TEXT,
  status        TEXT NOT NULL,
  pages_ok      INTEGER NOT NULL DEFAULT 0,
  products_ok   INTEGER NOT NULL DEFAULT 0,
  error_summary TEXT
);

CREATE TABLE nav_observation (
  crawl_run_id INTEGER NOT NULL REFERENCES crawl_run(id),
  product_code TEXT NOT NULL REFERENCES product(code),
  nav_date     TEXT NOT NULL,
  unit_nav     INTEGER,
  PRIMARY KEY (crawl_run_id, product_code)
);

CREATE INDEX idx_obs_code_date ON nav_observation(product_code, nav_date);

CREATE TABLE holding (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES user(id),
  product_code TEXT NOT NULL REFERENCES product(code),
  created_at   TEXT NOT NULL,
  UNIQUE (user_id, product_code)
);

CREATE TABLE ledger_entry (
  id             INTEGER PRIMARY KEY,
  holding_id     INTEGER NOT NULL REFERENCES holding(id) ON DELETE CASCADE,
  kind           TEXT NOT NULL,
  occur_date     TEXT NOT NULL,
  cash_fen       INTEGER NOT NULL,
  shares_e8      INTEGER NOT NULL,
  unit_nav_e8    INTEGER NOT NULL,
  nav_date_used  TEXT NOT NULL,
  voided_at      TEXT,
  note           TEXT,
  created_at     TEXT NOT NULL
);

CREATE INDEX idx_ledger_holding ON ledger_entry(holding_id, occur_date, id);

-- +goose Down

DROP TABLE ledger_entry;
DROP TABLE holding;
DROP TABLE nav_observation;
DROP TABLE crawl_run;
DROP TABLE nav_snapshot;
DROP TABLE product;
DROP TABLE session;
DROP TABLE user;
