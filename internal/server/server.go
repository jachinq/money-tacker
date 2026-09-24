package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"

	"money-tacker/internal/config"
	"money-tacker/internal/crawl"
	"money-tacker/internal/money"
	"money-tacker/internal/pnl"
	"money-tacker/internal/store"
)

type Server struct {
	Cfg       config.Config
	Store     *store.Store
	Now       func() time.Time
	Crawl     func() error
	CrawlBusy func() bool
	Static    string
}

func New(cfg config.Config, st *store.Store) *Server {
	return &Server{
		Cfg:   cfg,
		Store: st,
		Now:   func() time.Time { return time.Now().In(cfg.TZ) },
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/register", s.handleRegister)
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/logout", s.handleLogout)
		r.Get("/admin/crawl-runs", s.handleCrawlRuns)
		r.Post("/admin/crawl", s.handleCrawlNow)
		r.Group(func(r chi.Router) {
			r.Use(s.auth)
			r.Get("/me", s.handleMe)
			r.Post("/auth/password", s.handlePassword)
			r.Get("/overview", s.handleOverview)
			r.Get("/holdings", s.handleListHoldings)
			r.Post("/holdings", s.handleCreateHolding)
			r.Get("/holdings/{code}", s.handleGetHolding)
			r.Post("/holdings/{code}/buys", s.handleBuy)
			r.Post("/holdings/{code}/redeems", s.handleRedeem)
			r.Post("/holdings/{code}/ledger/{id}/void", s.handleVoid)
			r.Delete("/holdings/{code}", s.handleDeleteHolding)
			r.Get("/holdings/{code}/pnl", s.handleHoldingPnl)
			r.Get("/products", s.handleProductCatalog)
			r.Get("/crawl-runs", s.handleListCrawls)
			r.Post("/crawl-runs", s.handleStartCrawl)
			r.Get("/products/{code}", s.handleProduct)
			r.Get("/products/{code}/nav", s.handleProductNav)
		})
	})
	if s.Static != "" {
		r.Handle("/*", spa(s.Static))
	}
	return r
}

func spa(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			http.ServeFile(w, r, p)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}

func (s *Server) today() string {
	return s.Now().Format("2006-01-02")
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("sid")
		if err != nil || c.Value == "" {
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "未登录")
			return
		}
		u, err := s.Store.UserBySession(c.Value, s.Now())
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "未登录")
			return
		}
		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), u)))
	})
}

func (s *Server) sessionCookieSecure(r *http.Request) bool {
	if s.Cfg.CookieSecure {
		return true
	}
	if r != nil && r.TLS != nil {
		return true
	}
	if r != nil && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return false
}

func (s *Server) setSession(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "sid",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.sessionCookieSecure(r),
		MaxAge:   s.Cfg.SessionHours * 3600,
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Account  string `json:"account"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "BAD_REQUEST", "无效请求")
		return
	}
	req.Account = strings.TrimSpace(req.Account)
	if req.Account == "" || len(req.Password) < 6 {
		writeErr(w, 400, "BAD_REQUEST", "用户名不能为空，密码至少 6 位")
		return
	}
	n, err := s.Store.CountUsers()
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	if n > 0 && !s.Cfg.RegisterOpen {
		writeErr(w, 403, "REGISTER_CLOSED", "注册已关闭")
		return
	}
	u, err := s.Store.CreateUser(req.Account, req.Password, s.Cfg.BcryptCost, s.Now())
	if err != nil {
		writeErr(w, 409, "ACCOUNT_EXISTS", "账号已存在")
		return
	}
	tok, err := s.Store.CreateSession(u.ID, time.Duration(s.Cfg.SessionHours)*time.Hour, s.Now())
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	s.setSession(w, r, tok)
	writeJSON(w, 200, map[string]any{"account": u.Account})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Account  string `json:"account"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "BAD_REQUEST", "无效请求")
		return
	}
	u, err := s.Store.UserByAccount(strings.TrimSpace(req.Account))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		writeErr(w, 401, "UNAUTHORIZED", "用户名或密码错误")
		return
	}
	tok, err := s.Store.CreateSession(u.ID, time.Duration(s.Cfg.SessionHours)*time.Hour, s.Now())
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	s.setSession(w, r, tok)
	writeJSON(w, 200, map[string]any{"account": u.Account})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("sid"); err == nil {
		_ = s.Store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "sid",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.sessionCookieSecure(r),
	})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	writeJSON(w, 200, map[string]any{"account": u.Account})
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := decode(r, &req); err != nil || len(req.NewPassword) < 6 {
		writeErr(w, 400, "BAD_REQUEST", "新密码至少 6 位")
		return
	}
	u := userFrom(r.Context())
	full, err := s.Store.UserByID(u.ID)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(full.PasswordHash), []byte(req.OldPassword)) != nil {
		writeErr(w, 401, "UNAUTHORIZED", "原密码错误")
		return
	}
	if err := s.Store.UpdatePassword(u.ID, req.NewPassword, s.Cfg.BcryptCost); err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleCreateHolding(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	var req buyReq
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "BAD_REQUEST", "无效请求")
		return
	}
	req.ProductCode = strings.ToUpper(strings.TrimSpace(req.ProductCode))
	s.writeBuy(w, u.ID, req, true)
}

func (s *Server) handleBuy(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	var req buyReq
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "BAD_REQUEST", "无效请求")
		return
	}
	req.ProductCode = strings.ToUpper(chi.URLParam(r, "code"))
	s.writeBuy(w, u.ID, req, false)
}

type buyReq struct {
	ProductCode string `json:"product_code"`
	Amount      string `json:"amount"`
	OccurDate   string `json:"occur_date"`
	UnitNav     string `json:"unit_nav"`
	NavDate     string `json:"nav_date"`
	Cumulative  string `json:"cumulative"`
}

func (s *Server) writeBuy(w http.ResponseWriter, userID int64, req buyReq, create bool) {
	p, err := s.Store.Product(req.ProductCode)
	if err != nil {
		writeErr(w, 404, "PRODUCT_NOT_FOUND", "未在中国银行代销净值列表中找到该代码")
		return
	}
	if !p.Listed {
		if create {
			writeErr(w, 404, "PRODUCT_NOT_FOUND", "未在中国银行代销净值列表中找到该代码")
			return
		}
		writeErr(w, 409, "PRODUCT_DELISTED", "产品已不在代销目录，不能追加买入")
		return
	}
	cash, ok := money.ParseYuanToFen(strings.TrimSpace(req.Amount))
	if !ok || cash <= 0 {
		writeErr(w, 400, "BAD_REQUEST", "购入金额必须大于 0")
		return
	}
	occur := s.normOccur(req.OccurDate)
	if occur == "" || occur > s.today() {
		writeErr(w, 400, "BAD_REQUEST", "发生日不能晚于今天")
		return
	}
	navs := s.navPoints(req.ProductCode)
	var navE8, shares int64
	var used string
	var estimated bool
	if create {
		if cumStr := strings.TrimSpace(req.Cumulative); cumStr != "" {
			cum, parsed := money.ParseYuanToFen(cumStr)
			if !parsed {
				writeErr(w, 400, "BAD_REQUEST", "累计收益无效")
				return
			}
			latest, has := pnl.LatestNav(navs)
			if !has {
				writeErr(w, 400, "NAV_UNAVAILABLE", "尚无最新净值，不能按累计收益反推")
				return
			}
			shares, navE8, ok = pnl.ImpliedBuy(cash, cum, latest.UnitNavE8)
			if !ok {
				writeErr(w, 400, "BAD_REQUEST", "累计收益无效")
				return
			}
			used = occur
		} else {
			navE8, used, ok, estimated = pnl.ResolveNavForOccur(navs, occur, 0, "")
			if !ok {
				writeErr(w, 400, "NAV_UNAVAILABLE", "无可用净值，请填写累计收益")
				return
			}
			shares = money.SharesFromCash(cash, navE8)
		}
	} else {
		var manual int64
		if strings.TrimSpace(req.UnitNav) != "" {
			manual, ok = money.ParseNavToE8(req.UnitNav)
			if !ok || manual <= 0 {
				writeErr(w, 400, "BAD_REQUEST", "手工净值无效")
				return
			}
		}
		navE8, used, ok, estimated = pnl.ResolveNavForOccur(navs, occur, manual, strings.TrimSpace(req.NavDate))
		if !ok {
			writeErr(w, 400, "NAV_UNAVAILABLE", "无可用净值，请填写手工净值")
			return
		}
		shares = money.SharesFromCash(cash, navE8)
	}
	if shares <= 0 {
		writeErr(w, 400, "BAD_REQUEST", "份额无效")
		return
	}
	created := false
	h, err := s.Store.HoldingByUserCode(userID, req.ProductCode)
	if errors.Is(err, store.ErrNotFound) {
		if !create {
			writeErr(w, 404, "NOT_FOUND", "持仓账户不存在")
			return
		}
		h, err = s.Store.CreateHolding(userID, req.ProductCode, s.Now())
		if err != nil {
			writeErr(w, 500, "INTERNAL", "服务器错误")
			return
		}
		created = true
	} else if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	} else if create {
		writeErr(w, 409, "EXISTS", "该产品已有持仓账户，请走追加买入")
		return
	}
	id, err := s.Store.InsertLedger(store.Ledger{
		HoldingID: h.ID, Kind: "buy", OccurDate: occur,
		CashFen: cash, SharesE8: shares, UnitNavE8: navE8, NavDateUsed: used,
		CreatedAt: s.Now().Format(time.RFC3339),
	})
	if err != nil {
		if created {
			_ = s.Store.DeleteHolding(h.ID)
		}
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	writeJSON(w, 200, map[string]any{
		"id": id, "estimated": estimated, "nav_date_used": used,
		"unit_nav": money.FormatNav(navE8), "shares": money.FormatShares(shares),
	})
}

func (s *Server) handleRedeem(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	code := strings.ToUpper(chi.URLParam(r, "code"))
	h, err := s.Store.HoldingByUserCode(u.ID, code)
	if err != nil {
		writeErr(w, 404, "NOT_FOUND", "持仓账户不存在")
		return
	}
	var req struct {
		OccurDate string `json:"occur_date"`
		UnitNav   string `json:"unit_nav"`
		NavDate   string `json:"nav_date"`
		Shares    string `json:"shares"`
		Amount    string `json:"amount"`
		All       bool   `json:"all"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "BAD_REQUEST", "无效请求")
		return
	}
	nset := 0
	if req.All {
		nset++
	}
	if strings.TrimSpace(req.Shares) != "" {
		nset++
	}
	if strings.TrimSpace(req.Amount) != "" {
		nset++
	}
	if nset != 1 {
		writeErr(w, 400, "BAD_REQUEST", "份额、金额、全部赎回三选一")
		return
	}
	occur := s.normOccur(req.OccurDate)
	if occur == "" || occur > s.today() {
		writeErr(w, 400, "BAD_REQUEST", "发生日不能晚于今天")
		return
	}
	entries, err := s.ledgerAsPnl(h.ID)
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	st, err := pnl.Replay(entries)
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	if st.SharesE8 <= 0 {
		writeErr(w, 409, "EMPTY_HOLDING", "已清仓")
		return
	}
	var manual int64
	ok := true
	if strings.TrimSpace(req.UnitNav) != "" {
		manual, ok = money.ParseNavToE8(req.UnitNav)
		if !ok || manual <= 0 {
			writeErr(w, 400, "BAD_REQUEST", "手工净值无效")
			return
		}
	}
	navE8, used, ok, _ := pnl.ResolveNavForOccur(s.navPoints(code), occur, manual, strings.TrimSpace(req.NavDate))
	if !ok {
		writeErr(w, 400, "NAV_UNAVAILABLE", "无可用净值，请填写手工净值")
		return
	}
	var shares, cash int64
	switch {
	case req.All:
		shares = st.SharesE8
		cash = money.CashFromShares(shares, navE8)
	case strings.TrimSpace(req.Shares) != "":
		shares, ok = money.ParseNavToE8(req.Shares)
		if !ok || shares <= 0 {
			writeErr(w, 400, "BAD_REQUEST", "份额无效")
			return
		}
		cash = money.CashFromShares(shares, navE8)
	default:
		cash, ok = money.ParseYuanToFen(req.Amount)
		if !ok || cash <= 0 {
			writeErr(w, 400, "BAD_REQUEST", "金额无效")
			return
		}
		shares = money.SharesFromCash(cash, navE8)
		cash = money.CashFromShares(shares, navE8)
	}
	if shares > st.SharesE8 {
		writeErr(w, 409, "INSUFFICIENT_SHARES", "赎回份额超过剩余份额")
		return
	}
	id, err := s.Store.InsertLedger(store.Ledger{
		HoldingID: h.ID, Kind: "redeem", OccurDate: occur,
		CashFen: cash, SharesE8: shares, UnitNavE8: navE8, NavDateUsed: used,
		CreatedAt: s.Now().Format(time.RFC3339),
	})
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	writeJSON(w, 200, map[string]any{
		"id": id, "shares": money.FormatShares(shares), "amount": money.FormatFen(cash),
	})
}

func (s *Server) handleVoid(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	code := strings.ToUpper(chi.URLParam(r, "code"))
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	h, err := s.Store.HoldingByUserCode(u.ID, code)
	if err != nil {
		writeErr(w, 404, "NOT_FOUND", "持仓账户不存在")
		return
	}
	last, err := s.Store.LastActiveLedger(h.ID)
	if err != nil {
		writeErr(w, 404, "NOT_FOUND", "没有可作废流水")
		return
	}
	if last.ID != id {
		writeErr(w, 409, "NOT_LAST_ENTRY", "只能作废最后一笔未作废流水")
		return
	}
	if err := s.Store.VoidLedger(id, s.Now()); err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleDeleteHolding(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	code := strings.ToUpper(chi.URLParam(r, "code"))
	h, err := s.Store.HoldingByUserCode(u.ID, code)
	if err != nil {
		writeErr(w, 404, "NOT_FOUND", "持仓账户不存在")
		return
	}
	if err := s.Store.DeleteHolding(h.ID); err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleListHoldings(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	items, err := s.holdingViews(u.ID, r.URL.Query().Get("include_closed") == "true")
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	items, err := s.holdingViews(u.ID, false)
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	var mv, cum, day, buyFen int64
	dates := map[string]struct{}{}
	for _, it := range items {
		mv += it.MarketValueFen
		cum += it.CumulativeFen
		day += it.DailyPnlFen
		buyFen += it.TotalBuyFen
		if it.DisplayDate != "" {
			dates[it.DisplayDate] = struct{}{}
		}
	}
	writeJSON(w, 200, map[string]any{
		"market_value":      money.FormatFen(mv),
		"cumulative_pnl":    money.FormatFen(cum),
		"cumulative_return": formatPctE4(pnl.CumulativeReturnE4(cum, buyFen)),
		"daily_pnl":         money.FormatFen(day),
		"mixed_dates":       len(dates) > 1,
		"disclaimer":        "收益估算，以发行机构/代销机构结算为准",
		"items":             items,
		"natural_day":       s.today(),
	})
}

func (s *Server) handleGetHolding(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	code := strings.ToUpper(chi.URLParam(r, "code"))
	h, err := s.Store.HoldingByUserCode(u.ID, code)
	if err != nil {
		writeErr(w, 404, "NOT_FOUND", "持仓账户不存在")
		return
	}
	items, err := s.holdingViews(u.ID, true)
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	led, err := s.Store.ListLedger(h.ID)
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	var view *holdingView
	for i := range items {
		if items[i].ProductCode == code {
			view = &items[i]
			break
		}
	}
	writeJSON(w, 200, map[string]any{"holding": view, "ledger": mapLedger(led)})
}

func (s *Server) handleHoldingPnl(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	code := strings.ToUpper(chi.URLParam(r, "code"))
	h, err := s.Store.HoldingByUserCode(u.ID, code)
	if err != nil {
		writeErr(w, 404, "NOT_FOUND", "持仓账户不存在")
		return
	}
	entries, err := s.ledgerAsPnl(h.ID)
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	navs := s.navPoints(code)
	from := s.today()
	if len(navs) > 0 {
		from = navs[0].Date
	}
	for _, e := range entries {
		if e.OccurDate < from {
			from = e.OccurDate
		}
	}
	rows := pnl.CalendarPnl(entries, navs, from, s.today())
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"date": row.Date, "pnl": money.FormatFen(row.PnlFen),
			"hang_zero": row.HangZero, "has_nav": row.HasNav,
			"unit_nav": money.FormatNav(row.UnitNavE8),
		})
	}
	st, err := pnl.Replay(entries)
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	var cum int64
	if latest, has := pnl.LatestNav(navs); has {
		mv := money.CashFromShares(st.SharesE8, latest.UnitNavE8)
		cum = mv - st.CostFen + st.RealizedFen
	}
	collected := pnl.CollectedDailySum(entries, navs)
	writeJSON(w, 200, map[string]any{
		"days":            out,
		"collected_daily": money.FormatFen(collected),
		"collection_gap":  money.FormatFen(pnl.CollectionGap(cum, collected)),
	})
}

func (s *Server) handleProduct(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(chi.URLParam(r, "code"))
	p, err := s.Store.Product(code)
	if err != nil {
		writeErr(w, 404, "PRODUCT_NOT_FOUND", "未在中国银行代销净值列表中找到该代码")
		return
	}
	navs := s.navPoints(code)
	latest, hasLatest := pnl.LatestNav(navs)
	latestNavS, latestDate := "", ""
	if hasLatest {
		latestNavS = money.FormatNav(latest.UnitNavE8)
		latestDate = latest.Date
	}
	writeJSON(w, 200, map[string]any{
		"product_code":    p.Code,
		"name":            p.Name,
		"issuer":          p.Issuer,
		"listed":          p.Listed,
		"latest_nav":      latestNavS,
		"latest_nav_date": latestDate,
	})
}

func (s *Server) handleProductNav(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(chi.URLParam(r, "code"))
	list, err := s.Store.ListNav(code)
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, n := range list {
		out = append(out, map[string]any{"nav_date": n.NavDate, "unit_nav": money.FormatNav(n.UnitNavE8)})
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

func (s *Server) handleListCrawls(w http.ResponseWriter, r *http.Request) {
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeErr(w, 400, "BAD_REQUEST", "页码无效")
			return
		}
		page = n
	}
	problems := r.URL.Query().Get("problems") == "1"
	const pageSize = 50
	total, err := s.Store.CountCrawlRuns(problems)
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	list, err := s.Store.ListCrawlRunsPage(pageSize, (page-1)*pageSize, problems)
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	items := make([]map[string]any, 0, len(list))
	for _, c := range list {
		var finished any
		if c.FinishedAt.Valid {
			finished = c.FinishedAt.String
		}
		summary := ""
		if c.ErrorSummary.Valid {
			summary = c.ErrorSummary.String
		}
		items = append(items, map[string]any{
			"id":          c.ID,
			"started_at":  c.StartedAt,
			"finished_at": finished,
			"status":      c.Status,
			"pages_ok":    c.PagesOK,
			"products_ok": c.ProductsOK,
			"summary":     summary,
		})
	}
	busy := false
	if s.CrawlBusy != nil {
		busy = s.CrawlBusy()
	}
	writeJSON(w, 200, map[string]any{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"busy":      busy,
	})
}

func (s *Server) handleStartCrawl(w http.ResponseWriter, r *http.Request) {
	s.startCrawl(w)
}

func (s *Server) handleCrawlRuns(w http.ResponseWriter, r *http.Request) {
	if !s.adminOK(r) {
		writeErr(w, 401, "UNAUTHORIZED", "需要管理令牌")
		return
	}
	list, err := s.Store.ListCrawlRuns(50)
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	writeJSON(w, 200, map[string]any{"items": list})
}

func (s *Server) handleCrawlNow(w http.ResponseWriter, r *http.Request) {
	if !s.adminOK(r) {
		writeErr(w, 401, "UNAUTHORIZED", "需要管理令牌")
		return
	}
	s.startCrawl(w)
}

func (s *Server) startCrawl(w http.ResponseWriter) {
	if s.Crawl == nil {
		writeErr(w, 500, "INTERNAL", "采集未配置")
		return
	}
	if err := s.Crawl(); err != nil {
		if errors.Is(err, crawl.ErrBusy) {
			writeErr(w, 409, "CRAWL_BUSY", "采集进行中")
			return
		}
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true})
}

func (s *Server) adminOK(r *http.Request) bool {
	if s.Cfg.AdminToken != "" {
		return r.Header.Get("X-Admin-Token") == s.Cfg.AdminToken
	}
	host := r.Host
	return strings.HasPrefix(host, "127.0.0.1") || strings.HasPrefix(host, "localhost")
}

type holdingView struct {
	ProductCode                string `json:"product_code"`
	Name                       string `json:"name"`
	Listed                     bool   `json:"listed"`
	Shares                     string `json:"shares"`
	Cost                       string `json:"cost"`
	MarketValue                string `json:"market_value"`
	Unrealized                 string `json:"unrealized"`
	Realized                   string `json:"realized"`
	Cumulative                 string `json:"cumulative"`
	DailyPnl                   string `json:"daily_pnl"`
	DisplayDate                string `json:"display_date"`
	LatestNav                  string `json:"latest_nav"`
	LatestNavDate              string `json:"latest_nav_date"`
	HangZero                   bool   `json:"hang_zero"`
	StaleDays                  int    `json:"stale_days"`
	Closed                     bool   `json:"closed"`
	CumulativeReturn           string `json:"cumulative_return"`
	AnnualizedCumulativeReturn string `json:"annualized_cumulative_return"`
	MarketValueFen             int64  `json:"-"`
	CumulativeFen              int64  `json:"-"`
	DailyPnlFen                int64  `json:"-"`
	TotalBuyFen                int64  `json:"-"`
}

func (s *Server) holdingViews(userID int64, includeClosed bool) ([]holdingView, error) {
	hs, err := s.Store.ListHoldings(userID)
	if err != nil {
		return nil, err
	}
	today := s.today()
	lag := s.Cfg.DisplayLagDays
	if lag < 0 {
		lag = 0
	}
	yesterday, _ := addDays(today, -lag)
	out := make([]holdingView, 0, len(hs))
	for _, h := range hs {
		p, err := s.Store.Product(h.ProductCode)
		if err != nil {
			continue
		}
		entries, err := s.ledgerAsPnl(h.ID)
		if err != nil {
			return nil, err
		}
		st, err := pnl.Replay(entries)
		if err != nil {
			return nil, err
		}
		if st.SharesE8 == 0 && !includeClosed {
			continue
		}
		navs := s.navPoints(h.ProductCode)
		latest, hasLatest := pnl.LatestNav(navs)
		var mv, unrel int64
		latestNavS, latestDate := "", ""
		if hasLatest {
			mv = money.CashFromShares(st.SharesE8, latest.UnitNavE8)
			unrel = mv - st.CostFen
			latestNavS = money.FormatNav(latest.UnitNavE8)
			latestDate = latest.Date
		}
		cum := unrel + st.RealizedFen
		disp, hasDisp := pnl.DisplayDate(navs, today, lag)
		var day int64
		if hasDisp {
			day = pnl.DailyPnl(entries, navs, disp)
		}
		_, hasY := pnl.NavOn(navs, yesterday)
		stale := 0
		if hasLatest {
			stale = pnl.DaysBetween(latest.Date, today)
		}
		latestPt := pnl.NavPoint{}
		if hasLatest {
			latestPt = latest
		}
		out = append(out, holdingView{
			ProductCode: h.ProductCode, Name: p.Name, Listed: p.Listed,
			Shares: money.FormatShares(st.SharesE8), Cost: money.FormatFen(st.CostFen),
			MarketValue: money.FormatFen(mv), Unrealized: money.FormatFen(unrel),
			Realized: money.FormatFen(st.RealizedFen), Cumulative: money.FormatFen(cum),
			DailyPnl: money.FormatFen(day), DisplayDate: disp,
			LatestNav: latestNavS, LatestNavDate: latestDate,
			HangZero: !hasY, StaleDays: stale, Closed: st.SharesE8 == 0,
			CumulativeReturn:           formatPctE4(pnl.CumulativeReturnE4(cum, st.TotalBuyFen)),
			AnnualizedCumulativeReturn: formatPctE4(pnl.AnnualizedCumulativeReturnE4(entries, latestPt)),
			MarketValueFen:             mv, CumulativeFen: cum, DailyPnlFen: day, TotalBuyFen: st.TotalBuyFen,
		})
	}
	return out, nil
}

func (s *Server) navPoints(code string) []pnl.NavPoint {
	list, err := s.Store.ListNav(code)
	if err != nil {
		return nil
	}
	out := make([]pnl.NavPoint, 0, len(list))
	for _, n := range list {
		out = append(out, pnl.NavPoint{Date: n.NavDate, UnitNavE8: n.UnitNavE8})
	}
	return out
}

func (s *Server) ledgerAsPnl(holdingID int64) ([]pnl.Entry, error) {
	led, err := s.Store.ListLedger(holdingID)
	if err != nil {
		return nil, err
	}
	out := make([]pnl.Entry, 0, len(led))
	for _, e := range led {
		out = append(out, pnl.Entry{
			ID: e.ID, Kind: pnl.Kind(e.Kind), OccurDate: e.OccurDate,
			CashFen: e.CashFen, SharesE8: e.SharesE8, UnitNavE8: e.UnitNavE8,
			NavDateUsed: e.NavDateUsed, Voided: e.VoidedAt.Valid,
		})
	}
	return out, nil
}

func (s *Server) normOccur(d string) string {
	d = strings.TrimSpace(d)
	if d == "" {
		return s.today()
	}
	if _, err := time.Parse("2006-01-02", d); err != nil {
		return ""
	}
	return d
}

func mapLedger(led []store.Ledger) []map[string]any {
	out := make([]map[string]any, 0, len(led))
	for _, e := range led {
		out = append(out, map[string]any{
			"id": e.ID, "kind": e.Kind, "occur_date": e.OccurDate,
			"cash": money.FormatFen(e.CashFen), "shares": money.FormatShares(e.SharesE8),
			"unit_nav": money.FormatNav(e.UnitNavE8), "nav_date_used": e.NavDateUsed,
			"voided": e.VoidedAt.Valid,
		})
	}
	return out
}

func addDays(iso string, n int) (string, error) {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return "", err
	}
	return t.AddDate(0, 0, n).Format("2006-01-02"), nil
}

func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func formatPctE4(e4 int64, ok bool) string {
	if !ok {
		return ""
	}
	return money.FormatSignedPctE4(e4)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": code, "message": msg})
}
