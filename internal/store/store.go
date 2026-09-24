package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	DB *sql.DB
}

type User struct {
	ID           int64
	Account      string
	PasswordHash string
}

type Product struct {
	Code         string
	Name         string
	Issuer       string
	LastSeenPage int
	Listed       bool
}

type NavSnap struct {
	ProductCode   string
	NavDate       string
	UnitNavE8     int64
	AccNavE8      sql.NullInt64
	DailyReturnBP sql.NullInt64
	FetchedAt     string
}

type Holding struct {
	ID          int64
	UserID      int64
	ProductCode string
	CreatedAt   string
}

type Ledger struct {
	ID          int64
	HoldingID   int64
	Kind        string
	OccurDate   string
	CashFen     int64
	SharesE8    int64
	UnitNavE8   int64
	NavDateUsed string
	VoidedAt    sql.NullString
	Note        sql.NullString
	CreatedAt   string
}

type CrawlRun struct {
	ID           int64
	StartedAt    string
	FinishedAt   sql.NullString
	Status       string
	PagesOK      int
	ProductsOK   int
	ErrorSummary sql.NullString
}

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM user`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(account, password string, cost int, now time.Time) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return nil, err
	}
	res, err := s.DB.Exec(`INSERT INTO user(account, password_hash, created_at) VALUES(?,?,?)`,
		account, string(hash), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &User{ID: id, Account: account, PasswordHash: string(hash)}, nil
}

func (s *Store) UserByAccount(account string) (*User, error) {
	u := &User{}
	err := s.DB.QueryRow(`SELECT id, account, password_hash FROM user WHERE account = ?`, account).
		Scan(&u.ID, &u.Account, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func (s *Store) UserByID(id int64) (*User, error) {
	u := &User{}
	err := s.DB.QueryRow(`SELECT id, account, password_hash FROM user WHERE id = ?`, id).
		Scan(&u.ID, &u.Account, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func (s *Store) UpdatePassword(id int64, password string, cost int) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`UPDATE user SET password_hash=? WHERE id=?`, string(hash), id)
	return err
}

func (s *Store) CreateSession(userID int64, ttl time.Duration, now time.Time) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(b[:])
	exp := now.Add(ttl).Format(time.RFC3339)
	_, err := s.DB.Exec(`INSERT INTO session(token, user_id, expires_at) VALUES(?,?,?)`, tok, userID, exp)
	return tok, err
}

func (s *Store) UserBySession(token string, now time.Time) (*User, error) {
	u := &User{}
	var exp string
	err := s.DB.QueryRow(`
		SELECT u.id, u.account, u.password_hash, s.expires_at
		FROM session s JOIN user u ON u.id = s.user_id
		WHERE s.token = ?`, token).Scan(&u.ID, &u.Account, &u.PasswordHash, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339, exp)
	if err != nil || !t.After(now) {
		return nil, ErrNotFound
	}
	return u, nil
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.DB.Exec(`DELETE FROM session WHERE token=?`, token)
	return err
}

func (s *Store) Product(code string) (*Product, error) {
	p := &Product{}
	var listed int
	var issuer sql.NullString
	var page sql.NullInt64
	err := s.DB.QueryRow(`SELECT code, name, issuer, last_seen_page, listed FROM product WHERE code=?`, code).
		Scan(&p.Code, &p.Name, &issuer, &page, &listed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	p.Issuer = issuer.String
	p.LastSeenPage = int(page.Int64)
	p.Listed = listed == 1
	return p, err
}

func (s *Store) ListProducts() ([]Product, error) {
	rows, err := s.DB.Query(`SELECT code, name, issuer, last_seen_page, listed FROM product`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Product
	for rows.Next() {
		p := Product{}
		var listed int
		var issuer sql.NullString
		var page sql.NullInt64
		if err := rows.Scan(&p.Code, &p.Name, &issuer, &page, &listed); err != nil {
			return nil, err
		}
		p.Issuer = issuer.String
		p.LastSeenPage = int(page.Int64)
		p.Listed = listed == 1
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpsertProduct(code, name, issuer string, page int, now time.Time) error {
	_, err := s.DB.Exec(`
		INSERT INTO product(code, name, issuer, last_seen_page, last_seen_at, listed, updated_at)
		VALUES(?,?,?,?,?,1,?)
		ON CONFLICT(code) DO UPDATE SET
			name=excluded.name,
			issuer=COALESCE(excluded.issuer, product.issuer),
			last_seen_page=excluded.last_seen_page,
			last_seen_at=excluded.last_seen_at,
			listed=1,
			updated_at=excluded.updated_at
	`, code, name, nullStr(issuer), page, now.Format(time.RFC3339), now.Format(time.RFC3339))
	return err
}

func (s *Store) MarkMissingUnlisted(seen map[string]struct{}, now time.Time) error {
	rows, err := s.DB.Query(`SELECT code FROM product WHERE listed=1`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var missing []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return err
		}
		if _, ok := seen[code]; !ok {
			missing = append(missing, code)
		}
	}
	for _, code := range missing {
		if _, err := s.DB.Exec(`UPDATE product SET listed=0, updated_at=? WHERE code=?`, now.Format(time.RFC3339), code); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertSnapshot(code, navDate string, unit, acc, bp int64, extra, fetched string) error {
	_, err := s.DB.Exec(`
		INSERT INTO nav_snapshot(product_code, nav_date, unit_nav, acc_nav, daily_return_bp, extra_json, fetched_at)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(product_code, nav_date) DO UPDATE SET
			unit_nav=excluded.unit_nav,
			acc_nav=excluded.acc_nav,
			daily_return_bp=excluded.daily_return_bp,
			extra_json=excluded.extra_json,
			fetched_at=excluded.fetched_at
	`, code, navDate, unit, acc, bp, extra, fetched)
	return err
}

func (s *Store) ListNav(code string) ([]NavSnap, error) {
	rows, err := s.DB.Query(`
		SELECT product_code, nav_date, unit_nav, acc_nav, daily_return_bp, fetched_at
		FROM nav_snapshot WHERE product_code=? ORDER BY nav_date ASC`, code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NavSnap
	for rows.Next() {
		var n NavSnap
		if err := rows.Scan(&n.ProductCode, &n.NavDate, &n.UnitNavE8, &n.AccNavE8, &n.DailyReturnBP, &n.FetchedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) ListAllNav() ([]NavSnap, error) {
	rows, err := s.DB.Query(`
		SELECT product_code, nav_date, unit_nav, acc_nav, daily_return_bp, fetched_at
		FROM nav_snapshot ORDER BY product_code ASC, nav_date ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NavSnap
	for rows.Next() {
		var n NavSnap
		if err := rows.Scan(&n.ProductCode, &n.NavDate, &n.UnitNavE8, &n.AccNavE8, &n.DailyReturnBP, &n.FetchedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) StartCrawl(now time.Time) (int64, error) {
	res, err := s.DB.Exec(`INSERT INTO crawl_run(started_at, status) VALUES(?, 'running')`, now.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FinishCrawl(id int64, status string, pages, products int, errSum string, now time.Time) error {
	_, err := s.DB.Exec(`UPDATE crawl_run SET finished_at=?, status=?, pages_ok=?, products_ok=?, error_summary=? WHERE id=?`,
		now.Format(time.RFC3339), status, pages, products, nullStr(errSum), id)
	return err
}

func (s *Store) InsertObservation(runID int64, code, navDate string, unit int64) error {
	_, err := s.DB.Exec(`INSERT INTO nav_observation(crawl_run_id, product_code, nav_date, unit_nav) VALUES(?,?,?,?)`,
		runID, code, navDate, unit)
	return err
}

func (s *Store) UpdateObservation(runID int64, code, navDate string, unit int64) error {
	_, err := s.DB.Exec(`UPDATE nav_observation SET nav_date=?, unit_nav=? WHERE crawl_run_id=? AND product_code=?`,
		navDate, unit, runID, code)
	return err
}

func (s *Store) CountCrawlRuns(problemsOnly bool) (int, error) {
	q := `SELECT COUNT(*) FROM crawl_run`
	if problemsOnly {
		q += ` WHERE status IN ('fail','partial')`
	}
	var n int
	err := s.DB.QueryRow(q).Scan(&n)
	return n, err
}

func (s *Store) ListCrawlRunsPage(limit, offset int, problemsOnly bool) ([]CrawlRun, error) {
	q := `SELECT id, started_at, finished_at, status, pages_ok, products_ok, error_summary FROM crawl_run`
	if problemsOnly {
		q += ` WHERE status IN ('fail','partial')`
	}
	q += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	rows, err := s.DB.Query(q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CrawlRun
	for rows.Next() {
		var c CrawlRun
		if err := rows.Scan(&c.ID, &c.StartedAt, &c.FinishedAt, &c.Status, &c.PagesOK, &c.ProductsOK, &c.ErrorSummary); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ListCrawlRuns(limit int) ([]CrawlRun, error) {
	return s.ListCrawlRunsPage(limit, 0, false)
}

func (s *Store) HoldingByUserCode(userID int64, code string) (*Holding, error) {
	h := &Holding{}
	err := s.DB.QueryRow(`SELECT id, user_id, product_code, created_at FROM holding WHERE user_id=? AND product_code=?`, userID, code).
		Scan(&h.ID, &h.UserID, &h.ProductCode, &h.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return h, err
}

func (s *Store) ListHoldings(userID int64) ([]Holding, error) {
	rows, err := s.DB.Query(`SELECT id, user_id, product_code, created_at FROM holding WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Holding
	for rows.Next() {
		var h Holding
		if err := rows.Scan(&h.ID, &h.UserID, &h.ProductCode, &h.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) CreateHolding(userID int64, code string, now time.Time) (*Holding, error) {
	res, err := s.DB.Exec(`INSERT INTO holding(user_id, product_code, created_at) VALUES(?,?,?)`,
		userID, code, now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Holding{ID: id, UserID: userID, ProductCode: code, CreatedAt: now.Format(time.RFC3339)}, nil
}

func (s *Store) DeleteHolding(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM holding WHERE id=?`, id)
	return err
}

func (s *Store) ListLedger(holdingID int64) ([]Ledger, error) {
	rows, err := s.DB.Query(`
		SELECT id, holding_id, kind, occur_date, cash_fen, shares_e8, unit_nav_e8, nav_date_used, voided_at, note, created_at
		FROM ledger_entry WHERE holding_id=? ORDER BY occur_date ASC, id ASC`, holdingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ledger
	for rows.Next() {
		var e Ledger
		if err := rows.Scan(&e.ID, &e.HoldingID, &e.Kind, &e.OccurDate, &e.CashFen, &e.SharesE8, &e.UnitNavE8, &e.NavDateUsed, &e.VoidedAt, &e.Note, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) InsertLedger(e Ledger) (int64, error) {
	res, err := s.DB.Exec(`
		INSERT INTO ledger_entry(holding_id, kind, occur_date, cash_fen, shares_e8, unit_nav_e8, nav_date_used, note, created_at)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		e.HoldingID, e.Kind, e.OccurDate, e.CashFen, e.SharesE8, e.UnitNavE8, e.NavDateUsed, e.Note, e.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) LastActiveLedger(holdingID int64) (*Ledger, error) {
	e := &Ledger{}
	err := s.DB.QueryRow(`
		SELECT id, holding_id, kind, occur_date, cash_fen, shares_e8, unit_nav_e8, nav_date_used, voided_at, note, created_at
		FROM ledger_entry WHERE holding_id=? AND voided_at IS NULL ORDER BY id DESC LIMIT 1`, holdingID).
		Scan(&e.ID, &e.HoldingID, &e.Kind, &e.OccurDate, &e.CashFen, &e.SharesE8, &e.UnitNavE8, &e.NavDateUsed, &e.VoidedAt, &e.Note, &e.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return e, err
}

func (s *Store) VoidLedger(id int64, now time.Time) error {
	_, err := s.DB.Exec(`UPDATE ledger_entry SET voided_at=? WHERE id=?`, now.Format(time.RFC3339), id)
	return err
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
