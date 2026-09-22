package server

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"money-tacker/internal/money"
	"money-tacker/internal/pnl"
	"money-tacker/internal/store"
)

type catalogRow struct {
	Product       store.Product
	LatestNav     string
	LatestNavDate string
	Holding       string
	Ret           map[int]int64
	RetOK         map[int]bool
}

func (s *Server) handleProductCatalog(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	products, err := s.Store.ListProducts()
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	snaps, err := s.Store.ListAllNav()
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}
	navBy := map[string][]pnl.NavPoint{}
	for _, n := range snaps {
		navBy[n.ProductCode] = append(navBy[n.ProductCode], pnl.NavPoint{Date: n.NavDate, UnitNavE8: n.UnitNavE8})
	}
	hold, err := s.holdingStatusByCode(u.ID)
	if err != nil {
		writeErr(w, 500, "INTERNAL", "服务器错误")
		return
	}

	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	includeUnlisted := r.URL.Query().Get("include_unlisted") == "true"
	sortKey := r.URL.Query().Get("sort")
	if sortKey == "" {
		sortKey = "ret_360"
	}
	desc := r.URL.Query().Get("order") != "asc"

	var rows []catalogRow
	for _, p := range products {
		if !includeUnlisted && !p.Listed {
			continue
		}
		if q != "" && !catalogMatch(p, q) {
			continue
		}
		navs := navBy[p.Code]
		row := catalogRow{
			Product: p,
			Holding: hold[p.Code],
			Ret:     map[int]int64{},
			RetOK:   map[int]bool{},
		}
		if row.Holding == "" {
			row.Holding = "none"
		}
		if latest, ok := pnl.LatestNav(navs); ok {
			row.LatestNav = money.FormatNav(latest.UnitNavE8)
			row.LatestNavDate = latest.Date
		}
		for _, days := range pnl.CatalogWindows {
			if e4, ok := pnl.WindowPctE4(navs, days); ok {
				row.Ret[days] = e4
				row.RetOK[days] = true
			}
		}
		rows = append(rows, row)
	}
	sortCatalog(rows, sortKey, desc)

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}
	total := len(rows)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	pageRows := rows[start:end]
	items := make([]map[string]any, 0, len(pageRows))
	for _, row := range pageRows {
		item := map[string]any{
			"product_code":    row.Product.Code,
			"name":            row.Product.Name,
			"issuer":          row.Product.Issuer,
			"listed":          row.Product.Listed,
			"latest_nav":      row.LatestNav,
			"latest_nav_date": row.LatestNavDate,
			"holding":         row.Holding,
			"ret_30":          retJSON(row, 30),
			"ret_90":          retJSON(row, 90),
			"ret_180":         retJSON(row, 180),
			"ret_360":         retJSON(row, 360),
			"ret_730":         retJSON(row, 730),
		}
		items = append(items, item)
	}
	out := map[string]any{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}
	if len(products) == 0 {
		out["empty"] = "no_products"
	}
	writeJSON(w, 200, out)
}

func catalogMatch(p store.Product, q string) bool {
	return strings.Contains(strings.ToLower(p.Code), q) ||
		strings.Contains(strings.ToLower(p.Name), q) ||
		strings.Contains(strings.ToLower(p.Issuer), q)
}

func retJSON(row catalogRow, days int) any {
	if !row.RetOK[days] {
		return nil
	}
	return money.FormatSignedPctE4(row.Ret[days])
}

func sortCatalog(rows []catalogRow, key string, desc bool) {
	days := 0
	switch key {
	case "ret_30":
		days = 30
	case "ret_90":
		days = 90
	case "ret_180":
		days = 180
	case "ret_360":
		days = 360
	case "ret_730":
		days = 730
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		cmp := 0
		switch {
		case days > 0:
			cmp = cmpNullLast(a.RetOK[days], a.Ret[days], b.RetOK[days], b.Ret[days], desc)
		case key == "latest_nav_date":
			cmp = cmpNullLastStr(a.LatestNavDate != "", a.LatestNavDate, b.LatestNavDate != "", b.LatestNavDate, desc)
		case key == "listed":
			cmp = cmpBool(a.Product.Listed, b.Product.Listed, desc)
		case key == "name":
			cmp = cmpStr(a.Product.Name, b.Product.Name, desc)
		case key == "issuer":
			cmp = cmpStr(a.Product.Issuer, b.Product.Issuer, desc)
		default:
			cmp = cmpStr(a.Product.Code, b.Product.Code, desc)
		}
		if cmp != 0 {
			return cmp < 0
		}
		return a.Product.Code < b.Product.Code
	})
}

func cmpNullLast(aOK bool, a int64, bOK bool, b int64, desc bool) int {
	if aOK && bOK {
		if a == b {
			return 0
		}
		if desc {
			if a > b {
				return -1
			}
			return 1
		}
		if a < b {
			return -1
		}
		return 1
	}
	if aOK {
		return -1
	}
	if bOK {
		return 1
	}
	return 0
}

func cmpNullLastStr(aOK bool, a string, bOK bool, b string, desc bool) int {
	if aOK && bOK {
		return cmpStr(a, b, desc)
	}
	if aOK {
		return -1
	}
	if bOK {
		return 1
	}
	return 0
}

func cmpStr(a, b string, desc bool) int {
	if a == b {
		return 0
	}
	less := a < b
	if desc {
		less = a > b
	}
	if less {
		return -1
	}
	return 1
}

func cmpBool(a, b, desc bool) int {
	if a == b {
		return 0
	}
	ai, bi := 0, 0
	if a {
		ai = 1
	}
	if b {
		bi = 1
	}
	return cmpNullLast(true, int64(ai), true, int64(bi), desc)
}

func (s *Server) holdingStatusByCode(userID int64) (map[string]string, error) {
	hs, err := s.Store.ListHoldings(userID)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, h := range hs {
		entries, err := s.ledgerAsPnl(h.ID)
		if err != nil {
			return nil, err
		}
		st, err := pnl.Replay(entries)
		if err != nil {
			return nil, err
		}
		if st.SharesE8 > 0 {
			out[h.ProductCode] = "open"
		} else {
			out[h.ProductCode] = "closed"
		}
	}
	return out, nil
}
