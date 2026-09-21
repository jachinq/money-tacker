package crawl

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"money-tacker/internal/money"
)

type Row struct {
	Code          string
	Name          string
	Issuer        string
	UnitNavE8     int64
	AccNavE8      int64
	DailyReturnBP int64
	NavDate       string
	ExtraJSON     string
	HasUnitNav    bool
}

var dateRe = regexp.MustCompile(`(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})`)

func ParsePage(html string) ([]Row, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}
	var out []Row
	doc.Find("table").Each(func(_ int, table *goquery.Selection) {
		var headers []string
		table.Find("tr").Each(func(i int, tr *goquery.Selection) {
			cells := tr.Find("th, td")
			if cells.Length() == 0 {
				return
			}
			vals := make([]string, 0, cells.Length())
			cells.Each(func(_ int, td *goquery.Selection) {
				vals = append(vals, strings.TrimSpace(td.Text()))
			})
			if i == 0 || looksLikeHeader(vals) {
				if looksLikeHeader(vals) {
					headers = vals
					return
				}
			}
			if len(headers) == 0 {
				return
			}
			row := rowFrom(headers, vals)
			if row.Code != "" {
				out = append(out, row)
			}
		})
	})
	return out, nil
}

func looksLikeHeader(vals []string) bool {
	joined := strings.Join(vals, "")
	return strings.Contains(joined, "产品代码") || strings.Contains(joined, "代码")
}

func rowFrom(headers, vals []string) Row {
	m := map[string]string{}
	for i, h := range headers {
		if i < len(vals) {
			m[h] = vals[i]
		}
	}
	var r Row
	r.Code = first(m, "产品代码", "代码")
	r.Name = first(m, "产品名称", "名称")
	r.Issuer = first(m, "发行机构", "管理人")
	if v := first(m, "单位净值"); v != "" {
		if n, ok := money.ParseNavToE8(normalizeNum(v)); ok {
			r.UnitNavE8 = n
			r.HasUnitNav = true
		}
	}
	if v := first(m, "累计净值"); v != "" {
		if n, ok := money.ParseNavToE8(normalizeNum(v)); ok {
			r.AccNavE8 = n
		}
	}
	if v := first(m, "日净值增长率", "净值增长率"); v != "" {
		r.DailyReturnBP = parseBP(v)
	}
	if v := first(m, "截止日期", "净值日期"); v != "" {
		r.NavDate = normalizeDate(v)
	}
	return r
}

func first(m map[string]string, keys ...string) string {
	for k, v := range m {
		for _, want := range keys {
			if strings.Contains(k, want) {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

func normalizeNum(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSuffix(s, "%")
	return s
}

func parseBP(s string) int64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, "%", ""))
	s = strings.ReplaceAll(s, ",", "")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f * 100) // percent to bp? 0.02% -> 2 bp if f=0.02. daily return often "0.0123%" or "0.01"
}

func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	m := dateRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	y, mo, d := m[1], m[2], m[3]
	if len(mo) == 1 {
		mo = "0" + mo
	}
	if len(d) == 1 {
		d = "0" + d
	}
	return y + "-" + mo + "-" + d
}
