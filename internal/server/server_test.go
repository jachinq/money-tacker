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

func TestProductCatalog(t *testing.T) {
	s := testServer(t)
	h := s.Router()
	anon := doJSON(t, h, "GET", "/api/products", nil, nil)
	if anon.Code != 401 {
		t.Fatalf("anon %d", anon.Code)
	}

	reg := doJSON(t, h, "POST", "/api/auth/register", map[string]string{"account": "alice", "password": "secret1"}, nil)
	ck := cookie(reg)
	now := s.Now()
	fetched := now.Format(time.RFC3339)

	if err := s.Store.UpsertProduct("HI360001", "高窗口产品", "甲行", 1, now); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.UpsertSnapshot("HI360001", "2025-09-01", 100000000, 0, 0, "", fetched); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.UpsertSnapshot("HI360001", "2026-09-18", 110000000, 0, 0, "", fetched); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.UpsertProduct("LO360001", "低窗口产品", "乙行", 1, now); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.UpsertSnapshot("LO360001", "2025-09-01", 100000000, 0, 0, "", fetched); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.UpsertSnapshot("LO360001", "2026-09-18", 105000000, 0, 0, "", fetched); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.UpsertProduct("GONE0001", "已不在架", "丙行", 1, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB.Exec(`UPDATE product SET listed=0 WHERE code=?`, "GONE0001"); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.UpsertProduct("NONAV002", "无净值在架", "测试", 1, now); err != nil {
		t.Fatal(err)
	}

	buy := doJSON(t, h, "POST", "/api/holdings", map[string]string{
		"product_code": "HI360001", "amount": "10000", "occur_date": "2026-09-18",
	}, ck)
	if buy.Code != 200 {
		t.Fatalf("buy %d %s", buy.Code, buy.Body.String())
	}
	if err := s.Store.UpsertProduct("CLOSED01", "已清仓产品", "甲行", 1, now); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.UpsertSnapshot("CLOSED01", "2026-09-18", 100000000, 0, 0, "", fetched); err != nil {
		t.Fatal(err)
	}
	openClosed := doJSON(t, h, "POST", "/api/holdings", map[string]string{
		"product_code": "CLOSED01", "amount": "10000", "occur_date": "2026-09-18",
	}, ck)
	if openClosed.Code != 200 {
		t.Fatalf("open closed %d %s", openClosed.Code, openClosed.Body.String())
	}
	red := doJSON(t, h, "POST", "/api/holdings/CLOSED01/redeems", map[string]any{
		"all": true, "occur_date": "2026-09-18",
	}, ck)
	if red.Code != 200 {
		t.Fatalf("redeem %d %s", red.Code, red.Body.String())
	}

	type item struct {
		ProductCode   string  `json:"product_code"`
		Name          string  `json:"name"`
		Issuer        string  `json:"issuer"`
		Listed        bool    `json:"listed"`
		LatestNav     string  `json:"latest_nav"`
		LatestNavDate string  `json:"latest_nav_date"`
		Holding       string  `json:"holding"`
		Ret30         *string `json:"ret_30"`
		Ret90         *string `json:"ret_90"`
		Ret180        *string `json:"ret_180"`
		Ret360        *string `json:"ret_360"`
		Ret730        *string `json:"ret_730"`
	}
	type catalog struct {
		Items    []item `json:"items"`
		Total    int    `json:"total"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
		Empty    string `json:"empty"`
	}

	parse := func(path string) catalog {
		t.Helper()
		rec := doJSON(t, h, "GET", path, nil, ck)
		if rec.Code != 200 {
			t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
		}
		var out catalog
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	def := parse("/api/products")
	if def.Page != 1 || def.PageSize != 50 {
		t.Fatalf("page %+v", def)
	}
	if def.Empty != "" {
		t.Fatalf("empty hint %+v", def)
	}
	codes := make([]string, len(def.Items))
	hold := map[string]string{}
	for i, it := range def.Items {
		codes[i] = it.ProductCode
		hold[it.ProductCode] = it.Holding
		if !it.Listed && it.ProductCode == "GONE0001" {
			t.Fatal("default must hide unlisted")
		}
	}
	if len(codes) < 2 || codes[0] != "HI360001" || codes[1] != "LO360001" {
		t.Fatalf("default 360-day desc got %v", codes)
	}
	if def.Items[0].Ret360 == nil || *def.Items[0].Ret360 != "+10.00" {
		t.Fatalf("hi 360 %+v", def.Items[0].Ret360)
	}
	if def.Items[1].Ret360 == nil || *def.Items[1].Ret360 != "+5.00" {
		t.Fatalf("lo 360 %+v", def.Items[1].Ret360)
	}
	if hold["HI360001"] != "open" || hold["CLOSED01"] != "closed" || hold["LO360001"] != "none" {
		t.Fatalf("holding %v", hold)
	}
	var nonav *item
	for i := range def.Items {
		if def.Items[i].ProductCode == "NONAV002" {
			nonav = &def.Items[i]
		}
	}
	if nonav == nil {
		t.Fatal("product with no nav must still appear")
	}
	if nonav.LatestNav != "" || nonav.LatestNavDate != "" || nonav.Ret30 != nil || nonav.Ret360 != nil {
		t.Fatalf("no-nav row %+v", nonav)
	}

	withGone := parse("/api/products?include_unlisted=true")
	foundGone := false
	for _, it := range withGone.Items {
		if it.ProductCode == "GONE0001" {
			foundGone = true
			if it.Listed {
				t.Fatal("GONE0001 should be unlisted")
			}
		}
	}
	if !foundGone {
		t.Fatal("include_unlisted missing GONE0001")
	}

	q := parse("/api/products?q=hi360")
	if len(q.Items) != 1 || q.Items[0].ProductCode != "HI360001" {
		t.Fatalf("search code %v", q.Items)
	}
	q = parse("/api/products?q=信银")
	if len(q.Items) != 1 || q.Items[0].ProductCode != "AF247494G" {
		t.Fatalf("search name %v", q.Items)
	}

	page := parse("/api/products?page=1&page_size=1")
	if page.Total < 2 || page.PageSize != 1 || len(page.Items) != 1 || page.Items[0].ProductCode != "HI360001" {
		t.Fatalf("page1 %+v", page)
	}
	page2 := parse("/api/products?page=2&page_size=1")
	if len(page2.Items) != 1 || page2.Items[0].ProductCode != "LO360001" {
		t.Fatalf("page2 %+v", page2)
	}

	emptyS := testServer(t)
	if _, err := emptyS.Store.DB.Exec(`DELETE FROM nav_snapshot`); err != nil {
		t.Fatal(err)
	}
	if _, err := emptyS.Store.DB.Exec(`DELETE FROM product`); err != nil {
		t.Fatal(err)
	}
	emptyH := emptyS.Router()
	emptyReg := doJSON(t, emptyH, "POST", "/api/auth/register", map[string]string{"account": "alice", "password": "secret1"}, nil)
	emptyRec := doJSON(t, emptyH, "GET", "/api/products", nil, cookie(emptyReg))
	if emptyRec.Code != 200 {
		t.Fatalf("empty catalog %d %s", emptyRec.Code, emptyRec.Body.String())
	}
	var emptyCat catalog
	_ = json.Unmarshal(emptyRec.Body.Bytes(), &emptyCat)
	if emptyCat.Total != 0 || len(emptyCat.Items) != 0 || emptyCat.Empty != "no_products" {
		t.Fatalf("want empty catalog hint, got %+v", emptyCat)
	}
}

func TestHTTPProdDoesNotForceSecureSessionCookie(t *testing.T) {
	s := testServer(t)
	s.Cfg.Env = "prod"
	s.Cfg.CookieSecure = false
	h := s.Router()
	rec := doJSON(t, h, "POST", "/api/auth/register", map[string]string{"account": "alice", "password": "secret1"}, nil)
	if rec.Code != 200 {
		t.Fatalf("register %d %s", rec.Code, rec.Body.String())
	}
	cks := cookie(rec)
	if len(cks) == 0 || cks[0].Name != "sid" {
		t.Fatal("missing sid cookie")
	}
	if cks[0].Secure {
		t.Fatal("HTTP must not set Secure, or browsers drop the cookie")
	}
	me := doJSON(t, h, "GET", "/api/me", nil, cks)
	if me.Code != 200 {
		t.Fatalf("/api/me %d %s", me.Code, me.Body.String())
	}
}

func TestCookieSecureAndForwardedProto(t *testing.T) {
	s := testServer(t)
	s.Cfg.CookieSecure = true
	h := s.Router()
	rec := doJSON(t, h, "POST", "/api/auth/register", map[string]string{"account": "alice", "password": "secret1"}, nil)
	if rec.Code != 200 {
		t.Fatalf("register %d %s", rec.Code, rec.Body.String())
	}
	if cks := cookie(rec); len(cks) == 0 || !cks[0].Secure {
		t.Fatal("COOKIE_SECURE=true must set Secure")
	}

	s2 := testServer(t)
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(map[string]string{"account": "bob", "password": "secret1"})
	req := httptest.NewRequest("POST", "/api/auth/register", &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec2 := httptest.NewRecorder()
	s2.Router().ServeHTTP(rec2, req)
	if rec2.Code != 200 {
		t.Fatalf("register via https proto %d %s", rec2.Code, rec2.Body.String())
	}
	if cks := cookie(rec2); len(cks) == 0 || !cks[0].Secure {
		t.Fatal("X-Forwarded-Proto=https must set Secure")
	}
}

func TestCreateHoldingImpliedCumulative(t *testing.T) {
	s := testServer(t)
	h := s.Router()
	reg := doJSON(t, h, "POST", "/api/auth/register", map[string]string{"account": "alice", "password": "secret1"}, nil)
	ck := cookie(reg)
	if err := s.Store.UpsertSnapshot("AF247494G", "2026-09-18", 101000000, 101000000, 0, "", s.Now().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	buy := doJSON(t, h, "POST", "/api/holdings", map[string]string{
		"product_code": "AF247494G", "amount": "10000", "occur_date": "2024-03-01", "cumulative": "100.00",
	}, ck)
	if buy.Code != 200 {
		t.Fatalf("buy %d %s", buy.Code, buy.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(buy.Body.Bytes(), &created)
	if created["estimated"] == true {
		t.Fatalf("implied buy must not be estimated: %+v", created)
	}

	det := doJSON(t, h, "GET", "/api/holdings/AF247494G", nil, ck)
	if det.Code != 200 {
		t.Fatalf("detail %d %s", det.Code, det.Body.String())
	}
	var body struct {
		Holding struct {
			Cumulative  string `json:"cumulative"`
			Unrealized  string `json:"unrealized"`
			Cost        string `json:"cost"`
			MarketValue string `json:"market_value"`
		} `json:"holding"`
	}
	if err := json.Unmarshal(det.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Holding.Cost != "10000.00" {
		t.Fatalf("cost %s", body.Holding.Cost)
	}
	if body.Holding.Cumulative != "100.00" || body.Holding.Unrealized != "100.00" {
		t.Fatalf("want 累计/未实现 100.00 got cum=%s unrel=%s mv=%s",
			body.Holding.Cumulative, body.Holding.Unrealized, body.Holding.MarketValue)
	}

	pnlRec := doJSON(t, h, "GET", "/api/holdings/AF247494G/pnl", nil, ck)
	if pnlRec.Code != 200 {
		t.Fatalf("pnl %d %s", pnlRec.Code, pnlRec.Body.String())
	}
	var pnlBody struct {
		CollectedDaily string `json:"collected_daily"`
		CollectionGap  string `json:"collection_gap"`
	}
	if err := json.Unmarshal(pnlRec.Body.Bytes(), &pnlBody); err != nil {
		t.Fatal(err)
	}
	if pnlBody.CollectedDaily != "0.00" {
		t.Fatalf("single nav day has no prev, collected=%s", pnlBody.CollectedDaily)
	}
	if pnlBody.CollectionGap != "100.00" {
		t.Fatalf("gap %s want 100.00", pnlBody.CollectionGap)
	}
}

func TestCreateHoldingPastDateRequiresCumulative(t *testing.T) {
	s := testServer(t)
	h := s.Router()
	reg := doJSON(t, h, "POST", "/api/auth/register", map[string]string{"account": "alice", "password": "secret1"}, nil)
	ck := cookie(reg)
	miss := doJSON(t, h, "POST", "/api/holdings", map[string]string{
		"product_code": "AF247494G", "amount": "10000", "occur_date": "2024-03-01",
	}, ck)
	if miss.Code != 400 {
		t.Fatalf("want 400, got %d %s", miss.Code, miss.Body.String())
	}
	if !strings.Contains(miss.Body.String(), "累计收益") {
		t.Fatalf("want 累计收益 hint, got %s", miss.Body.String())
	}
	ignored := doJSON(t, h, "POST", "/api/holdings", map[string]string{
		"product_code": "AF247494G", "amount": "10000", "occur_date": "2024-03-01", "unit_nav": "1.0000",
	}, ck)
	if ignored.Code != 400 {
		t.Fatalf("create must ignore 手工净值, got %d %s", ignored.Code, ignored.Body.String())
	}
}

func TestCreateHoldingCumulativeNeedsLatestNav(t *testing.T) {
	s := testServer(t)
	h := s.Router()
	reg := doJSON(t, h, "POST", "/api/auth/register", map[string]string{"account": "alice", "password": "secret1"}, nil)
	ck := cookie(reg)
	if err := s.Store.UpsertProduct("NONAV001", "无净值产品", "测试", 1, s.Now()); err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, h, "POST", "/api/holdings", map[string]string{
		"product_code": "NONAV001", "amount": "10000", "occur_date": "2026-09-21", "cumulative": "100.00",
	}, ck)
	if rec.Code != 400 {
		t.Fatalf("want 400, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestAdditionalBuyIgnoresCumulativeUsesManualNav(t *testing.T) {
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
		"amount": "5000", "occur_date": "2024-03-01", "cumulative": "999.00", "unit_nav": "1.0000",
	}, ck)
	if add.Code != 200 {
		t.Fatalf("add %d %s", add.Code, add.Body.String())
	}
	var created struct {
		UnitNav     string `json:"unit_nav"`
		NavDateUsed string `json:"nav_date_used"`
	}
	_ = json.Unmarshal(add.Body.Bytes(), &created)
	if created.UnitNav != "1.00000000" || created.NavDateUsed != "2024-03-01" {
		t.Fatalf("append must use 手工净值, got %+v", created)
	}
}
