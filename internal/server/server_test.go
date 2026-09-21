package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"money-tacker/internal/config"
	"money-tacker/internal/db"
	"money-tacker/internal/money"
	"money-tacker/internal/store"
	"money-tacker/migrations"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "app.db"), migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	loc := time.FixedZone("CST", 8*3600)
	cfg := config.Config{
		Addr: ":8080", Env: "test", TZ: loc, SessionHours: 24,
		DisplayLagDays: 1, RegisterOpen: false, BcryptCost: 4,
	}
	s := New(cfg, &store.Store{DB: sqlDB})
	s.Now = func() time.Time { return time.Date(2026, 9, 21, 15, 0, 0, 0, loc) }
	if err := s.Store.UpsertProduct("AF247494G", "信银理财安盈象固收稳利一个月持有期60号理财产品", "信银理财", 1, s.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.UpsertSnapshot("AF247494G", "2026-09-18", 105620000, 105620000, 0, "", s.Now().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	return s
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func cookie(rec *httptest.ResponseRecorder) []*http.Cookie {
	return rec.Result().Cookies()
}

func TestTwoUsersCannotSeeEachOthersHoldings(t *testing.T) {
	s := testServer(t)
	h := s.Router()
	a := doJSON(t, h, "POST", "/api/auth/register", map[string]string{"account": "alice", "password": "secret1"}, nil)
	if a.Code != 200 {
		t.Fatalf("alice register %d %s", a.Code, a.Body.String())
	}
	b := doJSON(t, h, "POST", "/api/auth/register", map[string]string{"account": "bob", "password": "secret2"}, nil)
	if b.Code != 403 {
		t.Fatalf("second register should close, got %d %s", b.Code, b.Body.String())
	}
	s.Cfg.RegisterOpen = true
	b = doJSON(t, h, "POST", "/api/auth/register", map[string]string{"account": "bob", "password": "secret2"}, nil)
	if b.Code != 200 {
		t.Fatalf("bob register %d %s", b.Code, b.Body.String())
	}
	ca, cb := cookie(a), cookie(b)
	buy := doJSON(t, h, "POST", "/api/holdings", map[string]string{
		"product_code": "AF247494G", "amount": "10000", "occur_date": "2026-09-21",
	}, ca)
	if buy.Code != 200 {
		t.Fatalf("buy %d %s", buy.Code, buy.Body.String())
	}
	listB := doJSON(t, h, "GET", "/api/holdings", nil, cb)
	if listB.Code != 200 {
		t.Fatal(listB.Body.String())
	}
	var parsed map[string]any
	_ = json.Unmarshal(listB.Body.Bytes(), &parsed)
	items, _ := parsed["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("bob saw %v", parsed)
	}
	other := doJSON(t, h, "GET", "/api/holdings/AF247494G", nil, cb)
	if other.Code != 404 {
		t.Fatalf("bob should 404, got %d", other.Code)
	}
}

func TestOverRedeemAndVoidNotLast(t *testing.T) {
	s := testServer(t)
	h := s.Router()
	reg := doJSON(t, h, "POST", "/api/auth/register", map[string]string{"account": "alice", "password": "secret1"}, nil)
	ck := cookie(reg)
	buy := doJSON(t, h, "POST", "/api/holdings", map[string]string{
		"product_code": "AF247494G", "amount": "10000", "occur_date": "2026-09-18",
	}, ck)
	if buy.Code != 200 {
		t.Fatal(buy.Body.String())
	}
	add := doJSON(t, h, "POST", "/api/holdings/AF247494G/buys", map[string]string{
		"amount": "5000", "occur_date": "2026-09-18",
	}, ck)
	if add.Code != 200 {
		t.Fatal(add.Body.String())
	}
	over := doJSON(t, h, "POST", "/api/holdings/AF247494G/redeems", map[string]any{
		"shares": "999999", "occur_date": "2026-09-21",
	}, ck)
	if over.Code != 409 {
		t.Fatalf("over %d %s", over.Code, over.Body.String())
	}
	var addBody struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(buy.Body.Bytes(), &addBody)
	void := doJSON(t, h, "POST", "/api/holdings/AF247494G/ledger/"+itoa(addBody.ID)+"/void", map[string]any{}, ck)
	if void.Code != 409 {
		t.Fatalf("void first %d %s", void.Code, void.Body.String())
	}
}

func TestProductLookup(t *testing.T) {
	s := testServer(t)
	h := s.Router()
	reg := doJSON(t, h, "POST", "/api/auth/register", map[string]string{"account": "alice", "password": "secret1"}, nil)
	ck := cookie(reg)

	ok := doJSON(t, h, "GET", "/api/products/af247494g", nil, ck)
	if ok.Code != 200 {
		t.Fatalf("lookup %d %s", ok.Code, ok.Body.String())
	}
	var p map[string]any
	_ = json.Unmarshal(ok.Body.Bytes(), &p)
	if p["product_code"] != "AF247494G" || p["name"] == "" || p["listed"] != true {
		t.Fatalf("product %+v", p)
	}
	if p["latest_nav"] != money.FormatNav(105620000) || p["latest_nav_date"] != "2026-09-18" {
		t.Fatalf("nav %+v", p)
	}

	miss := doJSON(t, h, "GET", "/api/products/NOPE999", nil, ck)
	if miss.Code != 404 {
		t.Fatalf("miss %d %s", miss.Code, miss.Body.String())
	}

	if _, err := s.Store.DB.Exec(`UPDATE product SET listed=0 WHERE code=?`, "AF247494G"); err != nil {
		t.Fatal(err)
	}
	delisted := doJSON(t, h, "GET", "/api/products/AF247494G", nil, ck)
	if delisted.Code != 200 {
		t.Fatalf("delisted %d %s", delisted.Code, delisted.Body.String())
	}
	_ = json.Unmarshal(delisted.Body.Bytes(), &p)
	if p["listed"] != false {
		t.Fatalf("want listed false, got %+v", p)
	}

	if err := s.Store.UpsertProduct("NONAV001", "无净值产品", "测试", 1, s.Now()); err != nil {
		t.Fatal(err)
	}
	empty := doJSON(t, h, "GET", "/api/products/NONAV001", nil, ck)
	if empty.Code != 200 {
		t.Fatalf("empty %d %s", empty.Code, empty.Body.String())
	}
	_ = json.Unmarshal(empty.Body.Bytes(), &p)
	if p["latest_nav"] != "" || p["latest_nav_date"] != "" {
		t.Fatalf("want empty nav, got %+v", p)
	}
}

func TestBuyTodayDefaultProductShouldNotShowNegativePnL(t *testing.T) {
	s := testServer(t)
	h := s.Router()
	reg := doJSON(t, h, "POST", "/api/auth/register", map[string]string{"account": "alice", "password": "secret1"}, nil)
	ck := cookie(reg)
	buy := doJSON(t, h, "POST", "/api/holdings", map[string]string{
		"product_code": "AF247494G", "amount": "10000", "occur_date": "2026-09-21",
	}, ck)
	if buy.Code != 200 {
		t.Fatalf("buy %d %s", buy.Code, buy.Body.String())
	}

	ov := doJSON(t, h, "GET", "/api/overview", nil, ck)
	if ov.Code != 200 {
		t.Fatalf("overview %d %s", ov.Code, ov.Body.String())
	}

	var parsed struct {
		CumulativePnl string `json:"cumulative_pnl"`
		DailyPnl      string `json:"daily_pnl"`
		Items         []struct {
			Cost        string `json:"cost"`
			MarketValue string `json:"market_value"`
			Unrealized  string `json:"unrealized"`
			Cumulative  string `json:"cumulative"`
			DailyPnl    string `json:"daily_pnl"`
			DisplayDate string `json:"display_date"`
			Shares      string `json:"shares"`
		} `json:"items"`
	}
	if err := json.Unmarshal(ov.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Items) != 1 {
		t.Fatalf("items %+v", parsed)
	}
	it := parsed.Items[0]
	neg := strings.HasPrefix(it.Unrealized, "-") || strings.HasPrefix(it.Cumulative, "-") ||
		strings.HasPrefix(it.DailyPnl, "-") || strings.HasPrefix(parsed.CumulativePnl, "-") ||
		strings.HasPrefix(parsed.DailyPnl, "-")
	if neg {
		t.Fatalf("negative pnl after same-day buy: overview_cum=%s overview_day=%s cost=%s mv=%s unrel=%s cum=%s daily=%s display=%s shares=%s",
			parsed.CumulativePnl, parsed.DailyPnl, it.Cost, it.MarketValue, it.Unrealized, it.Cumulative, it.DailyPnl, it.DisplayDate, it.Shares)
	}
}

func TestUnknownProductNoHolding(t *testing.T) {
	s := testServer(t)
	h := s.Router()
	reg := doJSON(t, h, "POST", "/api/auth/register", map[string]string{"account": "alice", "password": "secret1"}, nil)
	miss := doJSON(t, h, "POST", "/api/holdings", map[string]string{
		"product_code": "NOPE999", "amount": "10000",
	}, cookie(reg))
	if miss.Code != 404 {
		t.Fatalf("got %d %s", miss.Code, miss.Body.String())
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
