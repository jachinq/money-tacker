package crawl

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"money-tacker/internal/db"
	"money-tacker/internal/store"
	"money-tacker/migrations"
)

type mapFetch map[string]string

func (m mapFetch) Get(url string) (string, int, error) {
	if b, ok := m[url]; ok {
		return b, 200, nil
	}
	return "", 404, nil
}

func TestUpsertSnapshotAndTwoObservations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	sqlDB, err := db.Open(path, migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	st := &store.Store{DB: sqlDB}
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	page1 := `https://example.test/index.html`
	html := fixture
	r := &Runner{
		Store: st,
		Fetcher: mapFetch{
			page1: html,
		},
		BaseURL:  "https://example.test/",
		Delay:    0,
		Now:      func() time.Time { return now },
		MaxPages: 3,
	}
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	navs, err := st.ListNav("AF247494G")
	if err != nil {
		t.Fatal(err)
	}
	if len(navs) != 1 {
		t.Fatalf("snapshots=%d want 1", len(navs))
	}
	var n int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM nav_observation WHERE product_code='AF247494G'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("observations=%d want 2", n)
	}
	var runs int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM crawl_run`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 2 {
		t.Fatalf("runs=%d", runs)
	}
}

func TestRepeatSightingSameNavKeepsOneObservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	sqlDB, err := db.Open(path, migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	st := &store.Store{DB: sqlDB}
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	html := `
<html><body><table>
<tr><th>产品代码</th><th>产品名称</th><th>发行机构</th><th>单位净值</th><th>累计净值</th><th>日净值增长率</th><th>截止日期</th></tr>
<tr><td>AF247494G</td><td>先见名称</td><td>先见机构</td><td>1.0562</td><td>1.0562</td><td>0.02%</td><td>2026/09/18</td></tr>
<tr><td>AF247494G</td><td>后见名称</td><td>后见机构</td><td>1.0562</td><td>1.0700</td><td>0.05%</td><td>2026/09/18</td></tr>
</table></body></html>`
	r := &Runner{
		Store:    st,
		Fetcher:  mapFetch{"https://example.test/index.html": html},
		BaseURL:  "https://example.test/",
		Now:      func() time.Time { return now },
		MaxPages: 2,
	}
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	navs, err := st.ListNav("AF247494G")
	if err != nil {
		t.Fatal(err)
	}
	if len(navs) != 1 || navs[0].NavDate != "2026-09-18" || navs[0].UnitNavE8 != 105620000 {
		t.Fatalf("nav %+v", navs)
	}
	if !navs[0].AccNavE8.Valid || navs[0].AccNavE8.Int64 != 107000000 {
		t.Fatalf("acc %+v", navs[0].AccNavE8)
	}
	if !navs[0].DailyReturnBP.Valid || navs[0].DailyReturnBP.Int64 != 5 {
		t.Fatalf("bp %+v", navs[0].DailyReturnBP)
	}
	var n int
	var navDate string
	var unit int64
	if err := sqlDB.QueryRow(`SELECT COUNT(*), nav_date, unit_nav FROM nav_observation WHERE product_code='AF247494G'`).Scan(&n, &navDate, &unit); err != nil {
		t.Fatal(err)
	}
	if n != 1 || navDate != "2026-09-18" || unit != 105620000 {
		t.Fatalf("observation n=%d date=%s unit=%d", n, navDate, unit)
	}
	runs, err := st.ListCrawlRuns(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "success" || runs[0].ErrorSummary.Valid {
		t.Fatalf("run %+v", runs)
	}
	p, err := st.Product("AF247494G")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "后见名称" || p.Issuer != "后见机构" {
		t.Fatalf("product %+v", p)
	}
}

func TestLaterSightingWithoutUnitNavLeavesObservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	sqlDB, err := db.Open(path, migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	st := &store.Store{DB: sqlDB}
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	html := `
<html><body><table>
<tr><th>产品代码</th><th>产品名称</th><th>单位净值</th><th>累计净值</th><th>截止日期</th></tr>
<tr><td>AF247494G</td><td>先见</td><td>1.0562</td><td>1.0562</td><td>2026/09/18</td></tr>
<tr><td>AF247494G</td><td>后见</td><td></td><td></td><td>2026/09/19</td></tr>
</table></body></html>`
	r := &Runner{
		Store:    st,
		Fetcher:  mapFetch{"https://example.test/index.html": html},
		BaseURL:  "https://example.test/",
		Now:      func() time.Time { return now },
		MaxPages: 2,
	}
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	navs, err := st.ListNav("AF247494G")
	if err != nil {
		t.Fatal(err)
	}
	if len(navs) != 1 || navs[0].NavDate != "2026-09-18" || navs[0].UnitNavE8 != 105620000 {
		t.Fatalf("nav %+v", navs)
	}
	var navDate string
	var unit int64
	if err := sqlDB.QueryRow(`SELECT nav_date, unit_nav FROM nav_observation WHERE product_code='AF247494G'`).Scan(&navDate, &unit); err != nil {
		t.Fatal(err)
	}
	if navDate != "2026-09-18" || unit != 105620000 {
		t.Fatalf("observation date=%s unit=%d", navDate, unit)
	}
	runs, err := st.ListCrawlRuns(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "success" || !runs[0].ErrorSummary.Valid || !strings.Contains(runs[0].ErrorSummary.String, "AF247494G") || !strings.Contains(runs[0].ErrorSummary.String, "缺少单位净值") {
		t.Fatalf("run %+v", runs)
	}
}

func TestLaterNavDateKeepsBothSnapshots(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	sqlDB, err := db.Open(path, migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	st := &store.Store{DB: sqlDB}
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	html := `
<html><body><table>
<tr><th>产品代码</th><th>产品名称</th><th>单位净值</th><th>累计净值</th><th>截止日期</th></tr>
<tr><td>AF247494G</td><td>先见</td><td>1.0562</td><td>1.0562</td><td>2026/09/18</td></tr>
<tr><td>AF247494G</td><td>后见</td><td>1.0570</td><td>1.0570</td><td>2026/09/19</td></tr>
</table></body></html>`
	r := &Runner{
		Store:    st,
		Fetcher:  mapFetch{"https://example.test/index.html": html},
		BaseURL:  "https://example.test/",
		Now:      func() time.Time { return now },
		MaxPages: 2,
	}
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	navs, err := st.ListNav("AF247494G")
	if err != nil {
		t.Fatal(err)
	}
	if len(navs) != 2 || navs[0].NavDate != "2026-09-18" || navs[0].UnitNavE8 != 105620000 || navs[1].NavDate != "2026-09-19" || navs[1].UnitNavE8 != 105700000 {
		t.Fatalf("nav %+v", navs)
	}
	var n int
	var navDate string
	var unit int64
	if err := sqlDB.QueryRow(`SELECT COUNT(*), nav_date, unit_nav FROM nav_observation WHERE product_code='AF247494G'`).Scan(&n, &navDate, &unit); err != nil {
		t.Fatal(err)
	}
	if n != 1 || navDate != "2026-09-19" || unit != 105700000 {
		t.Fatalf("observation n=%d date=%s unit=%d", n, navDate, unit)
	}
	runs, err := st.ListCrawlRuns(1)
	if err != nil {
		t.Fatal(err)
	}
	sum := ""
	if len(runs) == 1 && runs[0].ErrorSummary.Valid {
		sum = runs[0].ErrorSummary.String
	}
	if len(runs) != 1 || runs[0].Status != "success" || !strings.Contains(sum, "AF247494G") || !strings.Contains(sum, "2026-09-18") || !strings.Contains(sum, "2026-09-19") {
		t.Fatalf("run %+v", runs)
	}
}

func TestLaterUnitNavReplacesSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	sqlDB, err := db.Open(path, migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	st := &store.Store{DB: sqlDB}
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	html := `
<html><body><table>
<tr><th>产品代码</th><th>产品名称</th><th>单位净值</th><th>累计净值</th><th>截止日期</th></tr>
<tr><td>AF247494G</td><td>先见</td><td>1.0562</td><td>1.0562</td><td>2026/09/18</td></tr>
<tr><td>AF247494G</td><td>后见</td><td>1.0570</td><td>1.0570</td><td>2026/09/18</td></tr>
</table></body></html>`
	r := &Runner{
		Store:    st,
		Fetcher:  mapFetch{"https://example.test/index.html": html},
		BaseURL:  "https://example.test/",
		Now:      func() time.Time { return now },
		MaxPages: 2,
	}
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	navs, err := st.ListNav("AF247494G")
	if err != nil {
		t.Fatal(err)
	}
	if len(navs) != 1 || navs[0].UnitNavE8 != 105700000 {
		t.Fatalf("nav %+v", navs)
	}
	var unit int64
	if err := sqlDB.QueryRow(`SELECT unit_nav FROM nav_observation WHERE product_code='AF247494G'`).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	if unit != 105700000 {
		t.Fatalf("observation unit=%d", unit)
	}
	runs, err := st.ListCrawlRuns(1)
	if err != nil {
		t.Fatal(err)
	}
	sum := ""
	if len(runs) == 1 && runs[0].ErrorSummary.Valid {
		sum = runs[0].ErrorSummary.String
	}
	if len(runs) != 1 || runs[0].Status != "success" || !strings.Contains(sum, "105620000") || !strings.Contains(sum, "105700000") {
		t.Fatalf("run %+v", runs)
	}
}

func TestObservationNoteStillUpdatesListed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	sqlDB, err := db.Open(path, migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	st := &store.Store{DB: sqlDB}
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	if err := st.UpsertProduct("GONE0001", "已消失", "", 1, now); err != nil {
		t.Fatal(err)
	}
	html := `
<html><body><table>
<tr><th>产品代码</th><th>产品名称</th><th>单位净值</th><th>累计净值</th><th>截止日期</th></tr>
<tr><td>AF247494G</td><td>先见</td><td>1.0562</td><td>1.0562</td><td>2026/09/18</td></tr>
<tr><td>AF247494G</td><td>后见</td><td>1.0570</td><td>1.0570</td><td>2026/09/19</td></tr>
</table></body></html>`
	r := &Runner{
		Store:    st,
		Fetcher:  mapFetch{"https://example.test/index.html": html},
		BaseURL:  "https://example.test/",
		Now:      func() time.Time { return now },
		MaxPages: 2,
	}
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	gone, err := st.Product("GONE0001")
	if err != nil {
		t.Fatal(err)
	}
	if gone.Listed {
		t.Fatal("missing product stayed listed")
	}
	kept, err := st.Product("AF247494G")
	if err != nil {
		t.Fatal(err)
	}
	if !kept.Listed {
		t.Fatal("seen product unlisted")
	}
}

type errFetch struct{}

func (errFetch) Get(string) (string, int, error) {
	return "", 0, errDown
}

type downError struct{}

func (downError) Error() string { return "down" }

var errDown = downError{}

func TestPageFailureDoesNotUpdateListed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	sqlDB, err := db.Open(path, migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	st := &store.Store{DB: sqlDB}
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	if err := st.UpsertProduct("GONE0001", "已消失", "", 1, now); err != nil {
		t.Fatal(err)
	}
	r := &Runner{
		Store:    st,
		Fetcher:  errFetch{},
		BaseURL:  "https://example.test/",
		Now:      func() time.Time { return now },
		MaxPages: 2,
	}
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	gone, err := st.Product("GONE0001")
	if err != nil {
		t.Fatal(err)
	}
	if !gone.Listed {
		t.Fatal("page failure unlisted product")
	}
	runs, err := st.ListCrawlRuns(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "fail" {
		t.Fatalf("run %+v", runs)
	}
}
