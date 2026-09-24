package crawl

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNoCatalogPage    = errors.New("没有记下代销目录页")
	ErrProductNotOnPage = errors.New("这一页没有该产品")
	ErrPageUnread       = errors.New("这一页没有读成")
	ErrNoUnitNav        = errors.New("本页没有单位净值")
)

// RefreshProduct reads the catalog page last recorded for code and writes that product's latest published nav.
// It does not start a crawl and does not write an observation.
func (r *Runner) RefreshProduct(code string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	p, err := r.Store.Product(code)
	if err != nil {
		return err
	}
	if p.LastSeenPage <= 0 {
		return ErrNoCatalogPage
	}
	body, status, err := r.Fetcher.Get(catalogPageURL(r.BaseURL, p.LastSeenPage))
	if err != nil || status < 200 || status >= 300 || strings.TrimSpace(body) == "" {
		return ErrPageUnread
	}
	rows, err := ParsePage(body)
	if err != nil {
		return ErrPageUnread
	}
	row, found, hasUnit := pickProductRow(rows, code)
	if !found {
		return ErrProductNotOnPage
	}
	now := r.Now()
	if err := r.Store.UpsertProduct(row.Code, row.Name, row.Issuer, p.LastSeenPage, now); err != nil {
		return err
	}
	if !hasUnit {
		return ErrNoUnitNav
	}
	return r.Store.UpsertSnapshot(row.Code, row.NavDate, row.UnitNavE8, row.AccNavE8, row.DailyReturnBP, row.ExtraJSON, now.Format(time.RFC3339))
}

func catalogPageURL(base string, page int) string {
	base = strings.TrimRight(base, "/") + "/"
	if page <= 1 {
		return base + "index.html"
	}
	return fmt.Sprintf("%sindex_%d.html", base, page-1)
}

func pickProductRow(rows []Row, code string) (Row, bool, bool) {
	var picked Row
	found := false
	hasUnit := false
	for _, row := range rows {
		if strings.ToUpper(row.Code) != code || row.NavDate == "" {
			continue
		}
		found = true
		if hasUnit && !row.HasUnitNav {
			continue
		}
		if !row.HasUnitNav {
			if !hasUnit {
				picked = row
			}
			continue
		}
		picked = row
		hasUnit = true
	}
	return picked, found, hasUnit
}
